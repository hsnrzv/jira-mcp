package mcpserver

import (
	"context"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/internal/jira-mcp/internal/config"
	"github.com/internal/jira-mcp/internal/jira"
)

func listTools(t *testing.T, write bool) []string {
	t.Helper()
	cfg := &config.Config{JiraURL: "http://127.0.0.1:1", PersonalToken: "x", HTTPTimeoutSec: 1, EnableWriteTools: write}
	client, err := jira.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(cfg, client)
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}
	sort.Strings(names)
	return names
}

func TestToolRegistration(t *testing.T) {
	ro := listTools(t, false)
	if len(ro) != 11 {
		t.Errorf("read-only: %d tools: %v", len(ro), ro)
	}
	for _, n := range ro {
		switch n {
		case "jira_create_issue", "jira_update_issue", "jira_add_comment", "jira_transition_issue":
			t.Errorf("write tool %s exposed in read-only mode", n)
		}
	}
	if rw := listTools(t, true); len(rw) != 15 {
		t.Errorf("read-write: %d tools: %v", len(rw), rw)
	}
}
