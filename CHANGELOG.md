# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [1.0.0]

### Added
- MCP server over stdio for Jira Data Center (REST API v2 + Agile 1.0) with PAT authentication.
- 11 read tools, including `jira_get_issue_context` and `jira_get_attachment`.
- 4 opt-in write tools (`JIRA_ENABLE_WRITE_TOOLS`): create, update, comment, transition.
- Project allowlist (`JIRA_ALLOWED_PROJECTS`) with server-side JQL scoping.
- Native setup wizard for macOS and Windows with browser fallback; one-click install into GitHub Copilot.
- Cross-platform release script producing macOS `.app`/`.dmg`, Windows `.exe` and Linux binaries.
