# 02 — Security Model

| Threat | Mitigation |
|---|---|
| PAT disclosure | Read from env, or from the setup-UI settings file (`<user config dir>/jira-mcp/settings.json`, mode 0600, never echoed back to the browser); `Config.String()/GoString()` redact it; never placed in errors or logs; request errors strip the URL. Use a least-privilege PAT. |
| Unintended mutation | Defense in depth: (1) write tools not registered unless `JIRA_ENABLE_WRITE_TOOLS=true` (absent from `tools/list`); (2) every write op and `doWrite` check the flag and return `ErrWriteDisabled`. No DELETE operations exist. |
| Out-of-scope data access | `JIRA_ALLOWED_PROJECTS`: issue keys are validated (`^[A-Za-z][A-Za-z0-9_]*-\d+$`) and project-checked before any call; project and board lists are filtered; create/update/comment/transition verify the project. |
| JQL escape from allowlist | JQL is wrapped as `(<jql>) AND project IN (...)`; a trailing `ORDER BY` is moved outside the parentheses. Parentheses defeat `OR` injection. Empty JQL yields only the project clause. Sprint issues are scoped with the same clause. |
| Path injection | Keys validated; all path segments `url.PathEscape`d; query via `url.Values`. |
| Context flooding | `maxResults` capped at 50; response bodies capped at 10 MiB. |
| TLS / MITM | TLS ≥ 1.2, system roots plus optional `JIRA_CUSTOM_CA_CERT_PATH`; no insecure-skip-verify option. |
| Token forwarding via redirect | Redirects are never followed; 3xx is surfaced as an error. |
| Setup UI abuse | Listens on 127.0.0.1 only (random port); random per-run session token required for the page and every API call (constant-time compare); `Host` must equal the loopback address (anti DNS-rebinding); CSP `default-src 'self'`, `frame-ancestors 'none'`; UI process exits on Close. |
| Process compromise | Static binary, runs as the unprivileged local user, no shell-out. |

Limitation: the allowlist is best-effort for Jira features that bypass JQL (e.g. a sprint issue endpoint on older versions ignoring `jql`); the PAT's own permissions remain the authoritative boundary.
