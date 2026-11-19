// Package config loads and validates runtime configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config holds validated runtime settings.
type Config struct {
	JiraURL          string
	PersonalToken    string
	EnableWriteTools bool
	AllowedProjects  []string
	HTTPTimeoutSec   int
	CustomCACertPath string
	LogLevel         slog.Level
}

// Load reads configuration from the process environment.
func Load() (*Config, error) {
	return LoadFrom(os.Getenv)
}

// LoadFrom reads configuration using the supplied lookup function.
func LoadFrom(getenv func(string) string) (*Config, error) {
	var errs []error
	c := &Config{HTTPTimeoutSec: 30, LogLevel: slog.LevelInfo}

	raw := strings.TrimSpace(getenv("JIRA_URL"))
	if raw == "" {
		errs = append(errs, errors.New("JIRA_URL is required"))
	} else if u, err := url.Parse(raw); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, errors.New("JIRA_URL must be a valid http:// or https:// URL"))
	} else {
		c.JiraURL = strings.TrimRight(raw, "/")
	}

	c.PersonalToken = strings.TrimSpace(getenv("JIRA_PERSONAL_TOKEN"))
	if c.PersonalToken == "" {
		errs = append(errs, errors.New("JIRA_PERSONAL_TOKEN is required"))
	}

	switch v := strings.TrimSpace(getenv("JIRA_ENABLE_WRITE_TOOLS")); v {
	case "", "false":
	case "true":
		c.EnableWriteTools = true
	default:
		errs = append(errs, errors.New(`JIRA_ENABLE_WRITE_TOOLS must be "true" or "false"`))
	}

	for _, p := range strings.Split(getenv("JIRA_ALLOWED_PROJECTS"), ",") {
		if p = strings.ToUpper(strings.TrimSpace(p)); p != "" {
			c.AllowedProjects = append(c.AllowedProjects, p)
		}
	}

	if v := strings.TrimSpace(getenv("JIRA_HTTP_TIMEOUT_SECONDS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			errs = append(errs, errors.New("JIRA_HTTP_TIMEOUT_SECONDS must be a positive integer"))
		} else {
			c.HTTPTimeoutSec = n
		}
	}

	c.CustomCACertPath = strings.TrimSpace(getenv("JIRA_CUSTOM_CA_CERT_PATH"))

	if v := strings.TrimSpace(getenv("LOG_LEVEL")); v != "" {
		switch strings.ToLower(v) {
		case "debug":
			c.LogLevel = slog.LevelDebug
		case "info":
			c.LogLevel = slog.LevelInfo
		case "warn":
			c.LogLevel = slog.LevelWarn
		case "error":
			c.LogLevel = slog.LevelError
		default:
			errs = append(errs, errors.New("LOG_LEVEL must be one of debug, info, warn, error"))
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return c, nil
}

// IsProjectAllowed reports whether the project key is within the allowlist.
// An empty allowlist permits every project.
func (c *Config) IsProjectAllowed(projectKey string) bool {
	if len(c.AllowedProjects) == 0 {
		return true
	}
	key := strings.ToUpper(strings.TrimSpace(projectKey))
	for _, p := range c.AllowedProjects {
		if p == key {
			return true
		}
	}
	return false
}

// String renders the configuration with the token redacted.
func (c *Config) String() string {
	tok := ""
	if c.PersonalToken != "" {
		tok = "[REDACTED]"
	}
	return fmt.Sprintf("Config{JiraURL:%s PersonalToken:%s EnableWriteTools:%t AllowedProjects:%v HTTPTimeoutSec:%d CustomCACertPath:%q LogLevel:%s}",
		c.JiraURL, tok, c.EnableWriteTools, c.AllowedProjects, c.HTTPTimeoutSec, c.CustomCACertPath, c.LogLevel)
}

// GoString prevents %#v from leaking the token.
func (c *Config) GoString() string { return c.String() }
