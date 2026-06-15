package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/erickhilda/jt/internal/config"
	"github.com/erickhilda/jt/internal/jira"
	"github.com/erickhilda/jt/internal/renderer"
	"github.com/erickhilda/jt/internal/store"
	"github.com/spf13/cobra"
)

var pullCmd = &cobra.Command{
	Use:   "pull <TICKET-KEY>",
	Short: "Fetch a Jira ticket and save as markdown",
	Long:  "Fetches a Jira issue via REST API, converts it to markdown, and saves it locally.",
	Args:  cobra.ExactArgs(1),
	RunE:  runPull,
}

func init() {
	pullCmd.Flags().Bool("comments-only", false, "Only update the comments section")
	pullCmd.Flags().Bool("dry-run", false, "Show what would change without saving")
	rootCmd.AddCommand(pullCmd)
}

func runPull(cmd *cobra.Command, args []string) error {
	ticketKey := strings.ToUpper(strings.TrimSpace(args[0]))
	commentsOnly, _ := cmd.Flags().GetBool("comments-only")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	token, err := config.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("retrieving token: %w", err)
	}

	client := jira.NewClient(cfg.Instance, cfg.Email, token)

	// --comments-only always fetches comments regardless of config.
	fetchComments := cfg.ShouldFetchComments() || commentsOnly
	issue, err := client.GetIssueWithFields(ticketKey, issueFieldsFor(fetchComments))
	if err != nil {
		if errors.Is(err, jira.ErrNotFound) {
			return fmt.Errorf("ticket %s not found", ticketKey)
		}
		if errors.Is(err, jira.ErrUnauthorized) {
			return fmt.Errorf("authentication failed: %w", err)
		}
		return fmt.Errorf("fetching ticket: %w", err)
	}

	canonicalKey := issue.Key

	if commentsOnly {
		return pullCommentsOnly(cfg, issue, canonicalKey, dryRun)
	}

	content := renderer.RenderIssue(issue)

	// Preserve existing "## My Notes" (and "## Comments" when we didn't fetch
	// them) so local history survives re-pulls.
	if existing, err := store.Load(cfg.TicketsDir, canonicalKey); err == nil {
		content = preserveSections(existing, content, fetchComments)
	}

	if dryRun {
		return showDryRun(cfg, canonicalKey, content)
	}

	if err := store.Save(cfg.TicketsDir, canonicalKey, content); err != nil {
		return fmt.Errorf("saving ticket: %w", err)
	}

	path, _ := store.TicketPath(cfg.TicketsDir, canonicalKey)
	fmt.Printf("Saved %s to %s\n", canonicalKey, path)
	return nil
}

// preserveNotes appends the "## My Notes" section from oldContent into newContent.
func preserveNotes(oldContent, newContent string) string {
	notes := store.ExtractNotes(oldContent)
	if notes != "" {
		return strings.TrimRight(newContent, "\n") + "\n\n" + notes
	}
	return newContent
}

// preserveSections preserves user-editable sections from oldContent when
// rendering newContent. Always preserves "## My Notes". When fetchComments is
// false the remote issue has no comments, so we also preserve any existing
// "## Comments" block rather than dropping it from the file. Order: Comments
// before Notes (matching how RenderIssue emits them).
func preserveSections(oldContent, newContent string, fetchComments bool) string {
	if !fetchComments {
		if comments := store.ExtractSection(oldContent, "## Comments"); comments != "" {
			newContent = strings.TrimRight(newContent, "\n") + "\n\n" + comments
		}
	}
	return preserveNotes(oldContent, newContent)
}

// issueFieldsFor returns the Jira "fields" query value for a fetch. Empty
// string means "server default"; "*all,-comment" omits comments.
func issueFieldsFor(fetchComments bool) string {
	if fetchComments {
		return ""
	}
	return "*all,-comment"
}

func pullCommentsOnly(cfg *config.Config, issue *jira.Issue, key string, dryRun bool) error {
	existing, err := store.Load(cfg.TicketsDir, key)
	if err != nil {
		return fmt.Errorf("no local file for %s; run 'jt pull %s' first", key, key)
	}

	newComments := renderer.RenderComments(issue)
	content := store.ReplaceSection(existing, "## Comments", newComments)

	if dryRun {
		return showDryRun(cfg, key, content)
	}

	if err := store.Save(cfg.TicketsDir, key, content); err != nil {
		return fmt.Errorf("saving ticket: %w", err)
	}

	path, _ := store.TicketPath(cfg.TicketsDir, key)
	fmt.Printf("Updated comments for %s in %s\n", key, path)
	return nil
}

func showDryRun(cfg *config.Config, key, newContent string) error {
	return showDryRunDir(cfg.TicketsDir, key, newContent)
}

// showDryRunDir prints a line-by-line diff of newContent against the existing
// file at <dir>/<key>.md, used by both ticket pull and PR fetch.
func showDryRunDir(dir, key, newContent string) error {
	existing, err := store.Load(dir, key)
	if err != nil {
		// No existing file -- show the full new content.
		fmt.Printf("Would create new file for %s:\n\n", key)
		fmt.Print(newContent)
		return nil
	}

	if existing == newContent {
		fmt.Printf("No changes for %s\n", key)
		return nil
	}

	// Simple line-by-line diff display.
	oldLines := strings.Split(existing, "\n")
	newLines := strings.Split(newContent, "\n")

	fmt.Printf("Changes for %s:\n\n", key)

	maxLines := len(oldLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	for i := 0; i < maxLines; i++ {
		oldLine := ""
		newLine := ""
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}
		if oldLine != newLine {
			if oldLine != "" {
				fmt.Printf("- %s\n", oldLine)
			}
			if newLine != "" {
				fmt.Printf("+ %s\n", newLine)
			}
		}
	}
	return nil
}
