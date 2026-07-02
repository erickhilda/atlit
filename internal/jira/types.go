package jira

import "encoding/json"

// User represents a Jira Cloud user from the /rest/api/3/myself endpoint.
type User struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
	Email       string `json:"emailAddress"`
	Active      bool   `json:"active"`
	TimeZone    string `json:"timeZone"`
}

// Issue is the fully-parsed Jira issue returned by GetIssue.
type Issue struct {
	// ID is the numeric issue id (distinct from Key). Required by the
	// dev-status API to look up linked pull requests.
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Fields IssueFields `json:"fields"`
	Sprint *Sprint     `json:"-"`
	Epic   *Epic       `json:"-"`
	// PullRequests holds development-panel PRs linked to the issue. Populated
	// separately via GetPullRequests (not part of the issue REST payload).
	PullRequests []PullRequest `json:"-"`
}

// IssueRaw is an intermediate type for two-pass JSON decoding.
// The Fields value is kept as raw JSON so we can extract custom fields
// after identifying them via the "names" map from ?expand=names.
type IssueRaw struct {
	ID     string            `json:"id"`
	Key    string            `json:"key"`
	Fields json.RawMessage   `json:"fields"`
	Names  map[string]string `json:"names"`
}

// IssueFields holds the standard fields of a Jira issue.
type IssueFields struct {
	Summary     string       `json:"summary"`
	Description *ADFDoc      `json:"description"`
	Status      *Status      `json:"status"`
	IssueType   *IssueType   `json:"issuetype"`
	Priority    *Priority    `json:"priority"`
	Assignee    *User        `json:"assignee"`
	Reporter    *User        `json:"reporter"`
	Labels      []string     `json:"labels"`
	Created     string       `json:"created"`
	Updated     string       `json:"updated"`
	Comment     *CommentPage `json:"comment"`
	Subtasks    []Subtask    `json:"subtasks"`
	IssueLinks  []IssueLink  `json:"issuelinks"`
	Parent      *ParentIssue `json:"parent"`
	Attachment  []Attachment `json:"attachment"`
}

// Attachment is a file attached to a Jira issue. Content is an authenticated
// download URL (/rest/api/3/attachment/content/{id}).
type Attachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Content  string `json:"content"`
	Size     int    `json:"size"`
	Created  string `json:"created"`
}

// Status represents the issue status.
type Status struct {
	Name string `json:"name"`
}

// IssueType represents the issue type (Story, Bug, Task, etc.).
type IssueType struct {
	Name string `json:"name"`
}

// Priority represents the issue priority.
type Priority struct {
	Name string `json:"name"`
}

// CommentPage holds a page of comments from the Jira issue response.
type CommentPage struct {
	Total    int       `json:"total"`
	Comments []Comment `json:"comments"`
}

// Comment is a single issue comment.
type Comment struct {
	Author  *User   `json:"author"`
	Body    *ADFDoc `json:"body"`
	Created string  `json:"created"`
}

// Subtask represents a subtask of the issue.
type Subtask struct {
	Key    string        `json:"key"`
	Fields SubtaskFields `json:"fields"`
}

// SubtaskFields holds the fields we care about for subtasks.
type SubtaskFields struct {
	Summary string  `json:"summary"`
	Status  *Status `json:"status"`
}

// IssueLink represents a link between two issues.
type IssueLink struct {
	Type         *IssueLinkType `json:"type"`
	InwardIssue  *LinkedIssue   `json:"inwardIssue"`
	OutwardIssue *LinkedIssue   `json:"outwardIssue"`
}

// IssueLinkType describes the relationship (e.g. "blocks"/"is blocked by").
type IssueLinkType struct {
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

// LinkedIssue is a minimal issue representation used in links.
type LinkedIssue struct {
	Key    string            `json:"key"`
	Fields LinkedIssueFields `json:"fields"`
}

// LinkedIssueFields holds the fields for a linked issue.
type LinkedIssueFields struct {
	Summary string  `json:"summary"`
	Status  *Status `json:"status"`
}

// ParentIssue represents the parent of a sub-task or child issue.
type ParentIssue struct {
	Key    string            `json:"key"`
	Fields ParentIssueFields `json:"fields"`
}

// ParentIssueFields holds the fields for a parent issue.
type ParentIssueFields struct {
	Summary string  `json:"summary"`
	Status  *Status `json:"status"`
}

// Sprint represents a Jira sprint (extracted from custom fields).
type Sprint struct {
	Name string `json:"name"`
}

// Epic represents a Jira epic (extracted from custom fields).
type Epic struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
}

// SearchResult holds the response from a JQL search via /rest/api/3/search/jql.
type SearchResult struct {
	Issues        []Issue `json:"issues"`
	NextPageToken string  `json:"nextPageToken,omitempty"`
	IsLast        bool    `json:"isLast"`
}

// PullRequest is a development-panel pull request linked to a Jira issue,
// as returned by the dev-status API (/rest/dev-status/...). It is a flattened
// view of the data the Jira UI shows under "Development".
type PullRequest struct {
	ID          string     `json:"id"`     // display id, e.g. "#123"
	Name        string     `json:"name"`   // PR title
	URL         string     `json:"url"`    // link to the PR
	Status      string     `json:"status"` // OPEN | MERGED | DECLINED
	LastUpdate  string     `json:"lastUpdate"`
	Author      *DevUser   `json:"author"`
	Source      *DevBranch `json:"source"`
	Destination *DevBranch `json:"destination"`
	Reviewers   []DevUser  `json:"reviewers"`
	// RepositoryName is the human-readable repo, e.g. "acme/widget" for
	// Bitbucket Cloud (workspace/repo slugs).
	RepositoryName string `json:"repositoryName"`
	// AppType is the source application (e.g. "bitbucket", "github"). Set by
	// the client from the dev-status query, not present in the PR JSON itself.
	AppType string `json:"-"`
}

// DevUser is an author or reviewer in the dev-status payload.
type DevUser struct {
	Name     string `json:"name"`
	Approved bool   `json:"approved"`
}

// DevBranch is a source/destination branch reference in a dev-status PR. Only
// the branch name is captured; the branch URL is intentionally omitted (we do
// not render it, and there is no reason to hold it in memory).
type DevBranch struct {
	Branch string `json:"branch"`
}

// devStatusSummary is the response from /rest/dev-status/latest/issue/summary.
// It tells us how many PRs exist and which application types host them, so we
// can avoid hardcoding a provider and skip the detail call when there are none.
type devStatusSummary struct {
	Summary struct {
		PullRequest struct {
			Overall struct {
				Count int `json:"count"`
			} `json:"overall"`
			ByInstanceType map[string]struct {
				Count int    `json:"count"`
				Name  string `json:"name"`
			} `json:"byInstanceType"`
		} `json:"pullrequest"`
	} `json:"summary"`
}

// devStatusDetail is the response from /rest/dev-status/latest/issue/detail.
type devStatusDetail struct {
	Detail []struct {
		PullRequests []PullRequest `json:"pullRequests"`
	} `json:"detail"`
}

// ADFDoc is the top-level Atlassian Document Format document.
type ADFDoc struct {
	Type    string    `json:"type"`
	Version int       `json:"version"`
	Content []ADFNode `json:"content"`
}

// ADFNode is a node in the ADF tree (paragraph, heading, text, etc.).
type ADFNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text,omitempty"`
	Content []ADFNode      `json:"content,omitempty"`
	Marks   []ADFMark      `json:"marks,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// ADFMark represents inline formatting (bold, italic, link, etc.).
type ADFMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}
