package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/erickhilda/atlit/internal/bitbucket"
	"github.com/erickhilda/atlit/internal/config"
	"github.com/erickhilda/atlit/internal/renderer"
	"github.com/erickhilda/atlit/internal/store"
	"github.com/spf13/cobra"
)

// largeDiffBytes is the threshold above which we warn (but never truncate).
const largeDiffBytes = 500 * 1024

// jiraKeyRe matches a Jira issue key like PROJ-1234 in a branch or title.
var jiraKeyRe = regexp.MustCompile(`[A-Z][A-Z0-9]+-\d+`)

var prCmd = &cobra.Command{
	Use:   "pr <ID | repo/ID | workspace/repo/ID>",
	Short: "Fetch a Bitbucket pull request and save as markdown",
	Long: `Fetches a Bitbucket Cloud pull request (metadata, diff, comments) and saves it
as local markdown for code-review context.

Reference forms:
  atlit pr 4521                       infer workspace/repo from the git remote (run inside the repo)
  atlit pr widget/4521                repo explicit, workspace from config (bitbucket_workspace)
  atlit pr acme/widget/4521           fully explicit`,
	Args: cobra.ExactArgs(1),
	RunE: runPR,
}

var prListCmd = &cobra.Command{
	Use:   "list [repo | workspace/repo]",
	Short: "List a repository's pull requests",
	Long: `Lists a Bitbucket Cloud repository's pull requests as a table on stdout (open
by default, newest-updated first). Nothing is written to disk; run 'atlit pr <id>'
to fetch a chosen PR's diff and comments.

Repo reference forms:
  atlit pr list                 infer workspace/repo from the git remote (run inside the repo)
  atlit pr list widget          repo explicit, workspace from config (bitbucket_workspace)
  atlit pr list acme/widget     fully explicit`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPRList,
}

func init() {
	prCmd.Flags().Bool("no-diff", false, "Omit the unified diff (keep diffstat + comments)")
	prCmd.Flags().Bool("dry-run", false, "Show what would change without saving")

	prListCmd.Flags().String("state", "open", "Filter by state: open|merged|declined|all")
	prListCmd.Flags().Int("limit", 30, "Maximum number of PRs to list (rows shown, not the repo total)")
	prCmd.AddCommand(prListCmd)

	rootCmd.AddCommand(prCmd)
}

func runPRList(cmd *cobra.Command, args []string) error {
	stateFlag, _ := cmd.Flags().GetString("state")
	limit, _ := cmd.Flags().GetInt("limit")

	states, label, err := mapPRStates(stateFlag)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	arg := ""
	if len(args) == 1 {
		arg = args[0]
	}
	workspace, repo, err := resolveRepoRef(arg, cfg)
	if err != nil {
		return err
	}

	token, err := config.GetBitbucketToken(cfg)
	if err != nil {
		return fmt.Errorf("retrieving Bitbucket token (run 'atlit auth bitbucket'): %w", err)
	}

	client := bitbucket.NewClient(cfg.Email, token)
	prs, err := client.ListPullRequests(workspace, repo, states, limit)
	if err != nil {
		return wrapBBListError(err, workspace, repo)
	}

	printPRList(workspace, repo, label, prs)
	return nil
}

// mapPRStates maps the --state flag to the API state filter and a display label.
// "all" returns a nil slice (no state filter).
func mapPRStates(flag string) (states []string, label string, err error) {
	switch strings.ToLower(strings.TrimSpace(flag)) {
	case "", "open":
		return []string{"OPEN"}, "Open", nil
	case "merged":
		return []string{"MERGED"}, "Merged", nil
	case "declined":
		return []string{"DECLINED"}, "Declined", nil
	case "all":
		return nil, "All", nil
	default:
		return nil, "", fmt.Errorf("invalid --state %q (want open|merged|declined|all)", flag)
	}
}

// resolveRepoRef parses an optional "repo" or "workspace/repo" argument into a
// workspace and repo, inferring from the git remote / config when omitted. It
// mirrors resolvePRRef without the trailing PR id.
func resolveRepoRef(arg string, cfg *config.Config) (workspace, repo string, err error) {
	if arg == "" {
		ws, r, gerr := inferFromGitRemote()
		if gerr != nil {
			return "", "", fmt.Errorf("not in a Bitbucket repo (%v); use 'atlit pr list <repo>' or 'atlit pr list <workspace>/<repo>'", gerr)
		}
		return ws, r, nil
	}
	parts := strings.Split(arg, "/")
	switch len(parts) {
	case 2: // workspace/repo
		if parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("invalid repo reference %q", arg)
		}
		return parts[0], parts[1], nil
	case 1: // repo -> workspace from config, falling back to the git remote
		repo = parts[0]
		workspace = cfg.BitbucketWorkspace
		if workspace == "" {
			if ws, _, gerr := inferFromGitRemote(); gerr == nil {
				workspace = ws
			}
		}
		if workspace == "" {
			return "", "", fmt.Errorf("no workspace: set 'bitbucket_workspace' in config or use 'workspace/%s'", arg)
		}
		return workspace, repo, nil
	default:
		return "", "", fmt.Errorf("invalid repo reference %q", arg)
	}
}

// printPRList renders the PR table (or an empty-result line) to stdout.
func printPRList(workspace, repo, label string, prs []bitbucket.PullRequest) {
	if len(prs) == 0 {
		if label == "All" {
			fmt.Printf("No PRs for %s/%s\n", workspace, repo)
		} else {
			fmt.Printf("No %s PRs for %s/%s\n", strings.ToLower(label), workspace, repo)
		}
		return
	}

	fmt.Printf("%s PRs: %s/%s (%d)\n\n", label, workspace, repo, len(prs))
	fmt.Printf("%-6s %-40s %-16s %-14s %-24s %s\n", "#", "TITLE", "JIRA", "AUTHOR", "APPROVED BY", "UPDATED")

	now := time.Now()
	for i := range prs {
		pr := &prs[i]
		jira := detectJiraKey(pr)
		if jira == "" {
			jira = "-"
		}
		fmt.Printf("%-6d %-40s %-16s %-14s %-24s %s\n",
			pr.ID,
			truncate(pr.Title, 40),
			jira,
			truncate(pr.Author.DisplayName, 14),
			truncate(prApprovers(pr), 24),
			formatPRUpdated(now, pr.UpdatedOn),
		)
	}
}

// prApprovers lists the display names of everyone who approved the PR, or "-"
// when no one has approved yet.
func prApprovers(pr *bitbucket.PullRequest) string {
	var names []string
	for _, p := range pr.Participants {
		if p.Approved && p.User.DisplayName != "" {
			names = append(names, p.User.DisplayName)
		}
	}
	if len(names) == 0 {
		return "-"
	}
	return strings.Join(names, ", ")
}

// truncate shortens s to at most max runes, marking elision with an ellipsis.
// Rune-based so multibyte titles stay aligned under fmt's rune-counted width.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

// formatPRUpdated renders a Bitbucket updated_on timestamp as a relative age.
// Bitbucket is an Atlassian product and emits the same timestamp family as Jira,
// so parseJiraTime handles it; the raw value is shown if parsing ever fails.
func formatPRUpdated(now time.Time, updatedOn string) string {
	t, err := parseJiraTime(updatedOn)
	if err != nil {
		return updatedOn
	}
	return humanAge(now, t)
}

func wrapBBListError(err error, workspace, repo string) error {
	switch {
	case errors.Is(err, bitbucket.ErrUnauthorized):
		return fmt.Errorf("authentication failed: %w (re-run 'atlit auth bitbucket')", err)
	case errors.Is(err, bitbucket.ErrForbidden):
		return err
	case errors.Is(err, bitbucket.ErrNotFound):
		return fmt.Errorf("repo %s/%s not found or no access", workspace, repo)
	default:
		return err
	}
}

func runPR(cmd *cobra.Command, args []string) error {
	noDiff, _ := cmd.Flags().GetBool("no-diff")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	workspace, repo, id, err := resolvePRRef(args[0], cfg)
	if err != nil {
		return err
	}

	token, err := config.GetBitbucketToken(cfg)
	if err != nil {
		return fmt.Errorf("retrieving Bitbucket token (run 'atlit auth bitbucket'): %w", err)
	}

	client := bitbucket.NewClient(cfg.Email, token)

	pr, err := client.GetPullRequest(workspace, repo, id)
	if err != nil {
		return wrapBBError(err, workspace, repo, id)
	}

	diffstat, err := client.GetPullRequestDiffstat(workspace, repo, id)
	if err != nil {
		return wrapBBError(err, workspace, repo, id)
	}

	comments, err := client.GetPullRequestComments(workspace, repo, id)
	if err != nil {
		return wrapBBError(err, workspace, repo, id)
	}

	diff := ""
	if !noDiff {
		diff, err = client.GetPullRequestDiff(workspace, repo, id)
		if err != nil {
			return wrapBBError(err, workspace, repo, id)
		}
		if len(diff) > largeDiffBytes {
			fmt.Fprintf(os.Stderr, "warning: diff is %d KB; consider --no-diff or reviewing specific files\n", len(diff)/1024)
		}
	}

	jiraKey := detectJiraKey(pr)
	ticketPath := localTicketPath(cfg, jiraKey)

	content := renderer.RenderPullRequest(pr, workspace, repo, diffstat, diff, comments, jiraKey, ticketPath)

	prsDir := cfg.PRsDirOrDefault()
	key := prFileKey(workspace, repo, id)

	// Preserve a hand-added "## My Notes" section across re-pulls.
	if existing, err := store.Load(prsDir, key); err == nil {
		content = preserveNotes(existing, content)
	}

	if dryRun {
		return showDryRunDir(prsDir, key, content)
	}

	if err := store.Save(prsDir, key, content); err != nil {
		return fmt.Errorf("saving PR: %w", err)
	}

	path, _ := store.TicketPath(prsDir, key)
	fmt.Printf("Saved %s/%s PR #%d to %s\n", workspace, repo, id, path)
	return nil
}

// resolvePRRef parses a PR reference into workspace, repo, and numeric id.
func resolvePRRef(arg string, cfg *config.Config) (workspace, repo string, id int, err error) {
	parts := strings.Split(arg, "/")
	switch len(parts) {
	case 3: // workspace/repo/id
		workspace, repo = parts[0], parts[1]
		id, err = parsePRID(parts[2])
	case 2: // repo/id
		repo = parts[0]
		id, err = parsePRID(parts[1])
		workspace = cfg.BitbucketWorkspace
		if workspace == "" {
			if ws, _, gerr := inferFromGitRemote(); gerr == nil {
				workspace = ws
			}
		}
		if workspace == "" {
			return "", "", 0, fmt.Errorf("no workspace: set 'bitbucket_workspace' in config or use 'workspace/%s'", arg)
		}
	case 1: // id only -> infer workspace + repo from git remote
		id, err = parsePRID(parts[0])
		if err != nil {
			return "", "", 0, err
		}
		ws, r, gerr := inferFromGitRemote()
		if gerr != nil {
			return "", "", 0, fmt.Errorf("not in a Bitbucket repo (%v); use 'atlit pr <repo>/%d' or 'atlit pr <workspace>/<repo>/%d'", gerr, id, id)
		}
		workspace, repo = ws, r
	default:
		return "", "", 0, fmt.Errorf("invalid PR reference %q", arg)
	}
	if err != nil {
		return "", "", 0, err
	}
	return workspace, repo, id, nil
}

func parsePRID(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid PR id %q", s)
	}
	if id <= 0 {
		return 0, fmt.Errorf("PR id must be positive, got %d", id)
	}
	return id, nil
}

// inferFromGitRemote derives workspace and repo from the origin remote URL.
func inferFromGitRemote() (workspace, repo string, err error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", "", fmt.Errorf("no git 'origin' remote")
	}
	return parseBitbucketRemote(strings.TrimSpace(string(out)))
}

// parseBitbucketRemote extracts workspace and repo from an SSH or HTTPS
// bitbucket.org remote URL. Repo slugs may contain dots, so only a trailing
// ".git" suffix is stripped.
func parseBitbucketRemote(remote string) (workspace, repo string, err error) {
	var path string
	switch {
	case strings.HasPrefix(remote, "git@bitbucket.org:"):
		path = strings.TrimPrefix(remote, "git@bitbucket.org:")
	case strings.Contains(remote, "bitbucket.org/"):
		path = remote[strings.Index(remote, "bitbucket.org/")+len("bitbucket.org/"):]
	default:
		return "", "", fmt.Errorf("origin is not a bitbucket.org remote: %s", remote)
	}
	path = strings.TrimSuffix(path, ".git")
	segs := strings.SplitN(path, "/", 2)
	if len(segs) != 2 || segs[0] == "" || segs[1] == "" {
		return "", "", fmt.Errorf("cannot parse workspace/repo from remote: %s", remote)
	}
	return segs[0], segs[1], nil
}

// detectJiraKey finds a Jira key in the PR source branch, falling back to title.
func detectJiraKey(pr *bitbucket.PullRequest) string {
	if k := jiraKeyRe.FindString(pr.Source.Branch.Name); k != "" {
		return k
	}
	return jiraKeyRe.FindString(pr.Title)
}

// localTicketPath returns the local ticket file path for jiraKey if it exists.
func localTicketPath(cfg *config.Config, jiraKey string) string {
	if jiraKey == "" {
		return ""
	}
	p, err := store.TicketPath(cfg.TicketsDir, jiraKey)
	if err != nil {
		return ""
	}
	if _, statErr := os.Stat(p); statErr != nil {
		return ""
	}
	return p
}

func prFileKey(workspace, repo string, id int) string {
	return fmt.Sprintf("%s__%s__%d", workspace, repo, id)
}

func wrapBBError(err error, workspace, repo string, id int) error {
	switch {
	case errors.Is(err, bitbucket.ErrUnauthorized):
		return fmt.Errorf("authentication failed: %w (re-run 'atlit auth bitbucket')", err)
	case errors.Is(err, bitbucket.ErrForbidden):
		return err
	case errors.Is(err, bitbucket.ErrNotFound):
		return fmt.Errorf("PR %s/%s#%d not found or no access", workspace, repo, id)
	default:
		return err
	}
}
