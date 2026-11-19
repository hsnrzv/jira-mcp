package setupui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/internal/jira-mcp/internal/jira"
	"github.com/internal/jira-mcp/internal/settings"
)

func newTestServer(t *testing.T) (*Server, *settings.Settings) {
	t.Helper()
	s, err := New("/bin/jira-mcp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.listener.Close() })
	var stored settings.Settings
	s.load = func() (settings.Settings, error) { return stored, nil }
	s.save = func(v settings.Settings) error { stored = v; return nil }
	s.validate = func(context.Context, settings.Settings) (string, error) { return "ok", nil }
	return s, &stored
}

func do(s *Server, method, path, host, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = host
	if token != "" {
		r.Header.Set("X-Setup-Token", token)
	}
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	return w
}

func TestAccessControl(t *testing.T) {
	s, _ := newTestServer(t)
	host := s.listener.Addr().String()
	tests := []struct {
		name, method, path, host, token string
		want                            int
	}{
		{"index ok", "GET", "/?t=" + s.token, host, "", 200},
		{"index bad token", "GET", "/?t=nope", host, "", 403},
		{"rebinding host", "GET", "/?t=" + s.token, "evil.com", "", 403},
		{"favicon public", "GET", "/favicon.png", host, "", 200},
		{"favicon rebinding host", "GET", "/favicon.png", "evil.com", "", 403},
		{"api no token", "GET", "/api/settings", host, "", 403},
		{"api ok", "GET", "/api/settings", host, s.token, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := do(s, tt.method, tt.path, tt.host, tt.token, "").Code; got != tt.want {
				t.Errorf("status = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSaveKeepsExistingTokenAndNeverEchoesIt(t *testing.T) {
	s, stored := newTestServer(t)
	*stored = settings.Settings{PersonalToken: "oldtoken"}
	host := s.listener.Addr().String()

	w := do(s, "POST", "/api/save", host, s.token, `{"jiraUrl":"https://j","projectKeys":"plat, core"}`)
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if stored.PersonalToken != "oldtoken" || stored.ProjectKeys != "PLAT,CORE" {
		t.Errorf("stored = %v", *stored)
	}
	if strings.Contains(w.Body.String(), "oldtoken") {
		t.Error("response leaked token")
	}
	if g := do(s, "GET", "/api/settings", host, s.token, "").Body.String(); strings.Contains(g, "oldtoken") || !strings.Contains(g, `"hasToken":true`) {
		t.Errorf("settings response: %s", g)
	}
	if w := do(s, "POST", "/api/save", host, s.token, `{"jiraUrl":"https://j"}`); w.Code != 400 {
		t.Errorf("missing project key: status %d", w.Code)
	}
}

func TestProjectsEndpoint(t *testing.T) {
	s, stored := newTestServer(t)
	*stored = settings.Settings{PersonalToken: "oldtoken", ProjectKeys: "PLAT,GONE"}
	var got settings.Settings
	s.projects = func(_ context.Context, in settings.Settings) (string, []jira.Project, error) {
		got = in
		return "Ann", []jira.Project{{Key: "CORE", Name: "Core"}, {Key: "PLAT", Name: "Platform"}}, nil
	}
	host := s.listener.Addr().String()

	w := do(s, "POST", "/api/projects", host, s.token, `{"jiraUrl":" https://j ","customCaCertPath":" /ca "}`)
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if got.JiraURL != "https://j" || got.PersonalToken != "oldtoken" || got.CustomCACertPath != "/ca" {
		t.Errorf("projects called with %v", got)
	}
	var resp struct {
		Message  string         `json:"message"`
		Projects []jira.Project `json:"projects"`
		Selected []string       `json:"selected"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Projects) != 2 || !slices.Equal(resp.Selected, []string{"PLAT"}) || !strings.Contains(resp.Message, "Ann") {
		t.Errorf("response = %+v", resp)
	}
	if strings.Contains(w.Body.String(), "oldtoken") {
		t.Error("response leaked token")
	}

	s.projects = func(context.Context, settings.Settings) (string, []jira.Project, error) {
		return "", nil, errors.New("could not authenticate")
	}
	if w := do(s, "POST", "/api/projects", host, s.token, `{}`); w.Code != 400 || !strings.Contains(w.Body.String(), "could not authenticate") {
		t.Errorf("error: status %d: %s", w.Code, w.Body)
	}
	if w := do(s, "POST", "/api/projects", host, "", `{}`); w.Code != 403 {
		t.Errorf("no token: status %d", w.Code)
	}
}

func TestProjectsListsAllVisibleProjectsSorted(t *testing.T) {
	tests := []struct {
		name     string
		projects string
		status   int
		want     []string
		wantErr  string
	}{
		{"sorted, allowlist ignored", `[{"key":"plat","name":"Platform"},{"key":"CORE","name":"Core"}]`, 200, []string{"CORE", "PLAT"}, ""},
		{"none visible", `[]`, 200, nil, "cannot see any projects"},
		{"unauthorized", `{}`, 401, nil, "could not authenticate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jiraSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer pat" {
					t.Errorf("auth header = %q", r.Header.Get("Authorization"))
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/rest/api/2/myself":
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(`{"name":"ann","displayName":"Ann"}`))
				case "/rest/api/2/project":
					_, _ = w.Write([]byte(tt.projects))
				default:
					http.NotFound(w, r)
				}
			}))
			defer jiraSrv.Close()
			user, projects, err := Projects(context.Background(), settings.Settings{JiraURL: jiraSrv.URL, PersonalToken: "pat", ProjectKeys: "OTHER"})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			keys := make([]string, len(projects))
			for i, p := range projects {
				keys[i] = p.Key
			}
			if user != "Ann" || !slices.Equal(keys, tt.want) {
				t.Errorf("got %q %v, want Ann %v", user, keys, tt.want)
			}
		})
	}
	if _, _, err := Projects(context.Background(), settings.Settings{JiraURL: "https://j"}); err == nil {
		t.Error("expected error without token")
	}
}
