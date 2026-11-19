// Package jira is a minimal client for Jira Data Center REST API v2 and Agile API v1.0.
package jira

import (
	"encoding/json"
	"fmt"
	"strings"
)

type User struct {
	Key          string `json:"key,omitempty"`
	Name         string `json:"name,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
	Active       bool   `json:"active"`
}

type Named struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// StatusCategory's id is a JSON number in Jira DC, unlike most other ids.
type StatusCategory struct {
	ID   json.Number `json:"id,omitempty"`
	Key  string      `json:"key,omitempty"`
	Name string      `json:"name,omitempty"`
}

type Status struct {
	ID             string          `json:"id,omitempty"`
	Name           string          `json:"name,omitempty"`
	StatusCategory *StatusCategory `json:"statusCategory,omitempty"`
}

type Project struct {
	ID   string `json:"id,omitempty"`
	Key  string `json:"key,omitempty"`
	Name string `json:"name,omitempty"`
}

type IssueFields struct {
	Summary     string   `json:"summary,omitempty"`
	Description string   `json:"description,omitempty"` // Jira wiki markup
	Status      *Status  `json:"status,omitempty"`
	IssueType   *Named   `json:"issuetype,omitempty"`
	Priority    *Named   `json:"priority,omitempty"`
	Assignee    *User    `json:"assignee,omitempty"`
	Reporter    *User    `json:"reporter,omitempty"`
	Project     *Project `json:"project,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Components  []Named  `json:"components,omitempty"`
	FixVersions []Named  `json:"fixVersions,omitempty"`
	Created     string   `json:"created,omitempty"`
	Updated     string   `json:"updated,omitempty"`
	Resolution  *Named   `json:"resolution,omitempty"`

	Parent      *LinkedIssue  `json:"parent,omitempty"`
	Subtasks    []LinkedIssue `json:"subtasks,omitempty"`
	IssueLinks  []IssueLink   `json:"issuelinks,omitempty"`
	Attachments []Attachment  `json:"attachment,omitempty"`
}

// LinkedIssue is the compact issue representation Jira embeds in links, parents and subtasks.
type LinkedIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary   string  `json:"summary,omitempty"`
		Status    *Status `json:"status,omitempty"`
		IssueType *Named  `json:"issuetype,omitempty"`
	} `json:"fields"`
}

type IssueLinkType struct {
	Name    string `json:"name,omitempty"`
	Inward  string `json:"inward,omitempty"`
	Outward string `json:"outward,omitempty"`
}

type IssueLink struct {
	ID           string        `json:"id,omitempty"`
	Type         IssueLinkType `json:"type"`
	InwardIssue  *LinkedIssue  `json:"inwardIssue,omitempty"`
	OutwardIssue *LinkedIssue  `json:"outwardIssue,omitempty"`
}

type Attachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Author   *User  `json:"author,omitempty"`
	Created  string `json:"created,omitempty"`
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType,omitempty"`
	Content  string `json:"content,omitempty"`
}

// AttachmentContent is a downloaded attachment. Text is set for text-like files only.
type AttachmentContent struct {
	Attachment
	Text      string `json:"text,omitempty"`
	Data      []byte `json:"-"`
	Truncated bool   `json:"truncated,omitempty"`
	Note      string `json:"note,omitempty"`
}

// IssueContext bundles everything needed to understand a ticket.
type IssueContext struct {
	Issue       *Issue              `json:"issue"`
	Comments    []Comment           `json:"comments"`
	Attachments []AttachmentContent `json:"attachments,omitempty"`
}

type Issue struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Self   string      `json:"self,omitempty"`
	Fields IssueFields `json:"fields"`
}

type SearchResults struct {
	StartAt    int     `json:"startAt"`
	MaxResults int     `json:"maxResults"`
	Total      int     `json:"total"`
	Issues     []Issue `json:"issues"`
}

type Comment struct {
	ID      string `json:"id"`
	Author  *User  `json:"author,omitempty"`
	Body    string `json:"body"`
	Created string `json:"created,omitempty"`
	Updated string `json:"updated,omitempty"`
}

type CommentPage struct {
	StartAt    int       `json:"startAt"`
	MaxResults int       `json:"maxResults"`
	Total      int       `json:"total"`
	Comments   []Comment `json:"comments"`
}

type Transition struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	To   *Status `json:"to,omitempty"`
}

type TransitionsResponse struct {
	Transitions []Transition `json:"transitions"`
}

type Board struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location *struct {
		ProjectKey string `json:"projectKey,omitempty"`
		Name       string `json:"name,omitempty"`
	} `json:"location,omitempty"`
}

type BoardPage struct {
	StartAt    int     `json:"startAt"`
	MaxResults int     `json:"maxResults"`
	Total      int     `json:"total"`
	IsLast     bool    `json:"isLast"`
	Values     []Board `json:"values"`
}

type Sprint struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	State        string `json:"state"`
	StartDate    string `json:"startDate,omitempty"`
	EndDate      string `json:"endDate,omitempty"`
	CompleteDate string `json:"completeDate,omitempty"`
	Goal         string `json:"goal,omitempty"`
}

type SprintPage struct {
	StartAt    int      `json:"startAt"`
	MaxResults int      `json:"maxResults"`
	IsLast     bool     `json:"isLast"`
	Values     []Sprint `json:"values"`
}

// Write payloads.

type CreateIssueFields struct {
	Project     Project  `json:"project"`
	Summary     string   `json:"summary"`
	IssueType   Named    `json:"issuetype"`
	Description string   `json:"description,omitempty"`
	Priority    *Named   `json:"priority,omitempty"`
	Assignee    *User    `json:"assignee,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Components  []Named  `json:"components,omitempty"`
}

type CreateIssueRequest struct {
	Fields CreateIssueFields `json:"fields"`
}

type CreateIssueResponse struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

// UpdateIssueRequest carries a partial field update.
type UpdateIssueRequest struct {
	Fields map[string]any `json:"fields"`
}

type TransitionRef struct {
	ID string `json:"id"`
}

type CommentBody struct {
	Body string `json:"body"`
}

type TransitionIssueRequest struct {
	Transition TransitionRef           `json:"transition"`
	Update     map[string][]CommentAdd `json:"update,omitempty"`
}

type CommentAdd struct {
	Add CommentBody `json:"add"`
}

type AddCommentRequest = CommentBody

// JiraAPIError is a non-2xx response from Jira.
type JiraAPIError struct {
	StatusCode    int               `json:"-"`
	ErrorMessages []string          `json:"errorMessages"`
	Errors        map[string]string `json:"errors"`
}

func (e *JiraAPIError) Error() string {
	var parts []string
	parts = append(parts, e.ErrorMessages...)
	for k, v := range e.Errors {
		parts = append(parts, k+": "+v)
	}
	msg := strings.Join(parts, "; ")
	if msg == "" {
		msg = "no details"
	}
	return fmt.Sprintf("jira API error (HTTP %d): %s", e.StatusCode, msg)
}
