# ADR-001: Clean-room Go implementation on a distroless image

**Status:** Accepted

## Context
Corporate policy prohibits third-party Jira MCP servers (supply-chain risk). We need a small, auditable server.

## Decision
Implement our own server in Go against Atlassian's public Jira DC REST API and the official MCP Go SDK. Use only the standard library otherwise. Ship as a static binary in `gcr.io/distroless/static-debian12:nonroot`.

## Consequences
+ Tiny dependency surface, static binary, minimal CVE footprint.
− We own maintenance of API models and tools.

## Alternatives Rejected
- Adopting the third-party server (policy violation).
- Python/Node implementation (larger runtime and dependency trees).
