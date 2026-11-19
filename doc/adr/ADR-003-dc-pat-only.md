# ADR-003: Jira Data Center with PAT only

**Status:** Accepted

## Context
Our Jira is on-premise Data Center. Cloud uses `/rest/api/3`, Atlassian Document Format and email+token Basic auth.

## Decision
Support only Data Center REST API v2 and Agile 1.0, authenticating with `Authorization: Bearer <PAT>`. No Cloud, no Basic auth, no Confluence.

## Consequences
+ Simple plain-text (wiki markup) models; one auth path to secure.
− Not usable against Jira Cloud.

## Alternatives Rejected
- Basic auth with username/password (credential exposure).
- Dual Cloud/DC support (ADF complexity, larger attack surface).
