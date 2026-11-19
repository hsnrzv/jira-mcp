package nativeui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/internal/jira-mcp/internal/settings"
	"github.com/internal/jira-mcp/internal/setupui"
)

type fakeUI struct {
	forms    []FormResult
	choices  []int
	requests []FormRequest
	messages []string
	copied   []string
	formErr  error
}

func (u *fakeUI) Form(_ context.Context, req FormRequest) (FormResult, error) {
	u.requests = append(u.requests, req)
	if u.formErr != nil {
		return FormResult{}, u.formErr
	}
	if len(u.forms) == 0 {
		return FormResult{Action: ActionCancel}, nil
	}
	r := u.forms[0]
	u.forms = u.forms[1:]
	return r, nil
}

func (u *fakeUI) Choose(_ context.Context, _, message string, _ []string) (int, error) {
	u.messages = append(u.messages, message)
	if len(u.choices) == 0 {
		return 0, nil
	}
	c := u.choices[0]
	u.choices = u.choices[1:]
	return c, nil
}

func (u *fakeUI) Copy(_ context.Context, text string) error {
	u.copied = append(u.copied, text)
	return nil
}

func newFlow(ui *fakeUI, stored *settings.Settings, saves *int, target string) *Flow {
	return &Flow{
		UI:      ui,
		ExePath: "/bin/jira-mcp",
		Load:    func() (settings.Settings, error) { return *stored, nil },
		Save:    func(s settings.Settings) error { *stored = s; *saves++; return nil },
		Validate: func(_ context.Context, s settings.Settings) (string, error) {
			if s.PersonalToken == "" {
				return "", errors.New("personal access token is required")
			}
			if s.JiraURL == "https://bad" {
				return "", errors.New("could not authenticate")
			}
			return "Connected.", nil
		},
		Targets: func() ([]setupui.Target, error) {
			return []setupui.Target{{
				ID: "t", Label: "Test IDE", Path: target, Root: "servers",
				Entry: func(exe string) map[string]any { return map[string]any{"command": exe} },
			}}, nil
		},
	}
}

func form(a Action, url, tok, keys string) FormResult {
	return FormResult{Action: a, Values: settings.Settings{JiraURL: url, PersonalToken: tok, ProjectKeys: keys}}
}

func TestFlow(t *testing.T) {
	tests := []struct {
		name        string
		stored      settings.Settings
		forms       []FormResult
		choices     []int
		wantSaves   int
		wantStored  settings.Settings
		wantMsgs    []string // substrings of the form messages after the first
		wantCopied  int
		wantInstall bool
	}{
		{
			name:  "cancel saves nothing",
			forms: []FormResult{form(ActionCancel, "https://j", "x", "P")},
		},
		{
			name:       "test then save keeps typed token and normalizes",
			forms:      []FormResult{form(ActionTest, " https://j ", "newtok", "plat, core"), form(ActionSave, "https://j", "", "PLAT,CORE")},
			wantSaves:  1,
			wantStored: settings.Settings{JiraURL: "https://j", PersonalToken: "newtok", ProjectKeys: "PLAT,CORE"},
			wantMsgs:   []string{"Connection OK. Connected."},
		},
		{
			name:       "blank token keeps saved token",
			stored:     settings.Settings{JiraURL: "https://old", PersonalToken: "saved", ProjectKeys: "OLD"},
			forms:      []FormResult{form(ActionSave, "https://j", "", "P")},
			wantSaves:  1,
			wantStored: settings.Settings{JiraURL: "https://j", PersonalToken: "saved", ProjectKeys: "P"},
		},
		{
			name:       "errors are shown and the form is re-opened",
			forms:      []FormResult{form(ActionSave, "https://j", "", "P"), form(ActionSave, "https://j", "t", ""), form(ActionSave, "https://bad", "t", "P"), form(ActionSave, "https://j", "", "P")},
			wantSaves:  1,
			wantStored: settings.Settings{JiraURL: "https://j", PersonalToken: "t", ProjectKeys: "P"},
			wantMsgs:   []string{"personal access token is required", "project key is required", "could not authenticate"},
		},
		{
			name:        "register: install and copy",
			forms:       []FormResult{form(ActionSave, "https://j", "t", "P")},
			choices:     []int{1, 2, 3, -1},
			wantSaves:   1,
			wantStored:  settings.Settings{JiraURL: "https://j", PersonalToken: "t", ProjectKeys: "P"},
			wantCopied:  2,
			wantInstall: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stored, saves := tt.stored, 0
			target := filepath.Join(t.TempDir(), "mcp.json")
			ui := &fakeUI{forms: tt.forms, choices: tt.choices}
			if err := newFlow(ui, &stored, &saves, target).Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			if saves != tt.wantSaves {
				t.Errorf("saves = %d, want %d", saves, tt.wantSaves)
			}
			if tt.wantSaves > 0 && stored != tt.wantStored {
				t.Errorf("stored = %#v, want %#v", stored, tt.wantStored)
			}
			for i, req := range ui.requests {
				if req.Values.PersonalToken != "" {
					t.Errorf("request %d passed the token to the dialog", i)
				}
			}
			for i, want := range tt.wantMsgs {
				if i+1 >= len(ui.requests) || !strings.Contains(ui.requests[i+1].Message, want) {
					t.Errorf("form message %d: want %q in %+v", i+1, want, ui.requests)
				}
			}
			if len(ui.copied) != tt.wantCopied {
				t.Errorf("copied %d snippets, want %d", len(ui.copied), tt.wantCopied)
			}
			b, err := os.ReadFile(target)
			if tt.wantInstall != (err == nil) {
				t.Errorf("installed = %v, want %v", err == nil, tt.wantInstall)
			}
			if tt.wantInstall && !strings.Contains(string(b), `"jira"`) {
				t.Errorf("target file: %s", b)
			}
			if tt.wantInstall && !strings.Contains(ui.messages[1], "Added \"jira\" to Test IDE") {
				t.Errorf("install message: %q", ui.messages[1])
			}
		})
	}
}

func TestFlowTokenHint(t *testing.T) {
	stored, saves := settings.Settings{PersonalToken: "saved"}, 0
	ui := &fakeUI{}
	_ = newFlow(ui, &stored, &saves, "").Run(context.Background())
	if ui.requests[0].TokenHint == "" {
		t.Error("expected a token hint when a token is saved")
	}
}

func TestFlowDialogErrorIsReturned(t *testing.T) {
	stored, saves := settings.Settings{}, 0
	boom := errors.New("osascript failed")
	ui := &fakeUI{formErr: boom}
	if err := newFlow(ui, &stored, &saves, "").Run(context.Background()); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}
