package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/internal/jira-mcp/internal/config"
	"github.com/internal/jira-mcp/internal/jira"
	"github.com/internal/jira-mcp/internal/mcpserver"
	"github.com/internal/jira-mcp/internal/settings"
	"github.com/internal/jira-mcp/internal/setupui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// No arguments (e.g. double-click) opens the setup UI; MCP clients launch "serve".
	cmd := "setup"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "serve":
		os.Exit(runServe(ctx))
	case "setup":
		os.Exit(runSetup(ctx))
	default:
		fmt.Fprintf(os.Stderr, "usage: %s [setup|serve]\n", os.Args[0])
		os.Exit(2)
	}
}

func runServe(ctx context.Context) int {
	// stdout carries JSON-RPC only; logs go to stderr.
	saved, err := settings.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cfg, err := config.LoadFrom(saved.Getenv(os.Getenv))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "Run the app without arguments to open the setup form.")
		return 1
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})))

	client, err := jira.NewClient(cfg)
	if err != nil {
		slog.Error("failed to create Jira client", "error", err)
		return 1
	}

	server := mcpserver.NewServer(cfg, client)
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		slog.Error("server stopped", "error", err)
		return 1
	}
	return 0
}

func runSetup(ctx context.Context) int {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	srv, err := setupui.New(setupui.ExecutablePath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot start setup UI:", err)
		return 1
	}
	url := srv.URL()
	fmt.Fprintln(os.Stderr, "Jira MCP setup is running. Opening your browser...")
	fmt.Fprintln(os.Stderr, "If it does not open, visit:", url)
	if err := setupui.OpenBrowser(url); err != nil {
		slog.Warn("could not open browser", "error", err)
	}
	if err := srv.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "setup UI stopped:", err)
		return 1
	}
	return 0
}
