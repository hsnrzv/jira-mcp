package setupui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Target describes an MCP client config file the app can register itself in.
type Target struct {
	ID    string
	Label string
	Path  string
	Root  string
	Entry func(exe string) map[string]any
}

// Targets returns the supported client configs for the current user.
func Targets() ([]Target, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	intellij := filepath.Join(home, ".config", "github-copilot", "intellij", "mcp.json")
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			intellij = filepath.Join(la, "github-copilot", "intellij", "mcp.json")
		}
	}
	return []Target{
		{
			ID: "copilot-intellij", Label: "GitHub Copilot (IntelliJ)", Path: intellij, Root: "servers",
			Entry: func(exe string) map[string]any {
				return map[string]any{"type": "stdio", "command": exe, "args": []string{"serve"}}
			},
		},
		{
			ID: "copilot-air", Label: "Copilot CLI / Air plugin", Path: filepath.Join(home, ".copilot", "mcp-config.json"), Root: "mcpServers",
			Entry: func(exe string) map[string]any {
				return map[string]any{"type": "stdio", "command": exe, "args": []string{"serve"}, "tools": []string{"*"}}
			},
		},
	}, nil
}

// Install merges a "jira" server entry into the target file, keeping all other
// servers, and backs up an existing file to <path>.bak first.
func (t Target) Install(exe string) error {
	doc := map[string]any{}
	mode := os.FileMode(0o600)
	b, err := os.ReadFile(t.Path)
	switch {
	case err == nil:
		if fi, e := os.Stat(t.Path); e == nil {
			mode = fi.Mode().Perm()
		}
		if len(b) > 0 {
			if err := json.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("%s is not plain JSON (comments?): add the snippet manually: %w", t.Path, err)
			}
		}
		if err := os.WriteFile(t.Path+".bak", b, 0o600); err != nil {
			return fmt.Errorf("backup %s: %w", t.Path, err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	servers, ok := doc[t.Root].(map[string]any)
	if !ok {
		if doc[t.Root] != nil {
			return fmt.Errorf("%s: %q is not an object", t.Path, t.Root)
		}
		servers = map[string]any{}
	}
	servers["jira"] = t.Entry(exe)
	doc[t.Root] = servers
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(t.Path), 0o755); err != nil {
		return err
	}
	tmp := t.Path + ".tmp"
	if err := os.WriteFile(tmp, out, mode); err != nil {
		return err
	}
	return os.Rename(tmp, t.Path)
}
