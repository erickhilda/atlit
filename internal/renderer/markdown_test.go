package renderer

import (
	"strings"
	"testing"

	"github.com/erickhilda/atlit/internal/jira"
)

func TestRenderIssueFullFields(t *testing.T) {
	issue := &jira.Issue{
		Key: "PROJ-123",
		Fields: jira.IssueFields{
			Summary:   "Implement feature X",
			Status:    &jira.Status{Name: "In Progress"},
			IssueType: &jira.IssueType{Name: "Story"},
			Priority:  &jira.Priority{Name: "High"},
			Assignee:  &jira.User{DisplayName: "Alice"},
			Reporter:  &jira.User{DisplayName: "Bob"},
			Labels:    []string{"backend", "api"},
			Created:   "2026-02-01T10:00:00.000+0000",
			Updated:   "2026-02-10T14:30:00.000+0000",
			Description: &jira.ADFDoc{
				Type:    "doc",
				Version: 1,
				Content: []jira.ADFNode{
					{Type: "paragraph", Content: []jira.ADFNode{{Type: "text", Text: "Build feature X."}}},
				},
			},
		},
		Sprint: &jira.Sprint{Name: "Sprint 14"},
		Epic:   &jira.Epic{Key: "PROJ-80", Summary: "Authentication"},
	}

	got := RenderIssue(issue)

	checks := []string{
		"<!-- atlit:meta ticket=PROJ-123",
		"# PROJ-123: Implement feature X",
		"| Status | In Progress |",
		"| Type | Story |",
		"| Priority | High |",
		"| Assignee | Alice |",
		"| Reporter | Bob |",
		"| Sprint | Sprint 14 |",
		"| Epic | PROJ-80: Authentication |",
		"| Labels | backend, api |",
		"| Created | 2026-02-01 |",
		"| Updated | 2026-02-10 |",
		"## Description",
		"Build feature X.",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\nGot:\n%s", want, got)
		}
	}
}

func TestRenderIssueAttachments(t *testing.T) {
	issue := &jira.Issue{
		Key: "PROJ-200",
		Fields: jira.IssueFields{
			Summary: "Ticket with screenshots",
			Attachment: []jira.Attachment{
				{Filename: "screen.png", MimeType: "image/png", Content: "https://acme.atlassian.net/rest/api/3/attachment/content/42"},
				{Filename: "log.txt", MimeType: "text/plain", Content: "https://acme.atlassian.net/rest/api/3/attachment/content/43"},
			},
		},
	}

	got := RenderIssue(issue)
	for _, want := range []string{
		"## Attachments",
		"- screen.png (image/png) - https://acme.atlassian.net/rest/api/3/attachment/content/42",
		"- log.txt (text/plain) - https://acme.atlassian.net/rest/api/3/attachment/content/43",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\nGot:\n%s", want, got)
		}
	}
}

func TestRenderIssueNoAttachmentsSection(t *testing.T) {
	issue := &jira.Issue{Key: "PROJ-201", Fields: jira.IssueFields{Summary: "No files"}}
	if strings.Contains(RenderIssue(issue), "## Attachments") {
		t.Error("expected no Attachments section when there are no attachments")
	}
}

func TestRenderIssueNilAssignee(t *testing.T) {
	issue := &jira.Issue{
		Key: "TEST-1",
		Fields: jira.IssueFields{
			Summary:   "No assignee",
			Status:    &jira.Status{Name: "Open"},
			IssueType: &jira.IssueType{Name: "Bug"},
			Created:   "2026-01-01T00:00:00Z",
			Updated:   "2026-01-01T00:00:00Z",
		},
	}

	got := RenderIssue(issue)
	if !strings.Contains(got, "| Assignee | - |") {
		t.Errorf("expected nil assignee to show '-', got:\n%s", got)
	}
}

func TestRenderIssueNoDescription(t *testing.T) {
	issue := &jira.Issue{
		Key: "TEST-2",
		Fields: jira.IssueFields{
			Summary: "No desc",
			Created: "2026-01-01T00:00:00Z",
			Updated: "2026-01-01T00:00:00Z",
		},
	}

	got := RenderIssue(issue)
	if !strings.Contains(got, "*No description provided.*") {
		t.Errorf("expected no-description placeholder, got:\n%s", got)
	}
}

func TestRenderIssueSubtasksMixedStatus(t *testing.T) {
	issue := &jira.Issue{
		Key: "TEST-3",
		Fields: jira.IssueFields{
			Summary: "With subtasks",
			Created: "2026-01-01T00:00:00Z",
			Updated: "2026-01-01T00:00:00Z",
			Subtasks: []jira.Subtask{
				{Key: "TEST-4", Fields: jira.SubtaskFields{Summary: "Done task", Status: &jira.Status{Name: "Done"}}},
				{Key: "TEST-5", Fields: jira.SubtaskFields{Summary: "Open task", Status: &jira.Status{Name: "To Do"}}},
			},
		},
	}

	got := RenderIssue(issue)
	if !strings.Contains(got, "- [x] TEST-4: Done task (Done)") {
		t.Errorf("expected done subtask with checkbox, got:\n%s", got)
	}
	if !strings.Contains(got, "- [ ] TEST-5: Open task (To Do)") {
		t.Errorf("expected open subtask without checkbox, got:\n%s", got)
	}
}

func TestRenderIssueLinkedIssues(t *testing.T) {
	issue := &jira.Issue{
		Key: "TEST-6",
		Fields: jira.IssueFields{
			Summary: "With links",
			Created: "2026-01-01T00:00:00Z",
			Updated: "2026-01-01T00:00:00Z",
			IssueLinks: []jira.IssueLink{
				{
					Type:         &jira.IssueLinkType{Name: "Blocks", Outward: "blocks", Inward: "is blocked by"},
					OutwardIssue: &jira.LinkedIssue{Key: "TEST-7", Fields: jira.LinkedIssueFields{Summary: "Blocked issue"}},
				},
				{
					Type:        &jira.IssueLinkType{Name: "Blocks", Outward: "blocks", Inward: "is blocked by"},
					InwardIssue: &jira.LinkedIssue{Key: "TEST-8", Fields: jira.LinkedIssueFields{Summary: "Blocking issue"}},
				},
			},
		},
	}

	got := RenderIssue(issue)
	if !strings.Contains(got, "- blocks TEST-7: Blocked issue") {
		t.Errorf("expected outward link, got:\n%s", got)
	}
	if !strings.Contains(got, "- is blocked by TEST-8: Blocking issue") {
		t.Errorf("expected inward link, got:\n%s", got)
	}
}

func TestRenderIssuePullRequests(t *testing.T) {
	issue := &jira.Issue{
		Key: "TEST-PR",
		Fields: jira.IssueFields{
			Summary: "With PRs",
			Created: "2026-01-01T00:00:00Z",
			Updated: "2026-01-01T00:00:00Z",
		},
		PullRequests: []jira.PullRequest{
			{
				ID:             "#42",
				Name:           "Add feature",
				URL:            "https://bitbucket.org/x/repo/pull-requests/42",
				Status:         "MERGED",
				Author:         &jira.DevUser{Name: "alice"},
				Source:         &jira.DevBranch{Branch: "feature/TEST-PR"},
				Destination:    &jira.DevBranch{Branch: "develop"},
				Reviewers:      []jira.DevUser{{Name: "bob", Approved: true}, {Name: "carol", Approved: false}},
				RepositoryName: "acme/widget",
			},
		},
	}

	got := RenderIssue(issue)
	for _, want := range []string{
		"## Pull Requests (1)",
		"- [MERGED] [Add feature](https://bitbucket.org/x/repo/pull-requests/42) (#42)",
		"  - Repo: acme/widget",
		"  - Branch: feature/TEST-PR -> develop",
		"  - Author: alice",
		"  - Approved by: bob",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "carol") {
		t.Errorf("non-approving reviewer should not appear, got:\n%s", got)
	}
}

func TestRenderIssueNoPullRequestsSection(t *testing.T) {
	issue := &jira.Issue{Key: "PROJ-202", Fields: jira.IssueFields{Summary: "No PRs"}}
	if strings.Contains(RenderIssue(issue), "## Pull Requests") {
		t.Errorf("expected no Pull Requests section when none linked")
	}
}

func TestRenderIssueComments(t *testing.T) {
	issue := &jira.Issue{
		Key: "TEST-9",
		Fields: jira.IssueFields{
			Summary: "With comments",
			Created: "2026-01-01T00:00:00Z",
			Updated: "2026-01-01T00:00:00Z",
			Comment: &jira.CommentPage{
				Total: 1,
				Comments: []jira.Comment{
					{
						Author:  &jira.User{DisplayName: "Charlie"},
						Created: "2026-02-10T09:15:00.000+0000",
						Body: &jira.ADFDoc{
							Type:    "doc",
							Version: 1,
							Content: []jira.ADFNode{
								{Type: "paragraph", Content: []jira.ADFNode{{Type: "text", Text: "Looks good!"}}},
							},
						},
					},
				},
			},
		},
	}

	got := RenderIssue(issue)
	if !strings.Contains(got, "## Comments (1)") {
		t.Errorf("expected comments header, got:\n%s", got)
	}
	if !strings.Contains(got, "### Charlie -- 2026-02-10") {
		t.Errorf("expected comment author/date, got:\n%s", got)
	}
	if !strings.Contains(got, "Looks good!") {
		t.Errorf("expected comment body, got:\n%s", got)
	}
}

func TestRenderIssueParent(t *testing.T) {
	issue := &jira.Issue{
		Key: "TEST-10",
		Fields: jira.IssueFields{
			Summary: "Child issue",
			Created: "2026-01-01T00:00:00Z",
			Updated: "2026-01-01T00:00:00Z",
			Parent: &jira.ParentIssue{
				Key:    "TEST-1",
				Fields: jira.ParentIssueFields{Summary: "Parent story"},
			},
		},
	}

	got := RenderIssue(issue)
	if !strings.Contains(got, "| Parent | TEST-1: Parent story |") {
		t.Errorf("expected parent row, got:\n%s", got)
	}
}

func TestRenderIssueSprintAndEpic(t *testing.T) {
	issue := &jira.Issue{
		Key: "TEST-11",
		Fields: jira.IssueFields{
			Summary: "Sprinted issue",
			Created: "2026-01-01T00:00:00Z",
			Updated: "2026-01-01T00:00:00Z",
		},
		Sprint: &jira.Sprint{Name: "Sprint 5"},
		Epic:   &jira.Epic{Key: "EPIC-1"},
	}

	got := RenderIssue(issue)
	if !strings.Contains(got, "| Sprint | Sprint 5 |") {
		t.Errorf("expected sprint row, got:\n%s", got)
	}
	if !strings.Contains(got, "| Epic | EPIC-1 |") {
		t.Errorf("expected epic row (key only), got:\n%s", got)
	}
}

func TestRenderCommentsWithComments(t *testing.T) {
	issue := &jira.Issue{
		Key: "TEST-12",
		Fields: jira.IssueFields{
			Comment: &jira.CommentPage{
				Total: 2,
				Comments: []jira.Comment{
					{
						Author:  &jira.User{DisplayName: "Alice"},
						Created: "2026-02-10T09:00:00.000+0000",
						Body: &jira.ADFDoc{
							Type:    "doc",
							Version: 1,
							Content: []jira.ADFNode{
								{Type: "paragraph", Content: []jira.ADFNode{{Type: "text", Text: "First comment."}}},
							},
						},
					},
					{
						Author:  &jira.User{DisplayName: "Bob"},
						Created: "2026-02-11T10:00:00.000+0000",
						Body: &jira.ADFDoc{
							Type:    "doc",
							Version: 1,
							Content: []jira.ADFNode{
								{Type: "paragraph", Content: []jira.ADFNode{{Type: "text", Text: "Second comment."}}},
							},
						},
					},
				},
			},
		},
	}

	got := RenderComments(issue)
	if !strings.Contains(got, "## Comments (2)") {
		t.Errorf("expected comments header with count 2, got:\n%s", got)
	}
	if !strings.Contains(got, "### Alice -- 2026-02-10") {
		t.Errorf("expected first comment author, got:\n%s", got)
	}
	if !strings.Contains(got, "### Bob -- 2026-02-11") {
		t.Errorf("expected second comment author, got:\n%s", got)
	}
}

func TestRenderCommentsEmpty(t *testing.T) {
	issue := &jira.Issue{
		Key:    "TEST-13",
		Fields: jira.IssueFields{},
	}

	got := RenderComments(issue)
	if !strings.Contains(got, "## Comments (0)") {
		t.Errorf("expected empty comments header, got:\n%s", got)
	}
	if !strings.Contains(got, "*No comments.*") {
		t.Errorf("expected no-comments placeholder, got:\n%s", got)
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2026-02-01T10:00:00.000+0000", "2026-02-01"},
		{"2026-02-01T10:00:00Z", "2026-02-01"},
		{"2026-02-01T10:00:00+05:00", "2026-02-01"},
		{"", "-"},
		{"bad-date", "bad-date"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := formatDate(tt.input)
			if got != tt.want {
				t.Errorf("formatDate(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
