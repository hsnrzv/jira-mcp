# Security Policy

## Supported versions

Only the latest release receives security fixes.

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Report privately via GitHub: *Security* tab → *Report a vulnerability*
(GitHub private vulnerability reporting). Please include:

- affected version / commit,
- a description of the issue and its impact,
- steps to reproduce or a proof of concept.

You can expect an acknowledgement within a few days. Once a fix is available it will be released and
the advisory published, crediting you unless you prefer otherwise.

## Scope and threat model

The threat model and mitigations are documented in
[doc/architecture/02-security-model.md](doc/architecture/02-security-model.md).
Never include real Personal Access Tokens, Jira URLs or customer data in reports.
