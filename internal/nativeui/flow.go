package nativeui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/hsnrzv/jira-mcp/internal/jira"
	"github.com/hsnrzv/jira-mcp/internal/settings"
	"github.com/hsnrzv/jira-mcp/internal/setupui"
)

// Title is the window title.
const Title = "Jira MCP Setup"

const (
	intro        = "Connect the local Jira Data Center MCP server. Everything stays on this machine."
	keepTokenTip = "leave blank to keep the current token"
	registerHelp = "Register the server with your IDE/agent. Install adds a \"jira\" entry to its config " +
		"(other servers are kept; a .bak backup is written). Copy puts a config snippet on the clipboard. " +
		"The config contains no secrets."
)

// Flow runs the setup wizard (connect → pick projects → save → register with
// IDEs) in one native window. It shares validation and install logic with the
// web UI.
type Flow struct {
	UI           Window
	ExePath      string
	Load         func() (settings.Settings, error)
	Save         func(settings.Settings) error
	Validate     func(context.Context, settings.Settings) (string, error)
	ListProjects func(context.Context, settings.Settings) (string, []jira.Project, error)
	Targets      func() ([]setupui.Target, error)

	values  settings.Settings // current form values, without the token
	token   string            // kept in memory only, never sent to the window
	targets []setupui.Target
}

// NewFlow returns a Flow wired to the real settings store, Jira and targets.
func NewFlow(ui Window, exePath string) *Flow {
	return &Flow{
		UI:           ui,
		ExePath:      exePath,
		Load:         settings.Load,
		Save:         settings.Save,
		Validate:     setupui.Validate,
		ListProjects: setupui.Projects,
		Targets:      setupui.Targets,
	}
}

// Run shows the wizard and handles user actions until the window is closed
// (window close button or "Close server"). A returned error means the native
// window itself failed.
func (f *Flow) Run(ctx context.Context) error {
	saved, err := f.Load()
	if err != nil {
		slog.Warn("could not load settings", "error", err)
	}
	f.token = saved.PersonalToken
	f.values = saved
	f.values.PersonalToken = ""
	if err := f.showConnect(); err != nil {
		return err
	}
	for {
		ev, err := f.UI.Next(ctx)
		if err != nil {
			return err
		}
		switch ev.Action {
		case ActionClose:
			return nil
		case ActionNext:
			err = f.connect(ctx, ev)
		case ActionBack:
			f.keepSelection(ev)
			err = f.showConnect()
		case ActionSave:
			f.keepSelection(ev)
			err = f.save(ctx)
		case ActionChoose:
			err = f.choose(ev.Index)
		default:
			slog.Warn("ignoring unknown setup window action", "action", string(ev.Action))
		}
		if err != nil {
			return err
		}
	}
}

// opContext is cancelled when the window closes, so "Close server" aborts a
// slow Jira call (for example through an unresponsive proxy) at once.
func (f *Flow) opContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-f.UI.Closed():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func (f *Flow) showConnect() error {
	hint := ""
	if f.token != "" {
		hint = keepTokenTip
	}
	return f.UI.ShowConnect(ConnectPage{
		Message: intro, JiraURL: f.values.JiraURL, CustomCACertPath: f.values.CustomCACertPath, TokenHint: hint,
	})
}

func (f *Flow) connect(ctx context.Context, ev Event) error {
	conn := setupui.NormalizeConnection(settings.Settings{
		JiraURL: ev.JiraURL, PersonalToken: ev.PersonalToken, CustomCACertPath: ev.CustomCACertPath,
	}, f.token)
	f.values.JiraURL, f.values.CustomCACertPath = conn.JiraURL, conn.CustomCACertPath
	f.token = conn.PersonalToken
	opCtx, cancel := f.opContext(ctx)
	defer cancel()
	user, projects, err := f.ListProjects(opCtx, conn)
	if err != nil {
		return f.UI.Status("Error: "+err.Error(), true)
	}
	have := map[string]bool{}
	for _, p := range projects {
		have[p.Key] = true
	}
	var selected []string
	for _, k := range setupui.SplitKeys(f.values.ProjectKeys) {
		if have[k] {
			selected = append(selected, k)
		}
	}
	return f.UI.ShowProjects(ProjectsPage{
		Message:          fmt.Sprintf("Connected to Jira as %s. Select the project(s) the MCP server may access (%d available).", user, len(projects)),
		Projects:         projects,
		Selected:         selected,
		EnableWriteTools: f.values.EnableWriteTools,
	})
}

func (f *Flow) keepSelection(ev Event) {
	f.values.ProjectKeys = strings.Join(ev.ProjectKeys, ",")
	f.values.EnableWriteTools = ev.EnableWriteTools
}

func (f *Flow) save(ctx context.Context) error {
	in, err := setupui.Normalize(f.values, f.token)
	if err != nil {
		return f.UI.Status("Error: "+err.Error(), true)
	}
	opCtx, cancel := f.opContext(ctx)
	defer cancel()
	info, err := f.Validate(opCtx, in)
	if err != nil {
		return f.UI.Status("Error: "+err.Error(), true)
	}
	if err := f.Save(in); err != nil {
		slog.Error("save settings failed", "error", err)
		return f.UI.Status("Error: could not save settings.", true)
	}
	f.targets, err = f.Targets()
	if err != nil {
		slog.Warn("could not determine MCP client configs", "error", err)
	}
	buttons := make([]string, 0, len(f.targets)+2)
	for _, t := range f.targets {
		buttons = append(buttons, "Install in "+t.Label)
	}
	buttons = append(buttons, "Copy VS Code config", "Copy JetBrains / Claude config")
	return f.UI.ShowRegister(RegisterPage{Message: registerHelp, Status: "Saved. " + info, Buttons: buttons})
}

// choose handles a step-3 button: install targets first, then the two copy
// buttons.
func (f *Flow) choose(i int) error {
	n := len(f.targets)
	switch {
	case i >= 0 && i < n:
		t := f.targets[i]
		if err := t.Install(f.ExePath); err != nil {
			return f.UI.Status("Error: "+err.Error(), true)
		}
		return f.UI.Status(fmt.Sprintf("Added \"jira\" to %s (%s). Restart your IDE/agent.", t.Label, t.Path), false)
	case i == n:
		return f.UI.Copy(setupui.VSCodeSnippet(f.ExePath), "Copied the VS Code / GitHub Copilot (mcp.json) config to the clipboard.")
	case i == n+1:
		return f.UI.Copy(setupui.MCPServersSnippet(f.ExePath), "Copied the JetBrains / Claude Desktop (mcpServers) config to the clipboard.")
	default:
		return f.UI.Status("Error: unknown option.", true)
	}
}
