package nativeui

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/internal/jira-mcp/internal/settings"
	"github.com/internal/jira-mcp/internal/setupui"
)

const (
	title = "Jira MCP Setup"
	intro = "Connect the local Jira Data Center MCP server. Everything stays on this machine."
)

// Flow runs the setup wizard (form → test/save → register with IDEs) using
// native dialogs. It shares validation and install logic with the web UI.
type Flow struct {
	UI       Dialogs
	ExePath  string
	Load     func() (settings.Settings, error)
	Save     func(settings.Settings) error
	Validate func(context.Context, settings.Settings) (string, error)
	Targets  func() ([]setupui.Target, error)
}

// NewFlow returns a Flow wired to the real settings store, Jira and targets.
func NewFlow(ui Dialogs, exePath string) *Flow {
	return &Flow{
		UI:       ui,
		ExePath:  exePath,
		Load:     settings.Load,
		Save:     settings.Save,
		Validate: setupui.Validate,
		Targets:  setupui.Targets,
	}
}

// Run shows the dialogs until the user saves and closes, or cancels. A
// returned error means the native dialog itself failed.
func (f *Flow) Run(ctx context.Context) error {
	saved, err := f.Load()
	if err != nil {
		slog.Warn("could not load settings", "error", err)
	}
	// token is kept in memory only: the dialog shows a blank password field and
	// a blank submission keeps the saved (or previously entered) token.
	token := saved.PersonalToken
	values := saved
	values.PersonalToken = ""
	msg := intro
	for {
		hint := ""
		if token != "" {
			hint = "leave blank to keep the current token"
		}
		res, err := f.UI.Form(ctx, FormRequest{Title: title, Message: msg, TokenHint: hint, Values: values})
		if err != nil {
			return err
		}
		if res.Action == ActionCancel {
			return nil
		}
		in, nerr := setupui.Normalize(res.Values, token)
		token = in.PersonalToken
		values = in
		values.PersonalToken = ""
		if nerr != nil {
			msg = "Error: " + nerr.Error()
			continue
		}
		info, err := f.Validate(ctx, in)
		if err != nil {
			msg = "Error: " + err.Error()
			continue
		}
		if res.Action == ActionTest {
			msg = "Connection OK. " + info + "\nClick Save to store the settings."
			continue
		}
		if err := f.Save(in); err != nil {
			slog.Error("save settings failed", "error", err)
			msg = "Error: could not save settings."
			continue
		}
		return f.register(ctx, "Saved. "+info)
	}
}

// register offers one-click install into IDE configs and clipboard snippets.
func (f *Flow) register(ctx context.Context, status string) error {
	targets, err := f.Targets()
	if err != nil {
		slog.Warn("could not determine MCP client configs", "error", err)
	}
	buttons := []string{"Done"}
	for _, t := range targets {
		buttons = append(buttons, "Install in "+t.Label)
	}
	copyVSCode, copyMCPServers := len(buttons), len(buttons)+1
	buttons = append(buttons, "Copy VS Code config", "Copy JetBrains / Claude config")
	const help = "\n\nRegister the server with your IDE/agent. Install adds a \"jira\" entry to its config " +
		"(other servers are kept; a .bak backup is written). Copy puts a config snippet on the clipboard. " +
		"The config contains no secrets."
	for {
		i, err := f.UI.Choose(ctx, title, status+help, buttons)
		if err != nil {
			return err
		}
		switch {
		case i <= 0:
			return nil
		case i == copyVSCode:
			status = f.copy(ctx, "VS Code / GitHub Copilot (mcp.json)", setupui.VSCodeSnippet(f.ExePath))
		case i == copyMCPServers:
			status = f.copy(ctx, "JetBrains / Claude Desktop (mcpServers)", setupui.MCPServersSnippet(f.ExePath))
		default:
			t := targets[i-1]
			if err := t.Install(f.ExePath); err != nil {
				status = "Error: " + err.Error()
			} else {
				status = fmt.Sprintf("Added \"jira\" to %s (%s). Restart your IDE/agent.", t.Label, t.Path)
			}
		}
	}
}

func (f *Flow) copy(ctx context.Context, label, text string) string {
	if err := f.UI.Copy(ctx, text); err != nil {
		slog.Warn("clipboard copy failed", "error", err)
		return "Error: could not copy to the clipboard. Run \"" + f.ExePath + " setup --web\" to see the snippet."
	}
	return "Copied the " + label + " config to the clipboard."
}
