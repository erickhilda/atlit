package cmd

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/erickhilda/jt/internal/bitbucket"
	"github.com/erickhilda/jt/internal/config"
	"github.com/erickhilda/jt/internal/jira"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
}

var authTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Verify Jira credentials",
	Long:  "Calls the Jira /rest/api/3/myself endpoint to verify your credentials are valid.",
	RunE:  runAuthTest,
}

var authBitbucketCmd = &cobra.Command{
	Use:   "bitbucket",
	Short: "Set and verify your Bitbucket API token",
	Long: `Prompts for a Bitbucket Cloud API token and stores it.

Create the token at https://id.atlassian.com/manage-profile/security/api-tokens
with scopes: read:pullrequest:bitbucket and read:repository:bitbucket.`,
	RunE: runAuthBitbucket,
}

func init() {
	authCmd.AddCommand(authTestCmd)
	authCmd.AddCommand(authBitbucketCmd)
	rootCmd.AddCommand(authCmd)
}

func runAuthTest(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	token, err := config.GetToken(cfg)
	if err != nil {
		return fmt.Errorf("retrieving token: %w", err)
	}

	client := jira.NewClient(cfg.Instance, cfg.Email, token)
	user, err := client.Myself()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Printf("Authenticated as %s (%s)\n", user.DisplayName, user.Email)
	fmt.Printf("Account ID: %s\n", user.AccountID)
	fmt.Printf("Time zone:  %s\n", user.TimeZone)
	fmt.Printf("Active:     %v\n", user.Active)
	return nil
}

func runAuthBitbucket(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	fmt.Print("Bitbucket API token: ")
	tokenBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("reading token: %w", err)
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return fmt.Errorf("token is required")
	}

	storage, err := config.SetBitbucketToken(cfg.Email, token)
	if err != nil {
		return fmt.Errorf("storing token: %w", err)
	}
	fmt.Printf("Bitbucket token stored (via %s).\n", storage)

	// Verify against the configured workspace when one is set.
	if cfg.BitbucketWorkspace == "" {
		fmt.Println("Tip: set 'bitbucket_workspace' to enable 'repo/id' refs and verification.")
		return nil
	}
	client := bitbucket.NewClient(cfg.Email, token)
	if err := client.VerifyWorkspace(cfg.BitbucketWorkspace); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not verify access to workspace %q: %v\n", cfg.BitbucketWorkspace, err)
		return nil
	}
	fmt.Printf("Verified access to workspace %q.\n", cfg.BitbucketWorkspace)
	return nil
}
