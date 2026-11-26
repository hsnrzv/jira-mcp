package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hsnrzv/jira-mcp/internal/config"
	"github.com/hsnrzv/jira-mcp/internal/jira"
	"github.com/hsnrzv/jira-mcp/internal/mcpserver"
	"github.com/hsnrzv/jira-mcp/internal/nativeui"
	"github.com/hsnrzv/jira-mcp/internal/settings"
	"github.com/hsnrzv/jira-mcp/internal/setupui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// No arguments (e.g. double-click) opens the setup dialog; MCP clients launch "serve".
	cmd := "setup"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "serve":
		os.Exit(runServe(ctx))
	case "setup":
		web := len(os.Args) > 2 && os.Args[2] == "--web"
		os.Exit(runSetup(ctx, web))
	default:
		fmt.Fprintf(os.Stderr, "usage: %s [setup [--web]|serve]\n", os.Args[0])
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

func runSetup(ctx context.Context, web bool) int {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	exe := setupui.ExecutablePath()
	if !web {
		err := runNativeSetup(ctx, exe)
		if err == nil || ctx.Err() != nil {
			return 0
		}
		if !errors.Is(err, nativeui.ErrUnsupported) {
			slog.Warn("native setup dialog failed; falling back to the browser form", "error", err)
		}
	}
	return runWebSetup(ctx, exe)
}

func runNativeSetup(ctx context.Context, exe string) error {
	ui, err := nativeui.Open(ctx, nativeui.Title, setupui.IconPNG())
	if err != nil {
		return err
	}
	defer func() { _ = ui.Close() }()
	return nativeui.NewFlow(ui, exe).Run(ctx)
}

func runWebSetup(ctx context.Context, exe string) int {
	srv, err := setupui.New(exe)
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
