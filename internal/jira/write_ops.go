package jira

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

func (c *Client) checkWriteProject(key string) error {
	if !c.cfg.EnableWriteTools {
		return ErrWriteDisabled
	}
	if !c.cfg.IsProjectAllowed(key) {
		return fmt.Errorf("project %s is not in the allowed project list", strings.ToUpper(key))
	}
	return nil
}

func (c *Client) checkWriteIssue(issueKey string) error {
	if !c.cfg.EnableWriteTools {
		return ErrWriteDisabled
	}
	p, err := projectOfIssueKey(issueKey)
	if err != nil {
		return err
	}
	return c.checkWriteProject(p)
}

func (c *Client) CreateIssue(ctx context.Context, req CreateIssueRequest) (*CreateIssueResponse, error) {
	if err := c.checkWriteProject(req.Fields.Project.Key); err != nil {
		return nil, err
	}
	var out CreateIssueResponse
	if err := c.doWrite(ctx, "POST", "/rest/api/2/issue", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateIssue(ctx context.Context, issueKey string, req UpdateIssueRequest) error {
	if err := c.checkWriteIssue(issueKey); err != nil {
		return err
	}
	return c.doWrite(ctx, "PUT", "/rest/api/2/issue/"+url.PathEscape(strings.TrimSpace(issueKey)), req, nil)
}

func (c *Client) AddComment(ctx context.Context, issueKey, body string) (*Comment, error) {
	if err := c.checkWriteIssue(issueKey); err != nil {
		return nil, err
	}
	var out Comment
	path := "/rest/api/2/issue/" + url.PathEscape(strings.TrimSpace(issueKey)) + "/comment"
	if err := c.doWrite(ctx, "POST", path, AddCommentRequest{Body: body}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) TransitionIssue(ctx context.Context, issueKey, transitionID, comment string) error {
	if err := c.checkWriteIssue(issueKey); err != nil {
		return err
	}
	req := TransitionIssueRequest{Transition: TransitionRef{ID: transitionID}}
	if comment != "" {
		req.Update = map[string][]CommentAdd{"comment": {{Add: CommentBody{Body: comment}}}}
	}
	path := "/rest/api/2/issue/" + url.PathEscape(strings.TrimSpace(issueKey)) + "/transitions"
	return c.doWrite(ctx, "POST", path, req, nil)
}
