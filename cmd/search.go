package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/erickhilda/jt/internal/config"
	"github.com/erickhilda/jt/internal/jira"
	"github.com/spf13/cobra"
)

// searchFields are the issue fields requested for the list view. Keeping this
// narrow makes the JQL search payload small and the decode cheap.
var searchFields = []string{"summary", "status", "assignee", "issuetype", "updated"}

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search Jira tickets with preset filters or raw JQL",
	Long: `Searches Jira and lists matching tickets as a table on stdout. Nothing is
written to disk; run 'jt pull <KEY>' to fetch a chosen ticket.

Preset filters (composed with AND, scoped to default_project unless overridden):
  jt search --status "code review"
  jt search --status "code review,stage test"   # comma -> status in (...)
  jt search --assignee alice                      # name/email resolved to an account
  jt search --mine                                # assignee = currentUser()
  jt search --mine --active                       # exclude done-category statuses
  jt search --status "stage test" --project FOO   # override the project scope
  jt search --mine --all-projects                 # drop the project scope

Advanced (raw JQL escape hatch; cannot be combined with preset filters):
  jt search --jql "project = FOO AND sprint in openSprints() ORDER BY updated DESC"`,
	Args: cobra.NoArgs,
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().String("status", "", "Filter by status name; comma-separate for multiple (e.g. \"code review,stage test\")")
	searchCmd.Flags().String("assignee", "", "Filter by assignee; a name or email resolved to a Jira account")
	searchCmd.Flags().Bool("mine", false, "Filter to tickets assigned to you (assignee = currentUser())")
	searchCmd.Flags().Bool("active", false, "Exclude done-category statuses (statusCategory != Done)")
	searchCmd.Flags().String("jql", "", "Raw JQL query (advanced; cannot be combined with preset filters)")
	searchCmd.Flags().String("project", "", "Restrict to this project key (overrides default_project)")
	searchCmd.Flags().Bool("all-projects", false, "Do not restrict to a project")
	searchCmd.Flags().Int("limit", 30, "Maximum number of tickets to list (rows shown, not the query total)")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, _ []string) error {
	status, _ := cmd.Flags().GetString("status")
	assignee, _ := cmd.Flags().GetString("assignee")
	mine, _ := cmd.Flags().GetBool("mine")
	active, _ := cmd.Flags().GetBool("active")
	rawJQL, _ := cmd.Flags().GetString("jql")
	project, _ := cmd.Flags().GetString("project")
	allProjects, _ := cmd.Flags().GetBool("all-projects")
	limit, _ := cmd.Flags().GetInt("limit")

	status = strings.TrimSpace(status)
	assignee = strings.TrimSpace(assignee)
	rawJQL = strings.TrimSpace(rawJQL)
	project = strings.TrimSpace(project)

	if err := validateSearchFlags(status, assignee, mine, active, rawJQL, project, allProjects); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	token, err := config.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("retrieving token: %w", err)
	}
	client := jira.NewClient(cfg.Instance, cfg.Email, token)

	jql := rawJQL
	if jql == "" {
		assigneeClause := ""
		switch {
		case mine:
			assigneeClause = "assignee = currentUser()"
		case assignee != "":
			accountID, rerr := resolveAssignee(client, assignee)
			if rerr != nil {
				return rerr
			}
			assigneeClause = "assignee = " + quoteJQL(accountID)
		}

		projectKey := project
		if projectKey == "" && !allProjects {
			projectKey = cfg.DefaultProject
		}
		jql = buildJQL(projectKey, status, assigneeClause, active)
	}

	result, err := client.SearchIssues(jql, searchFields, limit)
	if err != nil {
		if errors.Is(err, jira.ErrUnauthorized) {
			return fmt.Errorf("authentication failed: %w", err)
		}
		return fmt.Errorf("searching: %w", err)
	}

	printSearchResults(jql, result.Issues, limit)
	return nil
}

// validateSearchFlags enforces the flag contract: --jql is a standalone escape
// hatch, --mine/--assignee and --project/--all-projects are each mutually
// exclusive, and at least one real filter must be present.
func validateSearchFlags(status, assignee string, mine, active bool, rawJQL, project string, allProjects bool) error {
	if rawJQL != "" {
		if status != "" || assignee != "" || mine || active || project != "" || allProjects {
			return errors.New("--jql cannot be combined with preset filters (--status/--assignee/--mine/--active/--project/--all-projects)")
		}
		return nil
	}
	if mine && assignee != "" {
		return errors.New("--mine and --assignee are mutually exclusive")
	}
	if project != "" && allProjects {
		return errors.New("--project and --all-projects are mutually exclusive")
	}
	if status == "" && assignee == "" && !mine && !active {
		return errors.New("provide at least one filter: --status, --assignee, --mine, --active, or --jql (advanced)")
	}
	return nil
}

// buildJQL assembles the preset query: project scope (when set) AND active
// (statusCategory != Done) AND status AND assignee, ordered newest-updated first.
func buildJQL(projectKey, status, assigneeClause string, active bool) string {
	var clauses []string
	if projectKey != "" {
		clauses = append(clauses, "project = "+quoteJQL(projectKey))
	}
	if active {
		clauses = append(clauses, "statusCategory != Done")
	}
	if status != "" {
		clauses = append(clauses, statusClause(status))
	}
	if assigneeClause != "" {
		clauses = append(clauses, assigneeClause)
	}
	return strings.Join(clauses, " AND ") + " ORDER BY updated DESC"
}

// statusClause renders one status as "status = X" and several (comma-separated)
// as "status in (X, Y)".
func statusClause(status string) string {
	parts := splitCSV(status)
	if len(parts) <= 1 {
		// Use the cleaned part, not the raw input, so a stray trailing comma or
		// surrounding whitespace (e.g. "Done,") does not leak into the value.
		val := status
		if len(parts) == 1 {
			val = parts[0]
		}
		return "status = " + quoteJQL(val)
	}
	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = quoteJQL(p)
	}
	return "status in (" + strings.Join(quoted, ", ") + ")"
}

// splitCSV splits on commas, trimming whitespace and dropping empty entries.
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// quoteJQL wraps a value in double quotes, escaping backslashes and quotes so
// status names and account ids with spaces or punctuation are valid JQL.
func quoteJQL(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

// resolveAssignee turns a human-friendly name/email into a Jira accountId via
// the user-search API, surfacing a clear error when the name matches zero or
// several people.
func resolveAssignee(client *jira.Client, name string) (string, error) {
	users, err := client.SearchUsers(name)
	if err != nil {
		if errors.Is(err, jira.ErrUnauthorized) {
			return "", fmt.Errorf("authentication failed: %w", err)
		}
		return "", fmt.Errorf("looking up assignee %q: %w", name, err)
	}
	return pickUser(name, users)
}

// pickUser selects a single accountId from user-search results: one match wins
// outright, a unique exact name/email match breaks ties, otherwise it errors
// listing the candidates so the user can refine.
func pickUser(name string, users []jira.User) (string, error) {
	var candidates []jira.User
	for _, u := range users {
		if u.AccountID != "" {
			candidates = append(candidates, u)
		}
	}

	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("no Jira user matches %q", name)
	case 1:
		return candidates[0].AccountID, nil
	}

	lower := strings.ToLower(name)
	var exact []jira.User
	for _, u := range candidates {
		if strings.ToLower(u.DisplayName) == lower || strings.ToLower(u.Email) == lower {
			exact = append(exact, u)
		}
	}
	if len(exact) == 1 {
		return exact[0].AccountID, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d users match %q — refine the name:", len(candidates), name)
	for _, u := range candidates {
		label := u.DisplayName
		if u.Email != "" {
			label += " <" + u.Email + ">"
		}
		fmt.Fprintf(&b, "\n  - %s", label)
	}
	return "", errors.New(b.String())
}

// printSearchResults renders the result table (or an empty-result line) to
// stdout, echoing the effective JQL for transparency.
func printSearchResults(jql string, issues []jira.Issue, limit int) {
	fmt.Printf("JQL: %s\n", jql)
	if len(issues) == 0 {
		fmt.Println("\nNo tickets match.")
		return
	}

	fmt.Printf("\n%-16s %-45s %-16s %-16s %s\n", "KEY", "SUMMARY", "STATUS", "ASSIGNEE", "UPDATED")
	now := time.Now()
	for i := range issues {
		is := &issues[i]
		status := "-"
		if is.Fields.Status != nil {
			status = is.Fields.Status.Name
		}
		assignee := "Unassigned"
		if is.Fields.Assignee != nil && is.Fields.Assignee.DisplayName != "" {
			assignee = is.Fields.Assignee.DisplayName
		}
		fmt.Printf("%-16s %-45s %-16s %-16s %s\n",
			is.Key,
			truncate(is.Fields.Summary, 45),
			truncate(status, 16),
			truncate(assignee, 16),
			formatSearchUpdated(now, is.Fields.Updated),
		)
	}

	if limit > 0 && len(issues) >= limit {
		fmt.Printf("\nShowing first %d (raise --limit for more). Run 'jt pull <KEY>' to fetch one.\n", limit)
	} else {
		fmt.Printf("\n%d ticket(s). Run 'jt pull <KEY>' to fetch one.\n", len(issues))
	}
}

// formatSearchUpdated renders a Jira timestamp as a relative age, falling back
// to the raw value if it cannot be parsed.
func formatSearchUpdated(now time.Time, raw string) string {
	if raw == "" {
		return "-"
	}
	t, err := parseJiraTime(raw)
	if err != nil {
		return raw
	}
	return humanAge(now, t)
}
