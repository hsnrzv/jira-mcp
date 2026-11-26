// Package mcpserver wires Jira operations into an MCP server.
package mcpserver

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hsnrzv/jira-mcp/internal/config"
	"github.com/hsnrzv/jira-mcp/internal/jira"
)

// NewServer builds the MCP server. Write tools are registered only when enabled in cfg.
func NewServer(cfg *config.Config, client *jira.Client) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "jira-mcp", Version: "1.0.0"}, nil)
	registerReadTools(s, client)
	if cfg.EnableWriteTools {
		registerWriteTools(s, client)
		slog.Info("server mode", "mode", "READ-WRITE")
	} else {
		slog.Info("server mode", "mode", "READ-ONLY")
	}
	return s
}

// jsonResult renders v as compact JSON text.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

// addTool registers a handler whose return value is serialized as JSON.
func addTool[In any](s *mcp.Server, name, desc string, readOnly bool, fn func(ctx context.Context, in In) (any, error)) {
	t := &mcp.Tool{Name: name, Description: desc, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly}}
	if !readOnly {
		f := false
		t.Annotations.DestructiveHint = &f
	}
	mcp.AddTool(s, t, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		v, err := fn(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(v)
	})
}
