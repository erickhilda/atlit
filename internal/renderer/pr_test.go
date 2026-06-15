package renderer

import (
	"strings"
	"testing"

	"github.com/erickhilda/jt/internal/bitbucket"
)

func samplePR() *bitbucket.PullRequest {
	pr := &bitbucket.PullRequest{
		ID:          42,
		Title:       "Fix bug",
		State:       "OPEN",
		Description: "Does the thing.",
		CreatedOn:   "2026-06-09T10:00:00.000000+00:00",
		UpdatedOn:   "2026-06-10T11:00:00.000000+00:00",
	}
	pr.Author.DisplayName = "Alice"
	pr.Source.Branch.Name = "feature/PROJ-1234_x"
	pr.Destination.Branch.Name = "develop"
	pr.Links.HTML.Href = "https://bitbucket.org/acme/widget/pull-requests/42"
	return pr
}

func TestRenderPullRequest(t *testing.T) {
	diffstat := []bitbucket.DiffstatEntry{
		{LinesAdded: 3, LinesRemoved: 1, New: &bitbucket.DiffFile{Path: "a.go"}},
	}
	to := 42
	c1 := bitbucket.Comment{User: bitbucket.Account{DisplayName: "Alice"}, CreatedOn: "2026-06-09T10:00:00+00:00"}
	c1.Content.Raw = "Looks good"
	c1.Inline = &bitbucket.Inline{Path: "a.go", To: &to}
	deleted := bitbucket.Comment{Deleted: true}
	deleted.Content.Raw = "gone"

	out := RenderPullRequest(samplePR(), "acme", "widget", diffstat,
		"diff --git a/a.go b/a.go\n+x\n", []bitbucket.Comment{c1, deleted}, "PROJ-1234", "/home/me/.jt/tickets/PROJ-1234.md")

	wantContains := []string{
		"<!-- jt:meta pr=acme/widget/42 fetched=",
		"# PR #42: Fix bug",
		"| State | OPEN |",
		"| Branch | feature/PROJ-1234_x -> develop |",
		"| Jira | PROJ-1234 |",
		"> Linked ticket file: /home/me/.jt/tickets/PROJ-1234.md",
		"## Diffstat",
		"- a.go (+3 -1)",
		"## Diff",
		"```diff",
		"## Comments (1)",
		"### Alice -- 2026-06-09 - a.go:42",
		"Looks good",
	}
	for _, w := range wantContains {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\n---\n%s", w, out)
		}
	}
	if strings.Contains(out, "gone") {
		t.Errorf("deleted comment should be skipped:\n%s", out)
	}
}

func TestRenderPullRequestNoDiffNoJira(t *testing.T) {
	out := RenderPullRequest(samplePR(), "acme", "widget", nil, "", nil, "", "")

	if strings.Contains(out, "## Diff\n") {
		t.Errorf("no-diff should omit the Diff section:\n%s", out)
	}
	if strings.Contains(out, "| Jira |") {
		t.Errorf("empty jiraKey should omit the Jira row:\n%s", out)
	}
	if strings.Contains(out, "Linked ticket file") {
		t.Errorf("empty ticketPath should omit the pointer:\n%s", out)
	}
	if !strings.Contains(out, "## Comments (0)") || !strings.Contains(out, "*No comments.*") {
		t.Errorf("expected empty comments section:\n%s", out)
	}
}
