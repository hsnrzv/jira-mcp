package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "settings.json")
	in := Settings{JiraURL: "https://j", PersonalToken: "tok", ProjectKeys: "A,B", EnableWriteTools: true}
	if err := SaveFile(p, in); err != nil {
		t.Fatal(err)
	}
	// Windows has no Unix permission bits; the user profile ACL protects %AppData% instead.
	if fi, _ := os.Stat(p); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
	out, err := LoadFile(p)
	if err != nil || out != in {
		t.Fatalf("got %v, %v", out, err)
	}
	if m, err := LoadFile(filepath.Join(t.TempDir(), "none.json")); err != nil || m.Complete() {
		t.Errorf("missing file should yield empty settings, got %v %v", m, err)
	}
}

func TestGetenvPrecedence(t *testing.T) {
	s := Settings{JiraURL: "https://saved", PersonalToken: "saved", ProjectKeys: "X", EnableWriteTools: true}
	env := map[string]string{"JIRA_URL": "https://env"}
	get := s.Getenv(func(k string) string { return env[k] })
	tests := []struct{ key, want string }{
		{"JIRA_URL", "https://env"},
		{"JIRA_PERSONAL_TOKEN", "saved"},
		{"JIRA_ALLOWED_PROJECTS", "X"},
		{"JIRA_ENABLE_WRITE_TOOLS", "true"},
	}
	for _, tt := range tests {
		if got := get(tt.key); got != tt.want {
			t.Errorf("%s = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestStringRedactsToken(t *testing.T) {
	s := Settings{PersonalToken: "supersecret"}
	if strings.Contains(s.String(), "supersecret") || strings.Contains(fmt.Sprintf("%#v", s), "supersecret") {
		t.Error("token leaked")
	}
}
