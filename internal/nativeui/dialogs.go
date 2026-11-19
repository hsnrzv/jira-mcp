// Package nativeui shows the setup form as a native OS modal dialog instead of
// a browser page. It stays CGO-free (so release builds still cross-compile) by
// driving the dialog toolkits that ship with the OS: Cocoa via
// `osascript -l JavaScript` on macOS and WinForms via Windows PowerShell on
// Windows. Other platforms return ErrUnsupported and use the browser form.
package nativeui

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/internal/jira-mcp/internal/settings"
)

// ErrUnsupported is returned by New on platforms without a native backend.
var ErrUnsupported = errors.New("native dialogs are not supported on this platform")

// Action is the button the user pressed in the settings form.
type Action string

const (
	ActionSave   Action = "save"
	ActionTest   Action = "test"
	ActionCancel Action = "cancel"
)

// FormRequest describes the settings form to show.
type FormRequest struct {
	Title     string
	Message   string
	TokenHint string
	// Values pre-fills the form. PersonalToken is never passed to the dialog.
	Values settings.Settings
}

// FormResult is the submitted form.
type FormResult struct {
	Action Action
	Values settings.Settings
}

// Dialogs abstracts the native toolkit so the setup flow can be tested.
type Dialogs interface {
	Form(ctx context.Context, req FormRequest) (FormResult, error)
	// Choose shows message with buttons and returns the pressed index, or -1
	// when the dialog was closed without a choice.
	Choose(ctx context.Context, title, message string, buttons []string) (int, error)
	Copy(ctx context.Context, text string) error
}

var (
	//go:embed scripts/macos_common.js
	macCommon string
	//go:embed scripts/macos_form.js
	macForm string
	//go:embed scripts/macos_choose.js
	macChoose string
	//go:embed scripts/windows_common.ps1
	winCommon string
	//go:embed scripts/windows_form.ps1
	winForm string
	//go:embed scripts/windows_choose.ps1
	winChoose string
	//go:embed scripts/windows_copy.ps1
	winCopy string
)

// runner executes a dialog script with extra environment variables and
// returns its stdout.
type runner func(ctx context.Context, script string, env []string) ([]byte, error)

type scriptDialogs struct {
	formScript, chooseScript string
	run                      runner
	copy                     func(ctx context.Context, text string) error
	iconPNG                  []byte
}

// New returns the native dialog backend for the current OS. iconPNG is shown
// in the dialogs (optional).
func New(iconPNG []byte) (Dialogs, error) {
	d := &scriptDialogs{iconPNG: iconPNG}
	switch runtime.GOOS {
	case "darwin":
		d.formScript = macCommon + "\n" + macForm
		d.chooseScript = macCommon + "\n" + macChoose
		d.run = runOSAScript
		d.copy = copyPbcopy
	case "windows":
		d.formScript = winCommon + "\n" + winForm
		d.chooseScript = winCommon + "\n" + winChoose
		d.run = runPowerShell
		d.copy = func(ctx context.Context, text string) error {
			_, err := runPowerShell(ctx, winCopy, []string{"JMCP_CLIP=" + text})
			return err
		}
	default:
		return nil, ErrUnsupported
	}
	return d, nil
}

func (d *scriptDialogs) baseEnv(title, message string) []string {
	env := []string{"JMCP_TITLE=" + title, "JMCP_MESSAGE=" + message}
	if len(d.iconPNG) > 0 {
		env = append(env, "JMCP_ICON_PNG="+base64.StdEncoding.EncodeToString(d.iconPNG))
	}
	return env
}

func formEnv(base []string, req FormRequest) []string {
	write := ""
	if req.Values.EnableWriteTools {
		write = "1"
	}
	return append(base,
		"JMCP_URL="+req.Values.JiraURL,
		"JMCP_KEYS="+req.Values.ProjectKeys,
		"JMCP_WRITE="+write,
		"JMCP_CA="+req.Values.CustomCACertPath,
		"JMCP_TOKEN_HINT="+req.TokenHint,
	)
}

func (d *scriptDialogs) Form(ctx context.Context, req FormRequest) (FormResult, error) {
	out, err := d.run(ctx, d.formScript, formEnv(d.baseEnv(req.Title, req.Message), req))
	if err != nil {
		return FormResult{}, err
	}
	return parseForm(out)
}

func (d *scriptDialogs) Choose(ctx context.Context, title, message string, buttons []string) (int, error) {
	env := append(d.baseEnv(title, message), "JMCP_BUTTONS="+strings.Join(buttons, "\n"))
	out, err := d.run(ctx, d.chooseScript, env)
	if err != nil {
		return -1, err
	}
	return parseChoice(out, len(buttons))
}

func (d *scriptDialogs) Copy(ctx context.Context, text string) error {
	return d.copy(ctx, text)
}

func trimOutput(out []byte) []byte {
	return bytes.TrimSpace(bytes.TrimPrefix(out, []byte("\xef\xbb\xbf")))
}

func parseForm(out []byte) (FormResult, error) {
	var raw struct {
		Action Action `json:"action"`
		settings.Settings
	}
	if err := json.Unmarshal(trimOutput(out), &raw); err != nil {
		return FormResult{}, fmt.Errorf("unexpected dialog output: %w", err)
	}
	switch raw.Action {
	case ActionSave, ActionTest, ActionCancel:
	default:
		return FormResult{}, fmt.Errorf("unexpected dialog action %q", raw.Action)
	}
	return FormResult{Action: raw.Action, Values: raw.Settings}, nil
}

func parseChoice(out []byte, n int) (int, error) {
	i, err := strconv.Atoi(string(trimOutput(out)))
	if err != nil {
		return -1, fmt.Errorf("unexpected dialog output: %w", err)
	}
	if i < 0 || i >= n {
		return -1, nil
	}
	return i, nil
}

func runCmd(cmd *exec.Cmd, env []string) ([]byte, error) {
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return nil, fmt.Errorf("%s: %w: %s", filepath.Base(cmd.Path), err, msg)
	}
	return out, nil
}

// runOSAScript feeds the script on stdin so it does not appear in argv.
func runOSAScript(ctx context.Context, script string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-")
	cmd.Stdin = strings.NewReader(script)
	return runCmd(cmd, env)
}

func copyPbcopy(ctx context.Context, text string) error {
	cmd := exec.CommandContext(ctx, "/usr/bin/pbcopy")
	cmd.Stdin = strings.NewReader(text)
	_, err := runCmd(cmd, nil)
	return err
}

func powerShellPath() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	}
	return "powershell.exe"
}

func runPowerShell(ctx context.Context, script string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, powerShellPath(),
		"-NoProfile", "-NonInteractive", "-STA", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", encodePowerShell(script))
	return runCmd(cmd, env)
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
