# 03 — API Mapping

| MCP tool | Method & endpoint | Parameters | Go model | Class |
|---|---|---|---|---|
| `jira_get_myself` | GET `/rest/api/2/myself` | – | `User` | Read |
| `jira_list_projects` | GET `/rest/api/2/project` | – | `[]Project` | Read |
| `jira_get_issue` | GET `/rest/api/2/issue/{key}` | `issue_key`, `fields` | `Issue` | Read |
| `jira_search_issues` | GET `/rest/api/2/search` | `jql`, `start_at`, `max_results`, `fields` | `SearchResults` | Read |
| `jira_get_issue_context` | GET `/rest/api/2/issue/{key}`, `/comment` (paged), attachment content URLs | `issue_key` | `IssueContext` | Read |
| `jira_get_attachment` | GET `/rest/api/2/issue/{key}?fields=attachment`, then the attachment `content` URL | `issue_key`, `attachment_id` | metadata + text/image | Read |
| `jira_get_issue_comments` | GET `/rest/api/2/issue/{key}/comment` | `issue_key`, paging | `CommentPage` | Read |
| `jira_get_issue_transitions` | GET `/rest/api/2/issue/{key}/transitions` | `issue_key` | `TransitionsResponse` | Read |
| `jira_list_boards` | GET `/rest/agile/1.0/board` | `project_key`, paging | `BoardPage` | Read |
| `jira_list_board_sprints` | GET `/rest/agile/1.0/board/{id}/sprint` | `board_id`, `state`, paging | `SprintPage` | Read |
| `jira_get_sprint_issues` | GET `/rest/agile/1.0/sprint/{id}/issue` | `sprint_id`, paging | `SearchResults` | Read |
| `jira_create_issue` | POST `/rest/api/2/issue` | project, summary, type, … | `CreateIssueRequest/Response` | Write |
| `jira_update_issue` | PUT `/rest/api/2/issue/{key}` | `issue_key`, fields | `UpdateIssueRequest` | Write |
| `jira_add_comment` | POST `/rest/api/2/issue/{key}/comment` | `issue_key`, `body` | `AddCommentRequest`, `Comment` | Write |
| `jira_transition_issue` | POST `/rest/api/2/issue/{key}/transitions` | `issue_key`, `transition_id`, `comment` | `TransitionIssueRequest` | Write |

Text fields use Jira wiki markup (API v2), not ADF. Errors: non-2xx responses decode to `JiraAPIError` (`errorMessages`, `errors`).

Attachment downloads: the content URL must be on the configured Jira host (the PAT is never sent elsewhere), redirects are refused, size is capped at 5 MB (100 KB per file / 400 KB total when inlined by `jira_get_issue_context`), and the attachment must belong to the given issue so the project allow-list applies. Linked issues, parents and subtasks outside the allow-list are filtered out.
