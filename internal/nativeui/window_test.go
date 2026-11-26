package nativeui

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/hsnrzv/jira-mcp/internal/jira"
)

// fakeProc plays the window process over in-memory pipes.
type fakeProc struct {
	cmds   chan command // commands received from Go
	out    *io.PipeWriter
	exit   chan error // value returned by wait
	inR    *io.PipeReader
	exited chan struct{}
}

func startFake(t *testing.T) (*process, *fakeProc) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	f := &fakeProc{cmds: make(chan command, 32), out: outW, exit: make(chan error, 1), inR: inR, exited: make(chan struct{})}
	go func() {
		sc := bufio.NewScanner(inR)
		for sc.Scan() {
			var c command
			if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
				t.Errorf("bad command %q: %v", sc.Text(), err)
				continue
			}
			f.cmds <- c
		}
		close(f.cmds)
	}()
	start := func(ctx context.Context) (io.WriteCloser, io.Reader, func() error, error) {
		go func() {
			<-ctx.Done() // killed
			f.stop(errors.New("killed"))
		}()
		wait := func() error {
			<-f.exited
			return <-f.exit
		}
		return inW, outR, wait, nil
	}
	p, err := open(context.Background(), start, command{Cmd: "init", Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.stop(nil); _ = p.Close() })
	return p, f
}

// stop ends the fake process with err (first call wins).
func (f *fakeProc) stop(err error) {
	select {
	case <-f.exited:
		return
	default:
	}
	f.exit <- err
	_ = f.inR.Close()
	_ = f.out.Close()
	close(f.exited)
}

func (f *fakeProc) emit(t *testing.T, line string) {
	t.Helper()
	if _, err := io.WriteString(f.out, line+"\n"); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeProc) recv(t *testing.T) command {
	t.Helper()
	select {
	case c := <-f.cmds:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("no command received")
		return command{}
	}
}

func nextEvent(t *testing.T, p *process) Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ev, err := p.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestWindowCommands(t *testing.T) {
	p, f := startFake(t)
	if c := f.recv(t); c.Cmd != "init" || c.Title != "T" {
		t.Errorf("first command = %#v", c)
	}
	steps := []struct {
		send func() error
		want command
	}{
		{func() error {
			return p.ShowConnect(ConnectPage{Message: "m", JiraURL: "https://j", CustomCACertPath: "/ca", TokenHint: "h"})
		}, command{Cmd: "connect", Message: "m", JiraURL: "https://j", CustomCACertPath: "/ca", TokenHint: "h"}},
		{func() error {
			return p.ShowProjects(ProjectsPage{Message: "m", Projects: []jira.Project{{Key: "A", Name: "Alpha"}}, Selected: []string{"A"}, EnableWriteTools: true})
		}, command{Cmd: "projects", Message: "m", Projects: []projectItem{{Key: "A", Name: "Alpha"}}, Selected: []string{"A"}, EnableWriteTools: true}},
		{func() error { return p.ShowRegister(RegisterPage{Message: "m", Status: "s", Buttons: []string{"b"}}) },
			command{Cmd: "register", Message: "m", Status: "s", Buttons: []string{"b"}}},
		{func() error { return p.Status("bad", true) }, command{Cmd: "status", Status: "bad", Error: true}},
		{func() error { return p.Copy("snippet", "copied") }, command{Cmd: "copy", Text: "snippet", Message: "copied"}},
	}
	for _, s := range steps {
		if err := s.send(); err != nil {
			t.Fatal(err)
		}
		got := f.recv(t)
		gb, _ := json.Marshal(got)
		wb, _ := json.Marshal(s.want)
		if string(gb) != string(wb) {
			t.Errorf("command = %s, want %s", gb, wb)
		}
	}
}

func TestWindowEvents(t *testing.T) {
	p, f := startFake(t)
	go func() {
		f.emit(t, "\xef\xbb\xbf"+`{"action":"next","jiraUrl":"https://j","personalToken":"pat-ü","customCaCertPath":"/ca"}`)
		f.emit(t, "")
		f.emit(t, "not json")
		f.emit(t, `{"action":"save","projectKeys":["A","B"],"enableWriteTools":true}`)
		f.emit(t, `{"action":"choose","index":2}`)
	}()
	tests := []Event{
		{Action: ActionNext, JiraURL: "https://j", PersonalToken: "pat-ü", CustomCACertPath: "/ca"},
		{Action: ActionSave, ProjectKeys: []string{"A", "B"}, EnableWriteTools: true},
		{Action: ActionChoose, Index: 2},
	}
	for _, want := range tests {
		got := nextEvent(t, p)
		if got.Action != want.Action || got.JiraURL != want.JiraURL || got.PersonalToken != want.PersonalToken ||
			got.CustomCACertPath != want.CustomCACertPath || !slices.Equal(got.ProjectKeys, want.ProjectKeys) ||
			got.EnableWriteTools != want.EnableWriteTools || got.Index != want.Index {
			t.Errorf("event = %#v, want %#v", got, want)
		}
	}
}

func TestWindowUserClose(t *testing.T) {
	p, f := startFake(t)
	f.recv(t) // init
	go func() {
		f.emit(t, `{"action":"close"}`)
		f.stop(errors.New("exit status 1")) // even a failing exit is fine after close
	}()
	if ev := nextEvent(t, p); ev.Action != ActionClose {
		t.Fatalf("event = %#v", ev)
	}
	select {
	case <-p.Closed():
	case <-time.After(2 * time.Second):
		t.Fatal("Closed not signalled")
	}
	if ev := nextEvent(t, p); ev.Action != ActionClose {
		t.Errorf("event after close = %#v", ev)
	}
	if err := p.Status("late", false); err != nil {
		t.Errorf("Status after close = %v", err)
	}
	if err := p.Close(); err != nil {
		t.Error(err)
	}
}

func TestWindowProcessFailure(t *testing.T) {
	tests := []struct {
		name    string
		exit    error
		wantErr bool
	}{
		{"crash", errors.New("osascript: exit status 1: syntax error"), true},
		{"clean exit", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, f := startFake(t)
			f.recv(t)
			f.stop(tt.exit)
			ev, err := p.Next(context.Background())
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "syntax error") {
					t.Errorf("err = %v, event = %#v", err, ev)
				}
				if err := p.Status("x", false); err == nil {
					t.Error("send to a crashed window succeeded")
				}
			} else if err != nil || ev.Action != ActionClose {
				t.Errorf("event = %#v, err = %v", ev, err)
			}
			<-p.Closed()
		})
	}
}

func TestWindowCloseSendsQuit(t *testing.T) {
	p, f := startFake(t)
	f.recv(t)
	go func() {
		for c := range f.cmds {
			if c.Cmd == "quit" {
				f.stop(nil)
			}
		}
	}()
	done := make(chan struct{})
	go func() { _ = p.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return")
	}
	select {
	case <-p.Closed():
	default:
		t.Error("Closed not signalled after Close")
	}
}

func TestNextHonoursContext(t *testing.T) {
	p, _ := startFake(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Next(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestEncodePowerShell(t *testing.T) {
	in := "Write-Host 'ü €'\n"
	b, err := base64.StdEncoding.DecodeString(encodePowerShell(in))
	if err != nil {
		t.Fatal(err)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	if got := string(utf16.Decode(u)); got != in {
		t.Errorf("round trip = %q", got)
	}
}

// The PowerShell command line must stay below the Windows 32767-char limit,
// independent of the script size.
func TestPowerShellCommandLineIsShort(t *testing.T) {
	if n := len(encodePowerShell(psBootstrap)); n > 2000 {
		t.Errorf("encoded bootstrap is %d chars", n)
	}
}

func TestScriptsImplementProtocol(t *testing.T) {
	for name, s := range map[string]string{"macos_window.js": macWindow, "windows_window.ps1": winWindow} {
		for _, want := range []string{"'init'", "'connect'", "'projects'", "'register'", "'status'", "'copy'", "'quit'",
			"'next'", "'back'", "'save'", "'choose'", "'close'", "Close server"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s does not handle %s", name, want)
			}
		}
		if strings.Contains(strings.ToLower(s), "test connection") {
			t.Errorf("%s still has a Test connection button", name)
		}
	}
}
