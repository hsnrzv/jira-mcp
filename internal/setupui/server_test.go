package setupui

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

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
