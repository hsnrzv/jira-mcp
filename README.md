# jira-mcp

Lightweight, clean-room Go MCP server for **Jira Data Center** (REST API v2 + Agile 1.0), authenticated with a Personal Access Token. The only third-party dependency is the official MCP Go SDK.

## Quick start (colleagues — no Docker, no clone)

Download the zip for your OS, double-click `Jira MCP.app` / `JiraMCP.exe`, paste Jira URL, PAT and project key into the native setup dialog, then install or copy the generated MCP config into your IDE. Details: `doc/runbooks/desktop-app.md`. The MCP server itself runs as `jira-mcp serve`.

## Configuration (environment; overrides values saved by the setup UI)

| Variable | Default | Notes |
|---|---|---|
| `JIRA_URL` | required | `http(s)://` base URL |
| `JIRA_PERSONAL_TOKEN` | required | PAT, never logged |
| `JIRA_ENABLE_WRITE_TOOLS` | `false` | `true`/`false` only |
| `JIRA_ALLOWED_PROJECTS` | all | e.g. `PLAT,CORE` |
| `JIRA_HTTP_TIMEOUT_SECONDS` | `30` | |
| `JIRA_CUSTOM_CA_CERT_PATH` | – | corporate root CA PEM |
| `LOG_LEVEL` | `info` | debug/info/warn/error (stderr) |

## Tools

Read (11): `jira_get_myself`, `jira_list_projects`, `jira_get_issue`, `jira_get_issue_context` (description, links, subtasks, all comments, attachment list + small text attachment contents in one call), `jira_get_attachment` (text/image download, max 5 MB), `jira_search_issues`, `jira_get_issue_comments`, `jira_get_issue_transitions`, `jira_list_boards`, `jira_list_board_sprints`, `jira_get_sprint_issues`.
Write (4, only when enabled): `jira_create_issue`, `jira_update_issue`, `jira_add_comment`, `jira_transition_issue`.

## Build & test

```sh
go vet ./... && go test ./...
./scripts/build-release.sh       # standalone desktop binaries in dist/
```

See `doc/` for architecture, security model, ADRs and runbooks.
