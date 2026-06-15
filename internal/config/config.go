package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// TokenStorage indicates where the API token is stored.
type TokenStorage string

const (
	TokenStorageKeyring TokenStorage = "keyring"
	TokenStorageFile    TokenStorage = "file"
)

// ErrNotFound is returned when no config file exists.
var ErrNotFound = errors.New("config file not found; run 'jt init' to set up")

// configDirOverride allows tests to redirect config storage.
var configDirOverride string

// Config holds the jt configuration.
type Config struct {
	Instance       string       `yaml:"instance"`
	Email          string       `yaml:"email"`
	DefaultProject string       `yaml:"default_project,omitempty"`
	TicketsDir     string       `yaml:"tickets_dir"`
	TokenStorage   TokenStorage `yaml:"token_storage"`
	// FetchComments controls whether pull/diff/sync request and render comments.
	// Pointer so "field absent" means "default true" (backward compatible).
	FetchComments *bool `yaml:"fetch_comments,omitempty"`
	// BitbucketWorkspace is the default workspace for `jt pr <repo>/<id>` refs.
	BitbucketWorkspace string `yaml:"bitbucket_workspace,omitempty"`
	// PRsDir is where `jt pr` saves pull-request markdown (default ~/.jt/prs).
	PRsDir string `yaml:"prs_dir,omitempty"`
}

// PRsDirOrDefault returns the configured PR storage directory, defaulting to
// ~/.jt/prs when unset.
func (c *Config) PRsDirOrDefault() string {
	if c == nil || c.PRsDir == "" {
		return "~/.jt/prs"
	}
	return c.PRsDir
}

// ShouldFetchComments returns whether comments should be fetched and rendered.
// Default (nil) is true to preserve prior behavior.
func (c *Config) ShouldFetchComments() bool {
	if c == nil || c.FetchComments == nil {
		return true
	}
	return *c.FetchComments
}

// SetConfigDir overrides the config directory (for testing).
func SetConfigDir(dir string) {
	configDirOverride = dir
}

// ResetConfigDir clears the override.
func ResetConfigDir() {
	configDirOverride = ""
}

// ConfigDir returns the directory where jt stores its config.
func ConfigDir() (string, error) {
	if configDirOverride != "" {
		return configDirOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".jt"), nil
}

// ConfigPath returns the full path to config.yaml.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Exists returns true if the config file is present on disk.
func Exists() (bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Load reads and parses the config file.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// Save writes the config to disk, creating the directory if needed.
func Save(cfg *Config) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	path := filepath.Join(dir, "config.yaml")
	return os.WriteFile(path, data, 0600)
}

// Validate checks that required fields are present and well-formed.
func (c *Config) Validate() error {
	var errs []string
	if c.Instance == "" {
		errs = append(errs, "instance is required")
	} else if !strings.HasPrefix(c.Instance, "https://") {
		errs = append(errs, "instance must start with https://")
	}
	if c.Email == "" {
		errs = append(errs, "email is required")
	}
	if c.TokenStorage != TokenStorageKeyring && c.TokenStorage != TokenStorageFile {
		errs = append(errs, "token_storage must be 'keyring' or 'file'")
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid config: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ExpandPath expands a leading ~ to the user's home directory.
func ExpandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}
