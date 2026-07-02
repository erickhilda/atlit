package renderer

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/erickhilda/atlit/internal/bitbucket"
)

// RenderPullRequest produces a self-contained markdown document for a Bitbucket
// pull request, intended as code-review context for an LLM agent.
//
// diff is embedded only when non-empty (callers pass "" for --no-diff).
// jiraKey and ticketPath are optional; when set they cross-link the PR to its
// Jira ticket and the local ticket file respectively.
func RenderPullRequest(
	pr *bitbucket.PullRequest,
	workspace, repo string,
	diffstat []bitbucket.DiffstatEntry,
	diff string,
	comments []bitbucket.Comment,
	jiraKey, ticketPath string,
) string {
	var b strings.Builder

	now := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(&b, "<!-- atlit:meta pr=%s/%s/%d fetched=%s -->\n", workspace, repo, pr.ID, now)
	fmt.Fprintf(&b, "# PR #%d: %s\n\n", pr.ID, pr.Title)

	// Metadata table.
	b.WriteString("| Field | Value |\n")
	b.WriteString("|-------|-------|\n")
	writeRow(&b, "State", pr.State)
	writeRow(&b, "Author", pr.Author.DisplayName)
	if approvers := approvedBy(pr.Participants); approvers != "" {
		writeRow(&b, "Approved by", approvers)
	}
	writeRow(&b, "Branch", pr.Source.Branch.Name+" -> "+pr.Destination.Branch.Name)
	if jiraKey != "" {
		writeRow(&b, "Jira", jiraKey)
	}
	writeRow(&b, "URL", pr.Links.HTML.Href)
	writeRow(&b, "Created", formatDate(pr.CreatedOn))
	writeRow(&b, "Updated", formatDate(pr.UpdatedOn))
	b.WriteString("\n")

	if ticketPath != "" {
		fmt.Fprintf(&b, "> Linked ticket file: %s\n\n", ticketPath)
	}

	// Description (already markdown; no ADF conversion needed).
	b.WriteString("## Description\n\n")
	if strings.TrimSpace(pr.Description) != "" {
		b.WriteString(strings.TrimRight(pr.Description, "\n"))
		b.WriteString("\n\n")
	} else {
		b.WriteString("*No description provided.*\n\n")
	}

	// Diffstat (always included when present).
	if len(diffstat) > 0 {
		b.WriteString("## Diffstat\n\n")
		for _, d := range diffstat {
			fmt.Fprintf(&b, "- %s (+%d -%d)\n", d.Path(), d.LinesAdded, d.LinesRemoved)
		}
		b.WriteString("\n")
	}

	// Diff (omitted for --no-diff).
	if strings.TrimSpace(diff) != "" {
		b.WriteString("## Diff\n\n")
		b.WriteString("```diff\n")
		b.WriteString(strings.TrimRight(diff, "\n"))
		b.WriteString("\n```\n\n")
	}

	// Comments (skip deleted/empty; annotate inline location).
	var cb strings.Builder
	rendered := 0
	for _, cm := range comments {
		if cm.Deleted || strings.TrimSpace(cm.Content.Raw) == "" {
			continue
		}
		loc := ""
		if cm.Inline != nil && cm.Inline.Path != "" {
			loc = " - " + cm.Inline.Path
			if cm.Inline.To != nil {
				loc += ":" + strconv.Itoa(*cm.Inline.To)
			}
		}
		fmt.Fprintf(&cb, "### %s -- %s%s\n\n", cm.User.DisplayName, formatDate(cm.CreatedOn), loc)
		cb.WriteString(strings.TrimRight(cm.Content.Raw, "\n"))
		cb.WriteString("\n\n")
		rendered++
	}
	fmt.Fprintf(&b, "## Comments (%d)\n\n", rendered)
	if rendered == 0 {
		b.WriteString("*No comments.*\n\n")
	} else {
		b.WriteString(cb.String())
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

// approvedBy returns a comma-joined list of the display names of everyone who
// approved the pull request. An empty result drops the metadata row entirely.
func approvedBy(participants []bitbucket.Participant) string {
	var names []string
	for _, p := range participants {
		if p.Approved && p.User.DisplayName != "" {
			names = append(names, p.User.DisplayName)
		}
	}
	return strings.Join(names, ", ")
}
