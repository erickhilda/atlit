package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/erickhilda/jt/internal/bitbucket"
	"github.com/erickhilda/jt/internal/config"
	"github.com/erickhilda/jt/internal/renderer"
	"github.com/erickhilda/jt/internal/store"
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
  jt pr 4521                       infer workspace/repo from the git remote (run inside the repo)
  jt pr widget/4521                repo explicit, workspace from config (bitbucket_workspace)
  jt pr acme/widget/4521           fully explicit`,
	Args: cobra.ExactArgs(1),
	RunE: runPR,
}

func init() {
	prCmd.Flags().Bool("no-diff", false, "Omit the unified diff (keep diffstat + comments)")
	prCmd.Flags().Bool("dry-run", false, "Show what would change without saving")
	rootCmd.AddCommand(prCmd)
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
		return fmt.Errorf("retrieving Bitbucket token (run 'jt auth bitbucket'): %w", err)
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
			return "", "", 0, fmt.Errorf("not in a Bitbucket repo (%v); use 'jt pr <repo>/%d' or 'jt pr <workspace>/<repo>/%d'", gerr, id, id)
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
		return fmt.Errorf("authentication failed: %w (re-run 'jt auth bitbucket')", err)
	case errors.Is(err, bitbucket.ErrForbidden):
		return err
	case errors.Is(err, bitbucket.ErrNotFound):
		return fmt.Errorf("PR %s/%s#%d not found or no access", workspace, repo, id)
	default:
		return err
	}
}
