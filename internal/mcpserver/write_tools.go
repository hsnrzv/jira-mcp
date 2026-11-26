package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hsnrzv/jira-mcp/internal/jira"
)

var errNoFields = errors.New("no fields to update")

type createIssueInput struct {
	ProjectKey  string   `json:"project_key" jsonschema:"Project key, e.g. PLAT"`
	Summary     string   `json:"summary" jsonschema:"Issue summary"`
	IssueType   string   `json:"issue_type" jsonschema:"Issue type name, e.g. Task, Bug"`
	Description string   `json:"description,omitempty" jsonschema:"Description in Jira wiki markup"`
	Priority    string   `json:"priority,omitempty" jsonschema:"Priority name"`
	Assignee    string   `json:"assignee,omitempty" jsonschema:"Assignee username"`
	Labels      []string `json:"labels,omitempty" jsonschema:"Labels"`
	Components  []string `json:"components,omitempty" jsonschema:"Component names"`
}

type updateIssueInput struct {
	IssueKey    string    `json:"issue_key" jsonschema:"Issue key, e.g. PLAT-123"`
	Summary     *string   `json:"summary,omitempty" jsonschema:"New summary"`
	Description *string   `json:"description,omitempty" jsonschema:"New description in Jira wiki markup"`
	Priority    *string   `json:"priority,omitempty" jsonschema:"New priority name"`
	Assignee    *string   `json:"assignee,omitempty" jsonschema:"New assignee username"`
	Labels      *[]string `json:"labels,omitempty" jsonschema:"Replacement label set"`
}

type addCommentInput struct {
	IssueKey string `json:"issue_key" jsonschema:"Issue key, e.g. PLAT-123"`
	Body     string `json:"body" jsonschema:"Comment text in Jira wiki markup"`
}

type transitionInput struct {
	IssueKey     string `json:"issue_key" jsonschema:"Issue key, e.g. PLAT-123"`
	TransitionID string `json:"transition_id" jsonschema:"Transition id from jira_get_issue_transitions"`
	Comment      string `json:"comment,omitempty" jsonschema:"Optional comment to add with the transition"`
}

func registerWriteTools(s *mcp.Server, c *jira.Client) {
	addTool(s, "jira_create_issue", "Create a Jira issue.", false,
		func(ctx context.Context, in createIssueInput) (any, error) {
			req := jira.CreateIssueRequest{Fields: jira.CreateIssueFields{
				Project:     jira.Project{Key: in.ProjectKey},
				Summary:     in.Summary,
				IssueType:   jira.Named{Name: in.IssueType},
				Description: in.Description,
				Labels:      in.Labels,
			}}
			if in.Priority != "" {
				req.Fields.Priority = &jira.Named{Name: in.Priority}
			}
			if in.Assignee != "" {
				req.Fields.Assignee = &jira.User{Name: in.Assignee}
			}
			for _, n := range in.Components {
				req.Fields.Components = append(req.Fields.Components, jira.Named{Name: n})
			}
			return c.CreateIssue(ctx, req)
		})
	addTool(s, "jira_update_issue", "Update fields of a Jira issue.", false,
		func(ctx context.Context, in updateIssueInput) (any, error) {
			f := map[string]any{}
			if in.Summary != nil {
				f["summary"] = *in.Summary
			}
			if in.Description != nil {
				f["description"] = *in.Description
			}
			if in.Priority != nil {
				f["priority"] = map[string]string{"name": *in.Priority}
			}
			if in.Assignee != nil {
				f["assignee"] = map[string]string{"name": *in.Assignee}
			}
			if in.Labels != nil {
				f["labels"] = *in.Labels
			}
			if len(f) == 0 {
				return nil, errNoFields
			}
			if err := c.UpdateIssue(ctx, in.IssueKey, jira.UpdateIssueRequest{Fields: f}); err != nil {
				return nil, err
			}
			return map[string]string{"status": "updated", "issue_key": in.IssueKey}, nil
		})
	addTool(s, "jira_add_comment", "Add a comment to a Jira issue.", false,
		func(ctx context.Context, in addCommentInput) (any, error) {
			return c.AddComment(ctx, in.IssueKey, in.Body)
		})
	addTool(s, "jira_transition_issue", "Move an issue through a workflow transition.", false,
		func(ctx context.Context, in transitionInput) (any, error) {
			if err := c.TransitionIssue(ctx, in.IssueKey, in.TransitionID, in.Comment); err != nil {
				return nil, err
			}
			return map[string]string{"status": "transitioned", "issue_key": in.IssueKey}, nil
		})
}
