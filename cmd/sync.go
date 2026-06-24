package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/erickhilda/jt/internal/config"
	"github.com/erickhilda/jt/internal/jira"
	"github.com/erickhilda/jt/internal/renderer"
	"github.com/erickhilda/jt/internal/store"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync local tickets with Jira",
	Long:  "Finds locally saved tickets that have been updated on Jira since last fetch and re-pulls them.",
	Args:  cobra.NoArgs,
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().StringP("project", "p", "", "Only sync tickets for this project prefix")
	syncCmd.Flags().Bool("dry-run", false, "Show which tickets would be synced without fetching")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, _ []string) error {
	project, _ := cmd.Flags().GetString("project")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	tickets, err := store.ListTickets(cfg.TicketsDir)
	if err != nil {
		return fmt.Errorf("listing tickets: %w", err)
	}

	if len(tickets) == 0 {
		fmt.Println("No local tickets to sync.")
		return nil
	}

	// Filter by project if specified.
	if project != "" {
		project = strings.ToUpper(project)
		filtered := tickets[:0]
		for _, t := range tickets {
			if strings.HasPrefix(t.Key, project+"-") || strings.HasPrefix(t.Key, project) {
				filtered = append(filtered, t)
			}
		}
		tickets = filtered
		if len(tickets) == 0 {
			fmt.Printf("No local tickets matching project %q.\n", project)
			return nil
		}
	}

	// Find the oldest fetch time to use as the JQL "updated" threshold.
	oldest := tickets[0].Fetched
	for _, t := range tickets[1:] {
		if t.Fetched.Before(oldest) {
			oldest = t.Fetched
		}
	}

	token, err := config.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("retrieving token: %w", err)
	}
	client := jira.NewClient(cfg.Instance, cfg.Email, token)

	// Build JQL in batches of 100 keys to avoid URL length limits.
	const batchSize = 100
	staleKeys := map[string]bool{}

	for i := 0; i < len(tickets); i += batchSize {
		end := i + batchSize
		if end > len(tickets) {
			end = len(tickets)
		}
		batch := tickets[i:end]

		keys := make([]string, len(batch))
		for j, t := range batch {
			keys[j] = t.Key
		}

		// Jira JQL date format: "YYYY/MM/DD HH:mm"
		jqlDate := oldest.UTC().Format("2006/01/02 15:04")
		jql := fmt.Sprintf("key in (%s) AND updated > \"%s\"",
			strings.Join(keys, ","), jqlDate)

		result, err := client.SearchIssues(jql, []string{"key"}, 0)
		if err != nil {
			if errors.Is(err, jira.ErrUnauthorized) {
				return fmt.Errorf("authentication failed: %w", err)
			}
			return fmt.Errorf("searching for updated tickets: %w", err)
		}

		for _, issue := range result.Issues {
			staleKeys[issue.Key] = true
		}
	}

	if len(staleKeys) == 0 {
		fmt.Println("All tickets are up to date.")
		return nil
	}

	if dryRun {
		fmt.Printf("Would sync %d ticket(s):\n", len(staleKeys))
		for _, t := range tickets {
			if staleKeys[t.Key] {
				age := humanAge(time.Now(), t.Fetched)
				fmt.Printf("  %s (fetched %s)\n", t.Key, age)
			}
		}
		return nil
	}

	// Re-pull each stale ticket.
	synced := 0
	for _, t := range tickets {
		if !staleKeys[t.Key] {
			continue
		}

		fetchComments := cfg.ShouldFetchComments()
		issue, err := client.GetIssueWithFields(t.Key, issueFieldsFor(fetchComments))
		if err != nil {
			fmt.Printf("  %s: error: %v\n", t.Key, err)
			continue
		}

		// Fetch development-panel pull requests (non-fatal; see runPull).
		prFetched := false
		if cfg.ShouldFetchPullRequests() {
			if prs, prErr := client.GetPullRequests(issue.ID); prErr != nil {
				fmt.Printf("  %s: warning: could not fetch pull requests: %v\n", t.Key, prErr)
			} else {
				issue.PullRequests = prs
				prFetched = true
			}
		}

		content := renderer.RenderIssue(issue)

		// Preserve local notes (and Comments/Pull Requests when we didn't fetch them).
		if existing, loadErr := store.Load(cfg.TicketsDir, t.Key); loadErr == nil {
			content = preserveSections(existing, content, fetchComments, prFetched)
		}

		if err := store.Save(cfg.TicketsDir, t.Key, content); err != nil {
			fmt.Printf("  %s: save error: %v\n", t.Key, err)
			continue
		}

		fmt.Printf("  %s: synced\n", t.Key)
		synced++
	}

	fmt.Printf("Synced %d/%d tickets.\n", synced, len(staleKeys))
	return nil
}
