package config

import (
	"fmt"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadFrom(t *testing.T) {
	base := map[string]string{"JIRA_URL": "https://jira.example.com//", "JIRA_PERSONAL_TOKEN": "s3cret"}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for kk, vv := range base {
			m[kk] = vv
		}
		m[k] = v
		return m
	}
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"valid defaults", base, ""},
		{"missing url", map[string]string{"JIRA_PERSONAL_TOKEN": "x"}, "JIRA_URL is required"},
		{"missing token", map[string]string{"JIRA_URL": "https://j"}, "JIRA_PERSONAL_TOKEN is required"},
		{"bad scheme", with("JIRA_URL", "ftp://j"), "JIRA_URL must be"},
		{"bad bool", with("JIRA_ENABLE_WRITE_TOOLS", "yes"), "JIRA_ENABLE_WRITE_TOOLS"},
		{"bad timeout", with("JIRA_HTTP_TIMEOUT_SECONDS", "0"), "JIRA_HTTP_TIMEOUT_SECONDS"},
		{"bad log level", with("LOG_LEVEL", "trace"), "LOG_LEVEL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFrom(env(tt.env))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "s3cret") {
				t.Fatal("error leaks token")
			}
		})
	}
}

func TestDefaultsAndNormalization(t *testing.T) {
	c, err := LoadFrom(env(map[string]string{
		"JIRA_URL": "https://jira.example.com//", "JIRA_PERSONAL_TOKEN": "s3cret",
		"JIRA_ALLOWED_PROJECTS": " plat, Core ,,",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.EnableWriteTools {
		t.Error("write tools must default to false")
	}
	if c.JiraURL != "https://jira.example.com" {
		t.Errorf("url = %q", c.JiraURL)
	}
	if c.HTTPTimeoutSec != 30 {
		t.Errorf("timeout = %d", c.HTTPTimeoutSec)
	}
	if len(c.AllowedProjects) != 2 || c.AllowedProjects[0] != "PLAT" || c.AllowedProjects[1] != "CORE" {
		t.Errorf("allowed = %v", c.AllowedProjects)
	}
}

func TestTokenRedaction(t *testing.T) {
	c := &Config{JiraURL: "https://j", PersonalToken: "s3cret"}
	for _, s := range []string{c.String(), fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c)} {
		if strings.Contains(s, "s3cret") || !strings.Contains(s, "[REDACTED]") {
			t.Errorf("bad output: %s", s)
		}
	}
}

func TestIsProjectAllowed(t *testing.T) {
	tests := []struct {
		allowed []string
		key     string
		want    bool
	}{
		{nil, "ANY", true},
		{[]string{"PLAT", "CORE"}, "plat", true},
		{[]string{"PLAT", "CORE"}, " CORE ", true},
		{[]string{"PLAT"}, "OTHER", false},
		{[]string{"PLAT"}, "", false},
	}
	for _, tt := range tests {
		c := &Config{AllowedProjects: tt.allowed}
		if got := c.IsProjectAllowed(tt.key); got != tt.want {
			t.Errorf("%v %q = %v, want %v", tt.allowed, tt.key, got, tt.want)
		}
	}
}
