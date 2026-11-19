package nativeui

import (
	"context"
	"encoding/base64"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/internal/jira-mcp/internal/settings"
)

func TestParseForm(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		want    FormResult
		wantErr bool
	}{
		{"save", `{"action":"save","jiraUrl":"https://j","personalToken":"p","projectKeys":"P","enableWriteTools":true,"customCaCertPath":"/ca"}` + "\n",
			FormResult{ActionSave, settings.Settings{JiraURL: "https://j", PersonalToken: "p", ProjectKeys: "P", EnableWriteTools: true, CustomCACertPath: "/ca"}}, false},
		{"cancel with BOM", "\xef\xbb\xbf" + `{"action":"cancel"}`, FormResult{Action: ActionCancel}, false},
		{"unknown action", `{"action":"delete"}`, FormResult{}, true},
		{"garbage", `oops`, FormResult{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseForm([]byte(tt.out))
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("got %#v, %v; want %#v, err=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestParseChoice(t *testing.T) {
	tests := []struct {
		out     string
		want    int
		wantErr bool
	}{
		{"0\n", 0, false}, {"2", 2, false}, {"-1", -1, false}, {"3", -1, false}, {"x", -1, true},
	}
	for _, tt := range tests {
		got, err := parseChoice([]byte(tt.out), 3)
		if got != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("parseChoice(%q) = %d, %v", tt.out, got, err)
		}
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

func TestDialogsNeverPassTokenToScript(t *testing.T) {
	var gotEnv []string
	d := &scriptDialogs{
		formScript: "form",
		run: func(_ context.Context, script string, env []string) ([]byte, error) {
			gotEnv = env
			return []byte(`{"action":"cancel"}`), nil
		},
		iconPNG: []byte{1, 2, 3},
	}
	req := FormRequest{Title: "T", Message: "M", Values: settings.Settings{JiraURL: "https://j", PersonalToken: "secret", EnableWriteTools: true}}
	if _, err := d.Form(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	for _, e := range gotEnv {
		if strings.Contains(e, "secret") {
			t.Errorf("token leaked into env: %q", e)
		}
	}
	for _, want := range []string{"JMCP_TITLE=T", "JMCP_URL=https://j", "JMCP_WRITE=1", "JMCP_ICON_PNG=AQID"} {
		if !slices.Contains(gotEnv, want) {
			t.Errorf("missing %q in %v", want, gotEnv)
		}
	}
}

func TestScriptsEmbedded(t *testing.T) {
	for name, s := range map[string]string{"macCommon": macCommon, "macForm": macForm, "macChoose": macChoose,
		"winCommon": winCommon, "winForm": winForm, "winChoose": winChoose, "winCopy": winCopy} {
		if strings.TrimSpace(s) == "" {
			t.Errorf("%s is empty", name)
		}
	}
}
