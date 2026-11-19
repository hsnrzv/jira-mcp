# 01 — System Overview

## Context

```mermaid
flowchart LR
  IDE["IntelliJ AI Assistant / Junie / Copilot Agent"] -- "stdio JSON-RPC" --> C["jira-mcp serve"]
  C -- "HTTPS + Bearer PAT" --> J["Jira Data Center<br/>/rest/api/2, /rest/agile/1.0"]
```

Trust boundaries: (1) IDE ↔ jira-mcp process (local process pipe); (2) jira-mcp ↔ Jira (network, TLS). The LLM is untrusted input: all tool arguments are validated and scoped server-side.

## Components

```mermaid
flowchart TB
  main["cmd/jira-mcp"] --> cfg["internal/config"]
  main --> srv["internal/mcpserver"]
  srv --> cl["internal/jira (client, read_ops, write_ops)"]
  cl --> cfg
```

## Request flow

```mermaid
sequenceDiagram
  participant A as Agent
  participant M as mcpserver
  participant C as jira.Client
  participant J as Jira DC
  A->>M: tools/call jira_search_issues
  M->>C: SearchIssues(jql)
  C->>C: scope JQL to allowed projects
  C->>J: GET /rest/api/2/search (Bearer)
  J-->>C: JSON
  C-->>M: typed model
  M-->>A: compact JSON text
```

## Stream isolation

`stdout` carries only JSON-RPC frames (owned by the SDK stdio transport). All diagnostics use `log/slog` JSON on `stderr`. Nothing else writes to `stdout`; a stray byte would corrupt the protocol.
