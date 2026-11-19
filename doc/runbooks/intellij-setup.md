# Runbook: IDE Setup

Build the binary with `./scripts/build-release.sh` (or `go build -o jira-mcp ./cmd/jira-mcp`) and point `command` at its absolute path.

## JetBrains AI Assistant / Junie (`mcpServers`) — read-only

```json
{
  "mcpServers": {
    "jira": {
      "command": "/absolute/path/to/jira-mcp",
      "args": ["serve"],
      "env": {
        "JIRA_URL": "https://jira.example.com",
        "JIRA_PERSONAL_TOKEN": "<your-pat>",
        "JIRA_ENABLE_WRITE_TOOLS": "false",
        "JIRA_ALLOWED_PROJECTS": "PLAT,CORE"
      }
    }
  }
}
```

For read-write set `"JIRA_ENABLE_WRITE_TOOLS": "true"`.

## GitHub Copilot Agent Mode (`servers`) — read-write

```json
{
  "inputs": [{ "id": "jira-pat", "type": "promptString", "description": "Jira PAT", "password": true }],
  "servers": {
    "jira": {
      "type": "stdio",
      "command": "/absolute/path/to/jira-mcp",
      "args": ["serve"],
      "env": {
        "JIRA_URL": "https://jira.example.com",
        "JIRA_PERSONAL_TOKEN": "${input:jira-pat}",
        "JIRA_ENABLE_WRITE_TOOLS": "true",
        "JIRA_ALLOWED_PROJECTS": "PLAT"
      }
    }
  }
}
```

For read-only set `"JIRA_ENABLE_WRITE_TOOLS": "false"`.
