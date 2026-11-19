# Copilot Instructions — jira-mcp

1. **Clean-room implementation only.** Never copy code, comments, prompts or schemas from third-party Jira MCP repositories. Implement strictly against Atlassian Jira Data Center REST API v2 (`/rest/api/2`) and `/rest/agile/1.0`.
2. **Scope.** On-premise Jira Data Center with Personal Access Token (`Authorization: Bearer <PAT>`) only. Never add Jira Cloud (`/rest/api/3`, ADF, email+token Basic auth) or Confluence support.
3. **Safety.** `JIRA_ENABLE_WRITE_TOOLS` defaults to `false`. When false, write tools are not registered on the MCP server and write methods in the Jira client return `ErrWriteDisabled`. No DELETE operations.
4. **Transport.** MCP runs over stdio. All logging uses `log/slog` to `os.Stderr`. Never use `fmt.Println` or write non-JSON-RPC data to `os.Stdout`.
5. **Secrets.** Never log or print the PAT.
6. **Docs.** Architectural decisions, security controls and API contracts live under `doc/`; update them with code changes.
7. **Idiomatic Go.** `context.Context`, typed structs, wrapped errors (`%w`), HTTP timeouts, table-driven tests. Run `go vet ./... && go test ./...` before finishing.
