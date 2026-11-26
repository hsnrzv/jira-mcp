package nativeui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hsnrzv/jira-mcp/internal/jira"
	"github.com/hsnrzv/jira-mcp/internal/settings"
	"github.com/hsnrzv/jira-mcp/internal/setupui"
)

// fakeWindow replays scripted events and records what the flow showed.
type fakeWindow struct {
	events   []Event
	calls    []string // "connect", "projects", "register", "status", "copy"
	connects []ConnectPage
	projects []ProjectsPage
	register []RegisterPage
	statuses []string // "E:"/"OK:" + text
	copied   []string
	closed   chan struct{}
	nextErr  error
}

func newFakeWindow(events ...Event) *fakeWindow {
	return &fakeWindow{events: events, closed: make(chan struct{})}
}

func (w *fakeWindow) ShowConnect(p ConnectPage) error {
	w.calls = append(w.calls, "connect")
	w.connects = append(w.connects, p)
	return nil
}

func (w *fakeWindow) ShowProjects(p ProjectsPage) error {
	w.calls = append(w.calls, "projects")
	w.projects = append(w.projects, p)
	return nil
}

func (w *fakeWindow) ShowRegister(p RegisterPage) error {
	w.calls = append(w.calls, "register")
	w.register = append(w.register, p)
	return nil
}

func (w *fakeWindow) Status(text string, isErr bool) error {
	w.calls = append(w.calls, "status")
	if isErr {
		w.statuses = append(w.statuses, "E:"+text)
	} else {
		w.statuses = append(w.statuses, "OK:"+text)
	}
	return nil
}

func (w *fakeWindow) Copy(text, message string) error {
	w.calls = append(w.calls, "copy")
	w.copied = append(w.copied, text)
	w.statuses = append(w.statuses, "OK:"+message)
	return nil
}

func (w *fakeWindow) Next(context.Context) (Event, error) {
	if w.nextErr != nil {
		return Event{}, w.nextErr
	}
	if len(w.events) == 0 {
		return Event{Action: ActionClose}, nil
	}
	ev := w.events[0]
	w.events = w.events[1:]
	return ev, nil
}

func (w *fakeWindow) Closed() <-chan struct{} { return w.closed }
func (w *fakeWindow) Close() error            { return nil }

var testProjects = []jira.Project{{Key: "CORE", Name: "Core"}, {Key: "PLAT", Name: "Platform"}}

func newFlow(ui Window, stored *settings.Settings, saves *int, target string) *Flow {
	return &Flow{
		UI:      ui,
		ExePath: "/bin/jira-mcp",
		Load:    func() (settings.Settings, error) { return *stored, nil },
		Save:    func(s settings.Settings) error { *stored = s; *saves++; return nil },
		Validate: func(_ context.Context, s settings.Settings) (string, error) {
			if s.ProjectKeys == "CORE" {
				return "", errors.New("project CORE is archived")
			}
			return "Connected.", nil
		},
		ListProjects: func(_ context.Context, s settings.Settings) (string, []jira.Project, error) {
			if s.PersonalToken == "" {
				return "", nil, errors.New("personal access token is required")
			}
			if s.JiraURL == "https://bad" {
				return "", nil, errors.New("could not authenticate")
			}
			return "Ann", testProjects, nil
		},
		Targets: func() ([]setupui.Target, error) {
			return []setupui.Target{{
				ID: "t", Label: "Test IDE", Path: target, Root: "servers",
				Entry: func(exe string) map[string]any { return map[string]any{"command": exe} },
			}}, nil
		},
	}
}

func next(url, tok string) Event { return Event{Action: ActionNext, JiraURL: url, PersonalToken: tok} }

func pick(a Action, write bool, keys ...string) Event {
	return Event{Action: a, ProjectKeys: keys, EnableWriteTools: write}
}

func choose(i int) Event { return Event{Action: ActionChoose, Index: i} }

func TestFlow(t *testing.T) {
	tests := []struct {
		name         string
		stored       settings.Settings
		events       []Event
		wantCalls    []string
		wantStatuses []string // substrings, in order
		wantSaves    int
		wantStored   settings.Settings
		wantSelected [][]string // Selected of each projects page
		wantCopied   int
		wantInstall  bool
	}{
		{
			name:      "close on first page",
			wantCalls: []string{"connect"},
		},
		{
			name:         "connection error is shown inline without re-rendering",
			events:       []Event{next("https://bad", "tok"), next("https://jira", "")},
			wantCalls:    []string{"connect", "status", "projects"},
			wantStatuses: []string{"E:Error: could not authenticate"},
			wantSelected: [][]string{nil},
		},
		{
			name:         "missing token",
			events:       []Event{next("https://jira", "")},
			wantCalls:    []string{"connect", "status"},
			wantStatuses: []string{"E:Error: personal access token is required"},
		},
		{
			name:   "save, install, copy both snippets",
			stored: settings.Settings{ProjectKeys: "PLAT,GONE"},
			events: []Event{
				next(" https://jira ", " tok "),
				pick(ActionSave, true, "CORE", "PLAT"),
				choose(0), choose(1), choose(2),
			},
			wantCalls:    []string{"connect", "projects", "register", "status", "copy", "copy"},
			wantStatuses: []string{"OK:Added \"jira\" to Test IDE", "OK:Copied the VS Code", "OK:Copied the JetBrains"},
			wantSaves:    1,
			wantStored:   settings.Settings{JiraURL: "https://jira", PersonalToken: "tok", ProjectKeys: "CORE,PLAT", EnableWriteTools: true},
			wantSelected: [][]string{{"PLAT"}},
			wantCopied:   2,
			wantInstall:  true,
		},
		{
			name:   "validation and selection errors keep the page",
			stored: settings.Settings{PersonalToken: "saved"},
			events: []Event{
				next("https://jira", ""),
				pick(ActionSave, false),
				pick(ActionSave, false, "CORE"),
				pick(ActionSave, false, "PLAT"),
			},
			wantCalls:    []string{"connect", "projects", "status", "status", "register"},
			wantStatuses: []string{"E:Error: select at least one project", "E:Error: project CORE is archived"},
			wantSaves:    1,
			wantStored:   settings.Settings{JiraURL: "https://jira", PersonalToken: "saved", ProjectKeys: "PLAT"},
			wantSelected: [][]string{nil},
		},
		{
			name: "back keeps selection and token",
			events: []Event{
				next("https://jira", "tok"),
				pick(ActionBack, true, "CORE"),
				next("https://jira", ""),
			},
			wantCalls:    []string{"connect", "projects", "connect", "projects"},
			wantSelected: [][]string{nil, {"CORE"}},
		},
		{
			name:         "unknown choice",
			events:       []Event{next("https://jira", "tok"), pick(ActionSave, false, "PLAT"), choose(9)},
			wantCalls:    []string{"connect", "projects", "register", "status"},
			wantStatuses: []string{"E:Error: unknown option."},
			wantSaves:    1,
			wantStored:   settings.Settings{JiraURL: "https://jira", PersonalToken: "tok", ProjectKeys: "PLAT"},
			wantSelected: [][]string{nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "mcp.json")
			ui := newFakeWindow(tt.events...)
			stored, saves := tt.stored, 0
			if err := newFlow(ui, &stored, &saves, target).Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(ui.calls, tt.wantCalls) {
				t.Errorf("calls = %v, want %v", ui.calls, tt.wantCalls)
			}
			if len(ui.statuses) != len(tt.wantStatuses) {
				t.Errorf("statuses = %q, want %q", ui.statuses, tt.wantStatuses)
			} else {
				for i, want := range tt.wantStatuses {
					if !strings.HasPrefix(ui.statuses[i], want) {
						t.Errorf("status %d = %q, want prefix %q", i, ui.statuses[i], want)
					}
				}
			}
			if saves != tt.wantSaves {
				t.Errorf("saves = %d, want %d", saves, tt.wantSaves)
			}
			if tt.wantSaves > 0 && stored != tt.wantStored {
				t.Errorf("stored = %#v, want %#v", stored, tt.wantStored)
			}
			if len(ui.projects) != len(tt.wantSelected) {
				t.Errorf("projects pages = %d, want %d", len(ui.projects), len(tt.wantSelected))
			} else {
				for i, want := range tt.wantSelected {
					if !slices.Equal(ui.projects[i].Selected, want) {
						t.Errorf("page %d selected = %v, want %v", i, ui.projects[i].Selected, want)
					}
				}
			}
			if len(ui.copied) != tt.wantCopied {
				t.Errorf("copied %d snippets, want %d", len(ui.copied), tt.wantCopied)
			}
			for _, c := range ui.copied {
				if !strings.Contains(c, "/bin/jira-mcp") || strings.Contains(c, "tok") {
					t.Errorf("snippet = %s", c)
				}
			}
			_, err := os.Stat(target)
			if installed := err == nil; installed != tt.wantInstall {
				t.Errorf("installed = %v, want %v", installed, tt.wantInstall)
			}
			for _, r := range ui.register {
				if !strings.HasPrefix(r.Status, "Saved. Connected.") ||
					!slices.Equal(r.Buttons, []string{"Install in Test IDE", "Copy VS Code config", "Copy JetBrains / Claude config"}) {
					t.Errorf("register page = %#v", r)
				}
			}
		})
	}
}

func TestFlowTokenHintAndPages(t *testing.T) {
	ui := newFakeWindow(next("https://jira", ""), pick(ActionBack, false))
	stored := settings.Settings{JiraURL: "https://jira", PersonalToken: "saved", CustomCACertPath: "/ca"}
	saves := 0
	if err := newFlow(ui, &stored, &saves, "").Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c := ui.connects[0]; c.JiraURL != "https://jira" || c.CustomCACertPath != "/ca" || c.Message != intro {
		t.Errorf("connect page = %#v", c)
	}
	for _, c := range ui.connects {
		if c.TokenHint != keepTokenTip {
			t.Errorf("token hint = %q", c.TokenHint)
		}
	}
	if len(ui.projects) != 1 || !strings.Contains(ui.projects[0].Message, "Connected to Jira as Ann") ||
		len(ui.projects[0].Projects) != 2 {
		t.Errorf("projects pages = %#v", ui.projects)
	}

	ui = newFakeWindow()
	stored = settings.Settings{}
	if err := newFlow(ui, &stored, &saves, "").Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ui.connects[0].TokenHint != "" {
		t.Errorf("hint without token = %q", ui.connects[0].TokenHint)
	}
}

func TestFlowWindowErrorIsReturned(t *testing.T) {
	ui := newFakeWindow()
	ui.nextErr = errors.New("boom")
	stored, saves := settings.Settings{}, 0
	if err := newFlow(ui, &stored, &saves, "").Run(context.Background()); err == nil || err.Error() != "boom" {
		t.Errorf("err = %v", err)
	}
}

// Closing the window must cancel a Jira call that hangs (e.g. on a proxy).
func TestFlowCloseCancelsSlowRequest(t *testing.T) {
	ui := newFakeWindow(next("https://jira", "tok"))
	stored, saves := settings.Settings{}, 0
	f := newFlow(ui, &stored, &saves, "")
	f.ListProjects = func(ctx context.Context, _ settings.Settings) (string, []jira.Project, error) {
		close(ui.closed)
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return "", nil, errors.New("not cancelled")
		}
	}
	if err := f.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(ui.statuses) != 1 || !strings.Contains(ui.statuses[0], context.Canceled.Error()) {
		t.Errorf("statuses = %q", ui.statuses)
	}
}
