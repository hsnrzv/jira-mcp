package jira

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	// MaxAttachmentBytes caps a single attachment download.
	MaxAttachmentBytes = 5 << 20
	// maxInlineTextBytes caps text inlined per attachment in GetIssueContext.
	maxInlineTextBytes = 100 << 10
	maxInlineTotal     = 400 << 10
	maxCommentPages    = 20
)

var textExts = map[string]bool{
	".txt": true, ".md": true, ".log": true, ".json": true, ".xml": true, ".yaml": true, ".yml": true,
	".csv": true, ".tsv": true, ".sql": true, ".java": true, ".go": true, ".py": true, ".js": true,
	".ts": true, ".html": true, ".css": true, ".properties": true, ".conf": true, ".ini": true,
	".sh": true, ".diff": true, ".patch": true, ".kt": true, ".cs": true, ".toml": true,
}

func isTextAttachment(a Attachment) bool {
	mt := strings.ToLower(a.MimeType)
	if strings.HasPrefix(mt, "text/") || strings.Contains(mt, "json") || strings.Contains(mt, "xml") || strings.Contains(mt, "yaml") {
		return true
	}
	return textExts[strings.ToLower(path.Ext(a.Filename))]
}

// IsImage reports whether the attachment is a raster image the MCP client can render.
func IsImage(a Attachment) bool {
	switch strings.ToLower(a.MimeType) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

// findAttachment returns the metadata of attachment id on issueKey.
func (c *Client) findAttachment(ctx context.Context, issueKey, id string) (*Attachment, error) {
	is, err := c.GetIssue(ctx, issueKey, "attachment")
	if err != nil {
		return nil, err
	}
	for i := range is.Fields.Attachments {
		if is.Fields.Attachments[i].ID == id {
			return &is.Fields.Attachments[i], nil
		}
	}
	return nil, fmt.Errorf("attachment %s not found on issue %s", id, issueKey)
}

// DownloadAttachment fetches attachment id of issueKey (max MaxAttachmentBytes).
// Text-like attachments additionally get Text populated.
func (c *Client) DownloadAttachment(ctx context.Context, issueKey, id string) (*AttachmentContent, error) {
	att, err := c.findAttachment(ctx, issueKey, id)
	if err != nil {
		return nil, err
	}
	return c.download(ctx, *att, MaxAttachmentBytes)
}

func (c *Client) download(ctx context.Context, att Attachment, limit int64) (*AttachmentContent, error) {
	out := &AttachmentContent{Attachment: att}
	base, err := url.Parse(c.cfg.JiraURL)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(att.Content)
	if err != nil || att.Content == "" {
		return nil, fmt.Errorf("attachment %s has no valid content URL", att.ID)
	}
	// Never send the bearer token to another host.
	if u.Scheme != base.Scheme || u.Host != base.Host {
		return nil, fmt.Errorf("attachment %s content URL is not on the configured Jira host", att.ID)
	}
	req, err := c.newRequest(ctx, http.MethodGet, u.RequestURI(), nil, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	resp, err := c.http.Do(req)
	if err != nil {
		if ue, ok := err.(*url.Error); ok {
			err = ue.Err
		}
		return nil, fmt.Errorf("attachment download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &JiraAPIError{StatusCode: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read attachment: %w", err)
	}
	if int64(len(data)) > limit {
		data = data[:limit]
		out.Truncated = true
	}
	out.Data = data
	if isTextAttachment(att) {
		if utf8.Valid(data) || out.Truncated {
			out.Text = strings.ToValidUTF8(string(data), "")
		} else {
			out.Note = "file is not valid UTF-8 text"
		}
	}
	return out, nil
}

// GetIssueContext gathers description, comments, links, subtasks, attachment metadata
// and the contents of small text attachments for an issue.
func (c *Client) GetIssueContext(ctx context.Context, issueKey string) (*IssueContext, error) {
	is, err := c.GetIssue(ctx, issueKey, "")
	if err != nil {
		return nil, err
	}
	c.filterRelated(is)
	res := &IssueContext{Issue: is, Comments: []Comment{}}

	for page := 0; page < maxCommentPages; page++ {
		p, err := c.GetIssueComments(ctx, issueKey, page*DefaultMaxResults, DefaultMaxResults)
		if err != nil {
			return nil, err
		}
		res.Comments = append(res.Comments, p.Comments...)
		if len(p.Comments) == 0 || len(res.Comments) >= p.Total {
			break
		}
	}

	budget := maxInlineTotal
	for _, a := range is.Fields.Attachments {
		ac := AttachmentContent{Attachment: a}
		switch {
		case !isTextAttachment(a):
			ac.Note = "binary attachment; use jira_get_attachment to download"
		case budget <= 0:
			ac.Note = "text not inlined (size budget exceeded); use jira_get_attachment"
		default:
			lim := int64(maxInlineTextBytes)
			if int64(budget) < lim {
				lim = int64(budget)
			}
			d, err := c.download(ctx, a, lim)
			if err != nil {
				ac.Note = "download failed: " + err.Error()
			} else {
				ac.Text, ac.Truncated, ac.Note = d.Text, d.Truncated, d.Note
				budget -= len(d.Data)
			}
		}
		res.Attachments = append(res.Attachments, ac)
	}
	return res, nil
}

// filterRelated drops linked issues, subtasks and parents outside the allowed projects.
func (c *Client) filterRelated(is *Issue) {
	if len(c.cfg.AllowedProjects) == 0 {
		return
	}
	ok := func(l *LinkedIssue) bool {
		if l == nil {
			return false
		}
		p, err := projectOfIssueKey(l.Key)
		return err == nil && c.cfg.IsProjectAllowed(p)
	}
	f := &is.Fields
	if !ok(f.Parent) {
		f.Parent = nil
	}
	subs := f.Subtasks[:0]
	for i := range f.Subtasks {
		if ok(&f.Subtasks[i]) {
			subs = append(subs, f.Subtasks[i])
		}
	}
	f.Subtasks = subs
	links := f.IssueLinks[:0]
	for _, l := range f.IssueLinks {
		if l.InwardIssue != nil && !ok(l.InwardIssue) || l.OutwardIssue != nil && !ok(l.OutwardIssue) {
			continue
		}
		links = append(links, l)
	}
	f.IssueLinks = links
}
