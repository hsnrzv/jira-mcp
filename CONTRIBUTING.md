# Contributing to jira-mcp

Thanks for your interest! Bug reports, ideas and pull requests are all welcome.

## Ground rules

- **Clean-room only.** Do not copy code, comments, prompts or schemas from other Jira MCP projects.
  Implement against Atlassian's public Jira Data Center REST API v2 (`/rest/api/2`) and Agile 1.0 (`/rest/agile/1.0`).
- **Scope.** Jira Data Center / Server with Personal Access Tokens. Jira Cloud and Confluence are out of scope
  (see [ADR-003](doc/adr/ADR-003-dc-pat-only.md)).
- **Safe defaults.** Write tools stay opt-in (`JIRA_ENABLE_WRITE_TOOLS=false`). No DELETE operations.
- **stdout is sacred.** `serve` speaks JSON-RPC on stdout; log only via `log/slog` to stderr.
- **Never log the PAT**, not even in tests or debug output.
- **Minimal dependencies.** The standard library plus the official MCP Go SDK. Discuss new dependencies in an issue first.

## Development setup

Requirements: Go (version in [`go.mod`](go.mod)) and Git.

```sh
go build -o jira-mcp ./cmd/jira-mcp
go vet ./... && go test ./...
```

Release artifacts: `./scripts/build-release.sh` (needs `zip`; DMGs only on macOS).

## Pull requests

1. Open an issue for larger changes so we can agree on the approach.
2. Keep changes focused; add table-driven tests for new behaviour.
3. Use `context.Context`, typed structs and wrapped errors (`fmt.Errorf("…: %w", err)`).
4. Update the docs under [`doc/`](doc/) (architecture, security model, API mapping, ADRs) when behaviour changes.
5. Make sure `gofmt`, `go vet ./...` and `go test ./...` pass — CI runs them on Linux, macOS and Windows.

By contributing you agree that your contributions are licensed under the [MIT License](LICENSE)
and that you follow the [Code of Conduct](CODE_OF_CONDUCT.md).
