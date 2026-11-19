package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/internal/jira-mcp/internal/jira"
)

type emptyInput struct{}

type getIssueInput struct {
	IssueKey string `json:"issue_key" jsonschema:"Issue key, e.g. PLAT-123"`
	Fields   string `json:"fields,omitempty" jsonschema:"Optional comma-separated field list, e.g. summary,status,description"`
}

type searchInput struct {
	JQL        string `json:"jql" jsonschema:"JQL query"`
	StartAt    int    `json:"start_at,omitempty" jsonschema:"Zero-based index of first result"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"Page size (max 50)"`
	Fields     string `json:"fields,omitempty" jsonschema:"Optional comma-separated field list"`
}

type commentsInput struct {
	IssueKey   string `json:"issue_key" jsonschema:"Issue key, e.g. PLAT-123"`
	StartAt    int    `json:"start_at,omitempty" jsonschema:"Zero-based index of first result"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"Page size (max 50)"`
}

type issueKeyInput struct {
	IssueKey string `json:"issue_key" jsonschema:"Issue key, e.g. PLAT-123"`
}

type attachmentInput struct {
	IssueKey     string `json:"issue_key" jsonschema:"Issue key the attachment belongs to, e.g. PLAT-123"`
	AttachmentID string `json:"attachment_id" jsonschema:"Attachment id from the issue's attachment list"`
}

type listBoardsInput struct {
	ProjectKey string `json:"project_key,omitempty" jsonschema:"Optional project key or id filter"`
	StartAt    int    `json:"start_at,omitempty" jsonschema:"Zero-based index of first result"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"Page size (max 50)"`
}

type boardSprintsInput struct {
	BoardID    int    `json:"board_id" jsonschema:"Agile board id"`
	State      string `json:"state,omitempty" jsonschema:"Comma-separated sprint states: future, active, closed"`
	StartAt    int    `json:"start_at,omitempty" jsonschema:"Zero-based index of first result"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"Page size (max 50)"`
}

type sprintIssuesInput struct {
	SprintID   int `json:"sprint_id" jsonschema:"Sprint id"`
	StartAt    int `json:"start_at,omitempty" jsonschema:"Zero-based index of first result"`
	MaxResults int `json:"max_results,omitempty" jsonschema:"Page size (max 50)"`
}

func registerReadTools(s *mcp.Server, c *jira.Client) {
	addTool(s, "jira_get_myself", "Get the Jira user the configured token belongs to.", true,
		func(ctx context.Context, _ emptyInput) (any, error) { return c.GetCurrentUser(ctx) })
	addTool(s, "jira_list_projects", "List accessible Jira projects.", true,
		func(ctx context.Context, _ emptyInput) (any, error) { return c.ListProjects(ctx) })
	addTool(s, "jira_get_issue", "Get a Jira issue by key.", true,
		func(ctx context.Context, in getIssueInput) (any, error) {
			return c.GetIssue(ctx, in.IssueKey, in.Fields)
		})
	addTool(s, "jira_get_issue_context",
		"Get everything needed to understand a ticket in one call: description, all fields, linked issues, parent/subtasks, all comments, attachment list and the contents of small text attachments. Call this first for prompts like 'summarize PLAT-123' or 'implement PLAT-123'.", true,
		func(ctx context.Context, in issueKeyInput) (any, error) {
			return c.GetIssueContext(ctx, in.IssueKey)
		})
	registerAttachmentTool(s, c)
	addTool(s, "jira_search_issues", "Search issues with JQL.", true,
		func(ctx context.Context, in searchInput) (any, error) {
			return c.SearchIssues(ctx, in.JQL, in.StartAt, in.MaxResults, in.Fields)
		})
	addTool(s, "jira_get_issue_comments", "List comments on an issue.", true,
		func(ctx context.Context, in commentsInput) (any, error) {
			return c.GetIssueComments(ctx, in.IssueKey, in.StartAt, in.MaxResults)
		})
	addTool(s, "jira_get_issue_transitions", "List workflow transitions currently available for an issue.", true,
		func(ctx context.Context, in issueKeyInput) (any, error) {
			return c.GetIssueTransitions(ctx, in.IssueKey)
		})
	addTool(s, "jira_list_boards", "List Agile boards.", true,
		func(ctx context.Context, in listBoardsInput) (any, error) {
			return c.ListBoards(ctx, in.ProjectKey, in.StartAt, in.MaxResults)
		})
	addTool(s, "jira_list_board_sprints", "List sprints of an Agile board.", true,
		func(ctx context.Context, in boardSprintsInput) (any, error) {
			return c.ListBoardSprints(ctx, in.BoardID, in.State, in.StartAt, in.MaxResults)
		})
	addTool(s, "jira_get_sprint_issues", "List issues in a sprint.", true,
		func(ctx context.Context, in sprintIssuesInput) (any, error) {
			return c.GetSprintIssues(ctx, in.SprintID, in.StartAt, in.MaxResults)
		})
}

// registerAttachmentTool serves text attachments as text, images as image content and
// other files as metadata only.
func registerAttachmentTool(s *mcp.Server, c *jira.Client) {
	t := &mcp.Tool{
		Name:        "jira_get_attachment",
		Description: "Download an issue attachment (max 5 MB): text files as text, images as images, other types as metadata only.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
	mcp.AddTool(s, t, func(ctx context.Context, _ *mcp.CallToolRequest, in attachmentInput) (*mcp.CallToolResult, any, error) {
		a, err := c.DownloadAttachment(ctx, in.IssueKey, in.AttachmentID)
		if err != nil {
			return nil, nil, err
		}
		meta, _ := json.Marshal(a)
		content := []mcp.Content{&mcp.TextContent{Text: string(meta)}}
		if a.Text == "" && jira.IsImage(a.Attachment) && !a.Truncated {
			content = append(content, &mcp.ImageContent{Data: a.Data, MIMEType: a.MimeType})
		}
		return &mcp.CallToolResult{Content: content}, nil, nil
	})
}
