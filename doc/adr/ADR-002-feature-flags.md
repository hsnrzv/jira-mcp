# ADR-002: Read-only by default with layered write gating

**Status:** Accepted

## Context
LLM agents may call mutating tools unexpectedly; many deployments only need read access.

## Decision
`JIRA_ENABLE_WRITE_TOOLS` defaults to `false`. When false, write tools are not registered (invisible in `tools/list`). Independently, every method in `write_ops.go` and `doWrite` returns `ErrWriteDisabled`. Writes additionally enforce `JIRA_ALLOWED_PROJECTS`. No delete operations are implemented.

## Consequences
+ Two independent barriers; a bug in one does not enable writes.
+ Switching modes needs a restart/config change, not a prompt.
− Runtime flag only; a read-only build variant is possible later.

## Alternatives Rejected
- Compile-time build tags only (less flexible for IDE profiles).
- Relying on PAT permissions alone.
