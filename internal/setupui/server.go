// Package setupui serves a small localhost-only web form that collects the Jira
// URL, PAT and project key, validates them against Jira and stores them.
package setupui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/internal/jira-mcp/internal/config"
	"github.com/internal/jira-mcp/internal/jira"
	"github.com/internal/jira-mcp/internal/settings"
)

//go:embed index.html
var indexHTML string

// faviconPNG is generated from assets/icon.svg by scripts/generate-icons.sh.
//
//go:embed favicon.png
var faviconPNG []byte

// Server is the setup web UI.
type Server struct {
	token    string
	exePath  string
	load     func() (settings.Settings, error)
	save     func(settings.Settings) error
	validate func(ctx context.Context, s settings.Settings) (string, error)
	targets  func() ([]Target, error)
	quit     chan struct{}
	listener net.Listener
}

// New creates a setup server bound to a random port on the loopback interface.
func New(exePath string) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		ln.Close()
		return nil, err
	}
	return &Server{
		token:    hex.EncodeToString(b),
		exePath:  exePath,
		load:     settings.Load,
		save:     settings.Save,
		validate: validateAgainstJira,
		targets:  Targets,
		quit:     make(chan struct{}),
		listener: ln,
	}, nil
}

// URL is the address to open in the browser (contains the session token).
func (s *Server) URL() string {
	return "http://" + s.listener.Addr().String() + "/?t=" + s.token
}

// Run serves until ctx is cancelled or the user clicks Close.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(s.listener) }()
	select {
	case <-ctx.Done():
	case <-s.quit:
	case err := <-errc:
		return err
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /favicon.png", handleFavicon)
	mux.HandleFunc("GET /api/settings", s.guard(s.handleSettings))
	mux.HandleFunc("POST /api/test", s.guard(s.handleTest))
	mux.HandleFunc("POST /api/save", s.guard(s.handleSave))
	mux.HandleFunc("POST /api/install", s.guard(s.handleInstall))
	mux.HandleFunc("POST /api/quit", s.guard(s.handleQuit))
	return s.hostCheck(mux)
}

// hostCheck rejects requests whose Host is not our loopback address, which
// blocks DNS-rebinding attacks from other web pages.
func (s *Server) hostCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != s.listener.Addr().String() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) tokenOK(got string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func (s *Server) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.tokenOK(r.Header.Get("X-Setup-Token")) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid session token"})
			return
		}
		h(w, r)
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if !s.tokenOK(r.URL.Query().Get("t")) {
		http.Error(w, "forbidden: open the URL printed by the app", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(strings.Replace(indexHTML, "__TOKEN__", s.token, 1)))
}

// handleFavicon serves the public app icon; it needs no session token.
func handleFavicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(faviconPNG)
}

func (s *Server) handleSettings(w http.ResponseWriter, _ *http.Request) {
	cur, err := s.load()
	if err != nil {
		slog.Warn("could not load settings", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jiraUrl":          cur.JiraURL,
		"projectKeys":      cur.ProjectKeys,
		"enableWriteTools": cur.EnableWriteTools,
		"customCaCertPath": cur.CustomCACertPath,
		"hasToken":         cur.PersonalToken != "",
	})
}

// readForm decodes the submitted form, keeping the saved token when blank.
func (s *Server) readForm(r *http.Request) (settings.Settings, error) {
	var in settings.Settings
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10)).Decode(&in); err != nil {
		return in, errors.New("invalid request body")
	}
	fallback := ""
	if cur, err := s.load(); err == nil {
		fallback = cur.PersonalToken
	}
	return Normalize(in, fallback)
}

// Normalize trims the form values, upper-cases project keys and uses
// fallbackToken when no token was entered. It is shared by all setup UIs.
func Normalize(in settings.Settings, fallbackToken string) (settings.Settings, error) {
	in.JiraURL = strings.TrimSpace(in.JiraURL)
	in.PersonalToken = strings.TrimSpace(in.PersonalToken)
	in.ProjectKeys = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(in.ProjectKeys), " ", ""))
	in.CustomCACertPath = strings.TrimSpace(in.CustomCACertPath)
	if in.PersonalToken == "" {
		in.PersonalToken = fallbackToken
	}
	if in.ProjectKeys == "" {
		return in, errors.New("project key is required")
	}
	return in, nil
}

func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	in, err := s.readForm(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	msg, err := s.validate(r.Context(), in)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	in, err := s.readForm(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	msg, err := s.validate(r.Context(), in)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.save(in); err != nil {
		slog.Error("save settings failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"message":    "Saved. " + msg,
		"vscode":     VSCodeSnippet(s.exePath),
		"mcpServers": MCPServersSnippet(s.exePath),
	})
}

func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	targets, err := s.targets()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for _, t := range targets {
		if t.ID != in.Target {
			continue
		}
		if err := t.Install(s.exePath); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Added \"jira\" to %s (%s). Restart your IDE/agent.", t.Label, t.Path)})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown target"})
}

func (s *Server) handleQuit(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "bye"})
	select {
	case <-s.quit:
	default:
		close(s.quit)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Validate checks the URL/PAT with /myself and that each project key exists.
func Validate(ctx context.Context, s settings.Settings) (string, error) {
	return validateAgainstJira(ctx, s)
}

func validateAgainstJira(ctx context.Context, s settings.Settings) (string, error) {
	if s.PersonalToken == "" {
		return "", errors.New("personal access token is required")
	}
	cfg, err := config.LoadFrom(s.Getenv(func(string) string { return "" }))
	if err != nil {
		return "", err
	}
	client, err := jira.NewClient(cfg)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	me, err := client.GetCurrentUser(ctx)
	if err != nil {
		return "", fmt.Errorf("could not authenticate against Jira: %w", err)
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return "", fmt.Errorf("authenticated, but listing projects failed: %w", err)
	}
	have := map[string]bool{}
	for _, p := range projects {
		have[strings.ToUpper(p.Key)] = true
	}
	for _, k := range cfg.AllowedProjects {
		if !have[k] {
			return "", fmt.Errorf("project %q was not found or is not visible to this token", k)
		}
	}
	name := me.DisplayName
	if name == "" {
		name = me.Name
	}
	return fmt.Sprintf("Connected to Jira as %s; project(s) %s OK.", name, strings.Join(cfg.AllowedProjects, ", ")), nil
}

// VSCodeSnippet renders a VS Code / Copilot mcp.json entry.
func VSCodeSnippet(exe string) string {
	return snippet("servers", exe, true)
}

// MCPServersSnippet renders a JetBrains / Claude Desktop entry.
func MCPServersSnippet(exe string) string {
	return snippet("mcpServers", exe, false)
}

func snippet(root, exe string, withType bool) string {
	entry := map[string]any{"command": exe, "args": []string{"serve"}}
	if withType {
		entry["type"] = "stdio"
	}
	b, _ := json.MarshalIndent(map[string]any{root: map[string]any{"jira": entry}}, "", "  ")
	return string(b)
}

// IconPNG returns the embedded application icon.
func IconPNG() []byte { return faviconPNG }

// OpenBrowser opens url in the default browser; failures are non-fatal.
func OpenBrowser(url string) error {
	return openBrowser(url)
}

// ExecutablePath returns the absolute path of the running binary.
func ExecutablePath() string {
	p, err := os.Executable()
	if err != nil {
		return "jira-mcp"
	}
	return p
}
