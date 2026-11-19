// Package settings persists the user's connection settings (entered through the
// setup UI) in the per-user config directory so the MCP server can start
// without any environment variables.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	dirName  = "jira-mcp"
	fileName = "settings.json"
)

// Settings are the values collected by the setup form.
type Settings struct {
	JiraURL          string `json:"jiraUrl"`
	PersonalToken    string `json:"personalToken"`
	ProjectKeys      string `json:"projectKeys"`
	EnableWriteTools bool   `json:"enableWriteTools"`
	CustomCACertPath string `json:"customCaCertPath,omitempty"`
}

// String redacts the token so the struct is safe to log accidentally.
func (s Settings) String() string {
	tok := ""
	if s.PersonalToken != "" {
		tok = "[REDACTED]"
	}
	return fmt.Sprintf("Settings{JiraURL:%s PersonalToken:%s ProjectKeys:%s EnableWriteTools:%t}",
		s.JiraURL, tok, s.ProjectKeys, s.EnableWriteTools)
}

// GoString prevents %#v from leaking the token.
func (s Settings) GoString() string { return s.String() }

// Path returns the settings file location.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config dir: %w", err)
	}
	return filepath.Join(dir, dirName, fileName), nil
}

// Load reads the saved settings. A missing file yields empty settings.
func Load() (Settings, error) {
	p, err := Path()
	if err != nil {
		return Settings{}, err
	}
	return LoadFile(p)
}

// LoadFile reads settings from an explicit path.
func LoadFile(path string) (Settings, error) {
	var s Settings
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("read settings: %w", err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return Settings{}, fmt.Errorf("parse settings %s: %w", path, err)
	}
	return s, nil
}

// Save writes the settings with owner-only permissions.
func Save(s Settings) error {
	p, err := Path()
	if err != nil {
		return err
	}
	return SaveFile(p, s)
}

// SaveFile writes settings to an explicit path (atomically, mode 0600).
func SaveFile(path string, s Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create settings dir: %w", err)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return fmt.Errorf("create temp settings: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Complete reports whether the required fields are present.
func (s Settings) Complete() bool {
	return strings.TrimSpace(s.JiraURL) != "" && strings.TrimSpace(s.PersonalToken) != ""
}

// Getenv returns a lookup function where real environment variables win and
// saved settings fill in whatever is not set.
func (s Settings) Getenv(env func(string) string) func(string) string {
	saved := map[string]string{
		"JIRA_URL":                 s.JiraURL,
		"JIRA_PERSONAL_TOKEN":      s.PersonalToken,
		"JIRA_ALLOWED_PROJECTS":    s.ProjectKeys,
		"JIRA_CUSTOM_CA_CERT_PATH": s.CustomCACertPath,
	}
	if s.EnableWriteTools {
		saved["JIRA_ENABLE_WRITE_TOOLS"] = "true"
	}
	return func(k string) string {
		if v := env(k); strings.TrimSpace(v) != "" {
			return v
		}
		return saved[k]
	}
}
