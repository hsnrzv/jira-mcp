package jira

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/internal/jira-mcp/internal/config"
)

func newTestClient(t *testing.T, h http.HandlerFunc, mut func(*config.Config)) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cfg := &config.Config{JiraURL: srv.URL, PersonalToken: "tok123", HTTPTimeoutSec: 5}
	if mut != nil {
		mut(cfg)
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestHeadersAndGet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" || r.Header.Get("Accept") != "application/json" ||
			r.Header.Get("User-Agent") != "jira-mcp/1.0" {
			t.Errorf("bad headers: %v", r.Header)
		}
		if r.URL.Path != "/rest/api/2/myself" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"name":"bob","active":true}`))
	}, nil)
	u, err := c.GetCurrentUser(context.Background())
	if err != nil || u.Name != "bob" {
		t.Fatalf("%v %+v", err, u)
	}
}

func TestErrorParsingNoTokenLeak(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"errorMessages":["Issue does not exist"],"errors":{}}`))
	}, nil)
	_, err := c.GetIssue(context.Background(), "ABC-1", "")
	var apiErr *JiraAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "Issue does not exist") || strings.Contains(err.Error(), "tok123") {
		t.Errorf("bad error: %v", err)
	}
}

func TestTimeout(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) }, nil)
	c.http.Timeout = 50 * time.Millisecond
	if _, err := c.GetCurrentUser(context.Background()); err == nil || strings.Contains(err.Error(), "tok123") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestNoRedirectFollow(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://evil.invalid/", http.StatusFound)
	}, nil)
	_, err := c.GetCurrentUser(context.Background())
	var apiErr *JiraAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 302 {
		t.Fatalf("err = %v", err)
	}
}

func TestWriteDisabled(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { called = true }, nil)
	ctx := context.Background()
	if err := c.doWrite(ctx, "POST", "/x", nil, nil); !errors.Is(err, ErrWriteDisabled) {
		t.Errorf("doWrite: %v", err)
	}
	if _, err := c.CreateIssue(ctx, CreateIssueRequest{}); !errors.Is(err, ErrWriteDisabled) {
		t.Errorf("create: %v", err)
	}
	if err := c.UpdateIssue(ctx, "A-1", UpdateIssueRequest{}); !errors.Is(err, ErrWriteDisabled) {
		t.Errorf("update: %v", err)
	}
	if _, err := c.AddComment(ctx, "A-1", "x"); !errors.Is(err, ErrWriteDisabled) {
		t.Errorf("comment: %v", err)
	}
	if err := c.TransitionIssue(ctx, "A-1", "1", ""); !errors.Is(err, ErrWriteDisabled) {
		t.Errorf("transition: %v", err)
	}
	if called {
		t.Error("server must not be contacted")
	}
}

func TestWriteProjectAllowlist(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(204)
	}, func(c *config.Config) { c.EnableWriteTools = true; c.AllowedProjects = []string{"PLAT"} })
	ctx := context.Background()
	req := CreateIssueRequest{}
	req.Fields.Project.Key = "OTHER"
	if _, err := c.CreateIssue(ctx, req); err == nil {
		t.Error("create outside allowlist should fail")
	}
	if err := c.UpdateIssue(ctx, "OTHER-1", UpdateIssueRequest{}); err == nil {
		t.Error("update outside allowlist should fail")
	}
	if _, err := c.AddComment(ctx, "OTHER-1", "x"); err == nil {
		t.Error("comment outside allowlist should fail")
	}
	if err := c.TransitionIssue(ctx, "OTHER-1", "1", ""); err == nil {
		t.Error("transition outside allowlist should fail")
	}
	if called {
		t.Error("server must not be contacted")
	}
	if err := c.TransitionIssue(ctx, "plat-1", "11", "done"); err != nil {
		t.Errorf("allowed transition: %v", err)
	}
}

func TestTransitionPayload(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		if r.Method != "POST" || r.URL.Path != "/rest/api/2/issue/A-1/transitions" ||
			r.Header.Get("Content-Type") != "application/json" || m["transition"].(map[string]any)["id"] != "5" || m["update"] == nil {
			t.Errorf("bad request %s %s %v", r.Method, r.URL.Path, m)
		}
		w.WriteHeader(204)
	}, func(c *config.Config) { c.EnableWriteTools = true })
	if err := c.TransitionIssue(context.Background(), "A-1", "5", "hi"); err != nil {
		t.Fatal(err)
	}
}

func TestScopeJQL(t *testing.T) {
	c := &Client{cfg: &config.Config{AllowedProjects: []string{"PLAT", "CORE"}}}
	tests := map[string]string{
		"status = Open":                       `(status = Open) AND project IN ("PLAT", "CORE")`,
		"status = Open ORDER BY created DESC": `(status = Open) AND project IN ("PLAT", "CORE") ORDER BY created DESC`,
		"project = SECRET OR x = 1":           `(project = SECRET OR x = 1) AND project IN ("PLAT", "CORE")`,
		`summary ~ "a order by b"`:            `(summary ~ "a order by b") AND project IN ("PLAT", "CORE")`,
		"":                                    `project IN ("PLAT", "CORE")`,
		"order by created":                    `project IN ("PLAT", "CORE") order by created`,
	}
	for in, want := range tests {
		if got := c.scopeJQL(in); got != want {
			t.Errorf("scopeJQL(%q)\n got %s\nwant %s", in, got, want)
		}
	}
	if got := (&Client{cfg: &config.Config{}}).scopeJQL("a = b"); got != "a = b" {
		t.Errorf("unscoped: %s", got)
	}
}

func TestReadOps(t *testing.T) {
	var gotQuery string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		switch r.URL.Path {
		case "/rest/api/2/project":
			_, _ = w.Write([]byte(`[{"key":"PLAT"},{"key":"SEC"}]`))
		case "/rest/agile/1.0/board":
			_, _ = w.Write([]byte(`{"values":[{"id":1,"location":{"projectKey":"PLAT"}},{"id":2,"location":{"projectKey":"SEC"}},{"id":3}]}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}, func(c *config.Config) { c.AllowedProjects = []string{"PLAT"} })
	ctx := context.Background()

	ps, _ := c.ListProjects(ctx)
	if len(ps) != 1 || ps[0].Key != "PLAT" {
		t.Errorf("projects %v", ps)
	}
	bs, _ := c.ListBoards(ctx, "", 0, 0)
	if len(bs.Values) != 1 || bs.Values[0].ID != 1 {
		t.Errorf("boards %+v", bs.Values)
	}
	if _, err := c.GetIssue(ctx, "SEC-1", ""); err == nil {
		t.Error("out-of-scope issue must be rejected")
	}
	if _, err := c.GetIssue(ctx, "../etc", ""); err == nil {
		t.Error("invalid key must be rejected")
	}
	if _, err := c.SearchIssues(ctx, "x=1", 0, 999, "summary"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "maxResults=50") || !strings.Contains(gotQuery, "fields=summary") {
		t.Errorf("query %s", gotQuery)
	}
}

func TestGetIssueContext(t *testing.T) {
	var c *Client
	c = newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/2/issue/ABC-1":
			_, _ = w.Write([]byte(`{"key":"ABC-1","fields":{"summary":"s","description":"d",
			"issuelinks":[{"type":{"name":"Blocks"},"outwardIssue":{"key":"ABC-2","fields":{"summary":"x"}}},
			{"type":{"name":"Blocks"},"outwardIssue":{"key":"OTH-9","fields":{"summary":"y"}}}],
			"attachment":[{"id":"10","filename":"a.log","mimeType":"text/plain","size":5,"content":"` + c.cfg.JiraURL + `/secure/attachment/10/a.log"},
			{"id":"11","filename":"b.bin","mimeType":"application/zip","size":5,"content":"` + c.cfg.JiraURL + `/secure/attachment/11/b.bin"}]}}`))
		case "/rest/api/2/issue/ABC-1/comment":
			_, _ = w.Write([]byte(`{"total":1,"comments":[{"id":"1","body":"hi"}]}`))
		case "/secure/attachment/10/a.log":
			if r.Header.Get("Authorization") == "" {
				t.Error("missing auth on download")
			}
			_, _ = w.Write([]byte("hello"))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}, func(cfg *config.Config) { cfg.AllowedProjects = []string{"ABC"} })
	ctx, err := c.GetIssueContext(context.Background(), "ABC-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.Comments) != 1 || len(ctx.Issue.Fields.IssueLinks) != 1 {
		t.Errorf("comments/links: %+v", ctx)
	}
	if len(ctx.Attachments) != 2 || ctx.Attachments[0].Text != "hello" || ctx.Attachments[1].Note == "" {
		t.Errorf("attachments: %+v", ctx.Attachments)
	}
}

func TestDownloadAttachmentContextPath(t *testing.T) {
	var c *Client
	c = newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jira/rest/api/2/issue/ABC-1":
			base := strings.TrimSuffix(c.cfg.JiraURL, "/jira")
			_, _ = w.Write([]byte(`{"key":"ABC-1","fields":{"attachment":[
			{"id":"1","filename":"a.png","mimeType":"image/png","content":"` + c.cfg.JiraURL + `/secure/attachment/1/a.png"},
			{"id":"2","filename":"b.png","mimeType":"image/png","content":"http://evil.invalid/jira/secure/attachment/2/b.png"},
			{"id":"3","filename":"c.png","mimeType":"image/png","content":"` + base + `/other/secure/attachment/3/c.png"}]}}`))
		case "/jira/secure/attachment/1/a.png":
			if r.Header.Get("Authorization") != "Bearer tok123" {
				t.Error("missing auth on download")
			}
			_, _ = w.Write([]byte("PNG"))
		default:
			http.Redirect(w, r, "/jira/login.jsp", http.StatusFound)
		}
	}, func(cfg *config.Config) { cfg.JiraURL += "/jira" })

	tests := []struct {
		id      string
		wantErr bool
	}{
		{"1", false},
		{"2", true},
		{"3", true},
	}
	for _, tt := range tests {
		a, err := c.DownloadAttachment(context.Background(), "ABC-1", tt.id)
		if tt.wantErr {
			if err == nil {
				t.Errorf("id %s: expected error", tt.id)
			}
			continue
		}
		if err != nil || string(a.Data) != "PNG" {
			t.Errorf("id %s: err=%v content=%+v", tt.id, err, a)
		}
	}
}
