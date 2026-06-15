package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/erickhilda/jt/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage jt configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current configuration",
	RunE:  runConfigShow,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Update a configuration setting",
	Long: `Valid keys: instance, email, default_project, tickets_dir, fetch_comments,
token, bitbucket_workspace, prs_dir, bitbucket_token

Examples:
  jt config set instance https://myorg.atlassian.net
  jt config set default_project PROJ
  jt config set fetch_comments false
  jt config set token <new-api-token>
  jt config set bitbucket_workspace acme
  jt config set bitbucket_token <new-bitbucket-api-token>`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	token := "(not stored)"
	t, err := config.GetToken(cfg)
	if err == nil && t != "" {
		token = maskToken(t)
	}

	bbToken := "(not stored)"
	if t, err := config.GetBitbucketToken(cfg); err == nil && t != "" {
		bbToken = maskToken(t)
	}

	fmt.Printf("instance:            %s\n", cfg.Instance)
	fmt.Printf("email:               %s\n", cfg.Email)
	fmt.Printf("default_project:     %s\n", cfg.DefaultProject)
	fmt.Printf("tickets_dir:         %s\n", cfg.TicketsDir)
	fmt.Printf("token_storage:       %s\n", cfg.TokenStorage)
	fmt.Printf("fetch_comments:      %t\n", cfg.ShouldFetchComments())
	fmt.Printf("token:               %s\n", token)
	fmt.Printf("bitbucket_workspace: %s\n", cfg.BitbucketWorkspace)
	fmt.Printf("prs_dir:             %s\n", cfg.PRsDirOrDefault())
	fmt.Printf("bitbucket_token:     %s\n", bbToken)
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	if key == "token" {
		return setToken(value)
	}
	if key == "bitbucket_token" {
		return setBitbucketToken(value)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	switch key {
	case "instance":
		if !strings.HasPrefix(value, "https://") {
			return fmt.Errorf("instance must start with https://")
		}
		cfg.Instance = strings.TrimRight(value, "/")
	case "email":
		cfg.Email = value
	case "default_project":
		cfg.DefaultProject = strings.ToUpper(value)
	case "tickets_dir":
		cfg.TicketsDir = value
	case "fetch_comments":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("fetch_comments must be true or false, got %q", value)
		}
		cfg.FetchComments = &b
	case "bitbucket_workspace":
		cfg.BitbucketWorkspace = value
	case "prs_dir":
		cfg.PRsDir = value
	default:
		return fmt.Errorf("unknown key %q; valid keys: instance, email, default_project, tickets_dir, fetch_comments, token, bitbucket_workspace, prs_dir, bitbucket_token", key)
	}

	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("%s set to %q\n", key, value)
	return nil
}

func setToken(value string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	storage, err := config.SetToken(cfg.Email, value)
	if err != nil {
		return fmt.Errorf("storing token: %w", err)
	}
	cfg.TokenStorage = storage
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("token updated (stored via %s)\n", storage)
	return nil
}

func setBitbucketToken(value string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	storage, err := config.SetBitbucketToken(cfg.Email, value)
	if err != nil {
		return fmt.Errorf("storing Bitbucket token: %w", err)
	}
	fmt.Printf("bitbucket_token updated (stored via %s)\n", storage)
	return nil
}

func maskToken(token string) string {
	if len(token) <= 4 {
		return "****"
	}
	return token[:4] + strings.Repeat("*", len(token)-4)
}
