package jira

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// DefaultMaxResults caps page sizes to protect LLM context windows.
const DefaultMaxResults = 50

var (
	issueKeyRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]*)-[0-9]+$`)
	orderByRe  = regexp.MustCompile(`(?i)(?:^|\s)order\s+by\s+`)
)

// projectOfIssueKey validates an issue key and returns its project key.
func projectOfIssueKey(key string) (string, error) {
	m := issueKeyRe.FindStringSubmatch(strings.TrimSpace(key))
	if m == nil {
		return "", fmt.Errorf("invalid issue key %q", key)
	}
	return strings.ToUpper(m[1]), nil
}

func (c *Client) checkIssueKey(key string) error {
	p, err := projectOfIssueKey(key)
	if err != nil {
		return err
	}
	if !c.cfg.IsProjectAllowed(p) {
		return fmt.Errorf("project %s is not in the allowed project list", p)
	}
	return nil
}

func clampPaging(startAt, maxResults int) (int, int) {
	if startAt < 0 {
		startAt = 0
	}
	if maxResults <= 0 || maxResults > DefaultMaxResults {
		maxResults = DefaultMaxResults
	}
	return startAt, maxResults
}

func pagingQuery(startAt, maxResults int) url.Values {
	s, m := clampPaging(startAt, maxResults)
	q := url.Values{}
	q.Set("startAt", strconv.Itoa(s))
	q.Set("maxResults", strconv.Itoa(m))
	return q
}

// scopeJQL restricts jql to the allowed projects, preserving a trailing ORDER BY.
func (c *Client) scopeJQL(jql string) string {
	if len(c.cfg.AllowedProjects) == 0 {
		return jql
	}
	jql = strings.TrimSpace(jql)
	order := ""
	if loc := orderByRe.FindAllStringIndex(jql, -1); len(loc) > 0 {
		last := loc[len(loc)-1]
		// Only split when the match is outside a quoted string.
		if strings.Count(jql[:last[0]], `"`)%2 == 0 && strings.Count(jql[:last[0]], `'`)%2 == 0 {
			order = " " + strings.TrimSpace(jql[last[0]:])
			jql = strings.TrimSpace(jql[:last[0]])
		}
	}
	quoted := make([]string, len(c.cfg.AllowedProjects))
	for i, p := range c.cfg.AllowedProjects {
		quoted[i] = `"` + p + `"`
	}
	clause := "project IN (" + strings.Join(quoted, ", ") + ")"
	if jql != "" {
		clause = "(" + jql + ") AND " + clause
	}
	return clause + order
}

func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	var u User
	if err := c.get(ctx, "/rest/api/2/myself", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var all []Project
	if err := c.get(ctx, "/rest/api/2/project", nil, &all); err != nil {
		return nil, err
	}
	if len(c.cfg.AllowedProjects) == 0 {
		return all, nil
	}
	out := make([]Project, 0, len(all))
	for _, p := range all {
		if c.cfg.IsProjectAllowed(p.Key) {
			out = append(out, p)
		}
	}
	return out, nil
}

// GetIssue fetches an issue; fields is an optional comma-separated field list.
func (c *Client) GetIssue(ctx context.Context, issueKey, fields string) (*Issue, error) {
	if err := c.checkIssueKey(issueKey); err != nil {
		return nil, err
	}
	q := url.Values{}
	if fields != "" {
		q.Set("fields", fields)
	}
	var is Issue
	if err := c.get(ctx, "/rest/api/2/issue/"+url.PathEscape(strings.TrimSpace(issueKey)), q, &is); err != nil {
		return nil, err
	}
	c.filterRelated(&is)
	return &is, nil
}

func (c *Client) SearchIssues(ctx context.Context, jql string, startAt, maxResults int, fields string) (*SearchResults, error) {
	if strings.TrimSpace(jql) == "" && len(c.cfg.AllowedProjects) == 0 {
		return nil, fmt.Errorf("jql must not be empty")
	}
	q := pagingQuery(startAt, maxResults)
	q.Set("jql", c.scopeJQL(jql))
	if fields != "" {
		q.Set("fields", fields)
	}
	var r SearchResults
	if err := c.get(ctx, "/rest/api/2/search", q, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) GetIssueComments(ctx context.Context, issueKey string, startAt, maxResults int) (*CommentPage, error) {
	if err := c.checkIssueKey(issueKey); err != nil {
		return nil, err
	}
	var p CommentPage
	path := "/rest/api/2/issue/" + url.PathEscape(strings.TrimSpace(issueKey)) + "/comment"
	if err := c.get(ctx, path, pagingQuery(startAt, maxResults), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) GetIssueTransitions(ctx context.Context, issueKey string) (*TransitionsResponse, error) {
	if err := c.checkIssueKey(issueKey); err != nil {
		return nil, err
	}
	var t TransitionsResponse
	path := "/rest/api/2/issue/" + url.PathEscape(strings.TrimSpace(issueKey)) + "/transitions"
	if err := c.get(ctx, path, nil, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Client) ListBoards(ctx context.Context, projectKeyFilter string, startAt, maxResults int) (*BoardPage, error) {
	q := pagingQuery(startAt, maxResults)
	if projectKeyFilter = strings.TrimSpace(projectKeyFilter); projectKeyFilter != "" {
		if !c.cfg.IsProjectAllowed(projectKeyFilter) {
			return nil, fmt.Errorf("project %s is not in the allowed project list", strings.ToUpper(projectKeyFilter))
		}
		q.Set("projectKeyOrId", projectKeyFilter)
	}
	var p BoardPage
	if err := c.get(ctx, "/rest/agile/1.0/board", q, &p); err != nil {
		return nil, err
	}
	if len(c.cfg.AllowedProjects) > 0 {
		kept := p.Values[:0]
		for _, b := range p.Values {
			if b.Location != nil && c.cfg.IsProjectAllowed(b.Location.ProjectKey) {
				kept = append(kept, b)
			}
		}
		p.Values = kept
	}
	return &p, nil
}

func (c *Client) ListBoardSprints(ctx context.Context, boardID int, state string, startAt, maxResults int) (*SprintPage, error) {
	q := pagingQuery(startAt, maxResults)
	if state != "" {
		q.Set("state", state)
	}
	var p SprintPage
	if err := c.get(ctx, "/rest/agile/1.0/board/"+strconv.Itoa(boardID)+"/sprint", q, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) GetSprintIssues(ctx context.Context, sprintID, startAt, maxResults int) (*SearchResults, error) {
	q := pagingQuery(startAt, maxResults)
	if len(c.cfg.AllowedProjects) > 0 {
		q.Set("jql", c.scopeJQL(""))
	}
	var r SearchResults
	if err := c.get(ctx, "/rest/agile/1.0/sprint/"+strconv.Itoa(sprintID)+"/issue", q, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
