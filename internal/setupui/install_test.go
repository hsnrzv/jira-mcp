package setupui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallMergesAndBacksUp(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d", "mcp.json")
	tg := Target{Path: p, Root: "servers", Entry: func(exe string) map[string]any {
		return map[string]any{"command": exe}
	}}
	if err := tg.Install("/x/app"); err != nil { // creates file
		t.Fatal(err)
	}
	orig := `{"servers":{"figma":{"url":"u"},"jira":{"command":"docker"}},"other":1}`
	os.WriteFile(p, []byte(orig), 0o600)
	if err := tg.Install("/x/app"); err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]any
	b, _ := os.ReadFile(p)
	json.Unmarshal(b, &got)
	if got["servers"]["figma"] == nil || got["servers"]["jira"].(map[string]any)["command"] != "/x/app" {
		t.Errorf("bad merge: %s", b)
	}
	if bak, _ := os.ReadFile(p + ".bak"); string(bak) != orig {
		t.Errorf("backup = %s", bak)
	}
	os.WriteFile(p, []byte("// c\n{}"), 0o600)
	if err := tg.Install("/x/app"); err == nil {
		t.Error("expected error for non-JSON file")
	}
}
