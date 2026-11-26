// Package nativeui shows the setup wizard in one native OS window instead of
// a browser page. It stays CGO-free (so release builds still cross-compile) by
// driving the GUI toolkits that ship with the OS: Cocoa via
// `osascript -l JavaScript` on macOS and WinForms via Windows PowerShell on
// Windows. The window process lives for the whole wizard; Go sends it JSON
// commands on stdin and reads the user's actions as JSON lines from stdout.
// Other platforms return ErrUnsupported and use the browser form.
package nativeui

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"

	"github.com/hsnrzv/jira-mcp/internal/jira"
)

// ErrUnsupported is returned by Open on platforms without a native backend.
var ErrUnsupported = errors.New("native dialogs are not supported on this platform")

// Action is what the user did in the window.
type Action string

const (
	ActionNext   Action = "next"   // step 1: connect
	ActionBack   Action = "back"   // step 2: back to step 1
	ActionSave   Action = "save"   // step 2: validate and save
	ActionChoose Action = "choose" // step 3: install/copy button Index
	ActionClose  Action = "close"  // window closed or "Close server"
)

// Event is a user action. Only the fields relevant to Action are set; it
// carries the PAT typed in step 1, so never log it.
type Event struct {
	Action           Action   `json:"action"`
	JiraURL          string   `json:"jiraUrl"`
	PersonalToken    string   `json:"personalToken"`
	CustomCACertPath string   `json:"customCaCertPath"`
	ProjectKeys      []string `json:"projectKeys"`
	EnableWriteTools bool     `json:"enableWriteTools"`
	Index            int      `json:"index"`
}

// ConnectPage is step 1: Jira URL, PAT and optional CA file. The PAT field is
// always shown empty; TokenHint is its placeholder.
type ConnectPage struct {
	Message          string
	JiraURL          string
	CustomCACertPath string
	TokenHint        string
}

// ProjectsPage is step 2: pick the allowed projects.
type ProjectsPage struct {
	Message          string
	Projects         []jira.Project
	Selected         []string
	EnableWriteTools bool
}

// RegisterPage is step 3: install/copy buttons; Status is shown as success.
type RegisterPage struct {
	Message string
	Status  string
	Buttons []string
}

// Window is one native setup window. Every user action except ActionClose
// puts the window into a busy state (buttons disabled, spinner) until the next
// Show*, Status or Copy call.
type Window interface {
	ShowConnect(ConnectPage) error
	ShowProjects(ProjectsPage) error
	ShowRegister(RegisterPage) error
	// Status shows text on the current page (in red when isErr).
	Status(text string, isErr bool) error
	// Copy puts text on the clipboard and shows message as the status.
	Copy(text, message string) error
	// Next blocks until the user acts. After the window was closed it
	// returns ActionClose.
	Next(ctx context.Context) (Event, error)
	// Closed is closed as soon as the window is gone.
	Closed() <-chan struct{}
	// Close closes the window (if still open) and waits for its process.
	Close() error
}

var (
	//go:embed scripts/macos_window.js
	macWindow string
	//go:embed scripts/windows_window.ps1
	winWindow string
)

// command is the JSON message sent to the window process.
type command struct {
	Cmd              string        `json:"cmd"`
	Title            string        `json:"title,omitempty"`
	Icon             string        `json:"icon,omitempty"`
	IconFile         string        `json:"iconFile,omitempty"`
	Message          string        `json:"message,omitempty"`
	Status           string        `json:"status,omitempty"`
	Error            bool          `json:"error,omitempty"`
	JiraURL          string        `json:"jiraUrl,omitempty"`
	CustomCACertPath string        `json:"customCaCertPath,omitempty"`
	TokenHint        string        `json:"tokenHint,omitempty"`
	Projects         []projectItem `json:"projects,omitempty"`
	Selected         []string      `json:"selected,omitempty"`
	EnableWriteTools bool          `json:"enableWriteTools,omitempty"`
	Buttons          []string      `json:"buttons,omitempty"`
	Text             string        `json:"text,omitempty"`
}

type projectItem struct {
	Key  string `json:"key"`
	Name string `json:"name,omitempty"`
}

// starter launches the window process and returns its pipes and a wait
// function that returns once it has exited.
type starter func(ctx context.Context) (stdin io.WriteCloser, stdout io.Reader, wait func() error, err error)

// Open starts the native window for the current OS. title and iconPNG are
// shown in the title bar, header and Dock/taskbar.
func Open(ctx context.Context, title string, iconPNG []byte) (Window, error) {
	init := command{Cmd: "init", Title: title}
	if len(iconPNG) > 0 {
		init.Icon = base64.StdEncoding.EncodeToString(iconPNG)
	}
	var start starter
	switch runtime.GOOS {
	case "darwin":
		init.IconFile = bundleIcon()
		start = osascript(macWindow)
	case "windows":
		start = powerShell(winWindow)
	default:
		return nil, ErrUnsupported
	}
	return open(ctx, start, init)
}

// bundleIcon returns the high-resolution icon of the enclosing .app, if any.
func bundleIcon() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	p := filepath.Join(filepath.Dir(exe), "..", "Resources", "AppIcon.icns")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// process implements Window over a child process speaking JSON lines.
type process struct {
	in     io.WriteCloser
	cancel context.CancelFunc

	mu sync.Mutex // serializes writes to in

	events     chan Event
	closed     chan struct{} // window gone (close event or process exit)
	closedOnce sync.Once
	userClosed atomic.Bool
	exited     chan struct{}
	exitErr    error // valid after exited is closed
	quit       chan struct{}
	quitOnce   sync.Once
}

func open(ctx context.Context, start starter, init command) (*process, error) {
	ctx, cancel := context.WithCancel(ctx)
	in, out, wait, err := start(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	p := &process{
		in:     in,
		cancel: cancel,
		events: make(chan Event, 16),
		closed: make(chan struct{}),
		exited: make(chan struct{}),
		quit:   make(chan struct{}),
	}
	go p.read(out, wait)
	if err := p.send(init); err != nil {
		_ = p.Close()
		return nil, err
	}
	return p, nil
}

func (p *process) markClosed() { p.closedOnce.Do(func() { close(p.closed) }) }

func (p *process) read(out io.Reader, wait func() error) {
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(bytes.TrimPrefix(sc.Bytes(), []byte("\xef\xbb\xbf")))
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			slog.Warn("ignoring unexpected output from the setup window")
			continue
		}
		if ev.Action == ActionClose {
			p.userClosed.Store(true)
			p.markClosed()
		}
		select {
		case p.events <- ev:
		case <-p.quit:
		}
	}
	_, _ = io.Copy(io.Discard, out)
	p.exitErr = wait()
	p.markClosed()
	close(p.exited)
	close(p.events)
}

// send writes one command. Writing to a window the user has just closed is
// not an error: the caller learns about it from Next.
func (p *process) send(c command) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.userClosed.Load() {
		return nil
	}
	if _, err := p.in.Write(append(b, '\n')); err != nil {
		select {
		case <-p.exited:
		case <-time.After(2 * time.Second):
		}
		if p.userClosed.Load() {
			return nil
		}
		return fmt.Errorf("setup window: %w", p.exitError(err))
	}
	return nil
}

// exitError prefers the process exit error (it includes stderr) over err.
func (p *process) exitError(err error) error {
	select {
	case <-p.exited:
		if p.exitErr != nil {
			return p.exitErr
		}
	default:
	}
	return err
}

func (p *process) ShowConnect(pg ConnectPage) error {
	return p.send(command{Cmd: "connect", Message: pg.Message, JiraURL: pg.JiraURL,
		CustomCACertPath: pg.CustomCACertPath, TokenHint: pg.TokenHint})
}

func (p *process) ShowProjects(pg ProjectsPage) error {
	items := make([]projectItem, len(pg.Projects))
	for i, pr := range pg.Projects {
		items[i] = projectItem{Key: pr.Key, Name: pr.Name}
	}
	return p.send(command{Cmd: "projects", Message: pg.Message, Projects: items,
		Selected: pg.Selected, EnableWriteTools: pg.EnableWriteTools})
}

func (p *process) ShowRegister(pg RegisterPage) error {
	return p.send(command{Cmd: "register", Message: pg.Message, Status: pg.Status, Buttons: pg.Buttons})
}

func (p *process) Status(text string, isErr bool) error {
	return p.send(command{Cmd: "status", Status: text, Error: isErr})
}

func (p *process) Copy(text, message string) error {
	return p.send(command{Cmd: "copy", Text: text, Message: message})
}

func (p *process) Next(ctx context.Context) (Event, error) {
	select {
	case <-ctx.Done():
		return Event{}, ctx.Err()
	case ev, ok := <-p.events:
		if ok {
			return ev, nil
		}
		if p.exitErr != nil && !p.userClosed.Load() {
			return Event{}, fmt.Errorf("setup window: %w", p.exitErr)
		}
		return Event{Action: ActionClose}, nil
	}
}

func (p *process) Closed() <-chan struct{} { return p.closed }

func (p *process) Close() error {
	p.quitOnce.Do(func() {
		_ = p.send(command{Cmd: "quit"})
		_ = p.in.Close()
		close(p.quit)
	})
	select {
	case <-p.exited:
	case <-time.After(3 * time.Second):
		p.cancel()
		<-p.exited
	}
	p.cancel()
	return nil
}

// execStarter runs name with args, wiring stdin/stdout pipes and keeping the
// tail of stderr for error messages.
func execStarter(name string, args []string, env []string) starter {
	return func(ctx context.Context) (io.WriteCloser, io.Reader, func() error, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = append(os.Environ(), env...)
		stderr := &tailBuffer{max: 2000}
		cmd.Stderr = stderr
		in, err := cmd.StdinPipe()
		if err != nil {
			return nil, nil, nil, err
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, nil, nil, fmt.Errorf("start %s: %w", filepath.Base(name), err)
		}
		wait := func() error {
			if err := cmd.Wait(); err != nil {
				return fmt.Errorf("%s: %w: %s", filepath.Base(name), err, strings.TrimSpace(stderr.String()))
			}
			return nil
		}
		return in, out, wait, nil
	}
}

// osascript passes the script as an argument because stdin carries commands.
func osascript(script string) starter {
	return execStarter("/usr/bin/osascript", []string{"-l", "JavaScript", "-e", script}, nil)
}

// powerShell keeps the command line short (Windows limits it to 32767
// characters): a tiny -EncodedCommand bootstrap reads the base64 script from
// the first stdin line, runs it, and leaves $JmcpIn (the stdin reader) for the
// command stream. No user data is ever placed in the script.
func powerShell(script string) starter {
	args := []string{"-NoProfile", "-NonInteractive", "-STA", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShell(psBootstrap)}
	inner := execStarter(powerShellPath(), args, nil)
	return func(ctx context.Context) (io.WriteCloser, io.Reader, func() error, error) {
		in, out, wait, err := inner(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		// The script is larger than a pipe buffer, so write it in the
		// background; later writes wait for it via lockedWriter.
		line := base64.StdEncoding.EncodeToString([]byte(script)) + "\n"
		w := &lockedWriter{w: in}
		w.mu.Lock()
		go func() {
			defer w.mu.Unlock()
			_, _ = io.WriteString(in, line)
		}()
		return w, out, wait, nil
	}
}

const psBootstrap = "$JmcpIn = New-Object System.IO.StreamReader([Console]::OpenStandardInput(), (New-Object System.Text.UTF8Encoding $false)); " +
	". ([scriptblock]::Create([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($JmcpIn.ReadLine()))))"

// lockedWriter delays writes until the bootstrap line has been written.
type lockedWriter struct {
	mu sync.Mutex
	w  io.WriteCloser
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(b)
}

func (l *lockedWriter) Close() error { return l.w.Close() }

func powerShellPath() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	}
	return "powershell.exe"
}

// encodePowerShell returns the base64 UTF-16LE form expected by -EncodedCommand.
func encodePowerShell(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
