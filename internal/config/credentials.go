package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "jt-cli"
	credFileName   = "credentials"
	// bitbucketKeyringSuffix namespaces the Bitbucket token under a second
	// keyring account so it sits beside the Jira token without colliding.
	bitbucketKeyringSuffix = "#bitbucket"
	// bitbucketCredFileName holds the Bitbucket token in the file-fallback case,
	// kept separate from the Jira credentials file to avoid format changes.
	bitbucketCredFileName = "credentials-bitbucket"
)

// SetToken stores the API token. It tries the system keyring first;
// if unavailable, it falls back to a file. Returns which storage was used.
func SetToken(email, token string) (TokenStorage, error) {
	if isKeyringAvailable() {
		if err := keyring.Set(keyringService, email, token); err == nil {
			return TokenStorageKeyring, nil
		}
	}
	return TokenStorageFile, setTokenFile(token)
}

// GetToken retrieves the API token using the method recorded in config.
func GetToken(cfg *Config) (string, error) {
	switch cfg.TokenStorage {
	case TokenStorageKeyring:
		token, err := keyring.Get(keyringService, cfg.Email)
		if err != nil {
			return "", fmt.Errorf("reading token from keyring: %w", err)
		}
		return token, nil
	case TokenStorageFile:
		return getTokenFile()
	default:
		return "", fmt.Errorf("unknown token_storage: %q", cfg.TokenStorage)
	}
}

// DeleteToken removes the stored token.
func DeleteToken(cfg *Config) error {
	switch cfg.TokenStorage {
	case TokenStorageKeyring:
		return keyring.Delete(keyringService, cfg.Email)
	case TokenStorageFile:
		return deleteTokenFile()
	default:
		return fmt.Errorf("unknown token_storage: %q", cfg.TokenStorage)
	}
}

// SetBitbucketToken stores the Bitbucket API token under a second keyring
// account (email + suffix), falling back to a separate file.
func SetBitbucketToken(email, token string) (TokenStorage, error) {
	if isKeyringAvailable() {
		if err := keyring.Set(keyringService, email+bitbucketKeyringSuffix, token); err == nil {
			return TokenStorageKeyring, nil
		}
	}
	return TokenStorageFile, setBitbucketTokenFile(token)
}

// GetBitbucketToken retrieves the Bitbucket API token using the configured
// storage method.
func GetBitbucketToken(cfg *Config) (string, error) {
	switch cfg.TokenStorage {
	case TokenStorageKeyring:
		token, err := keyring.Get(keyringService, cfg.Email+bitbucketKeyringSuffix)
		if err != nil {
			return "", fmt.Errorf("reading Bitbucket token from keyring: %w", err)
		}
		return token, nil
	case TokenStorageFile:
		return getBitbucketTokenFile()
	default:
		return "", fmt.Errorf("unknown token_storage: %q", cfg.TokenStorage)
	}
}

func setBitbucketTokenFile(token string) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, bitbucketCredFileName), []byte(token), 0600)
}

func getBitbucketTokenFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, bitbucketCredFileName))
	if err != nil {
		return "", fmt.Errorf("reading Bitbucket credentials file: %w", err)
	}
	return string(data), nil
}

// isKeyringAvailable tests whether the system keyring works by doing
// a set/delete round-trip with a throwaway value.
func isKeyringAvailable() bool {
	testKey := "jt-keyring-test"
	testVal := "test"
	if err := keyring.Set(keyringService, testKey, testVal); err != nil {
		return false
	}
	_ = keyring.Delete(keyringService, testKey)
	return true
}

func credentialsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, credFileName), nil
}

func setTokenFile(token string) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	path := filepath.Join(dir, credFileName)
	return os.WriteFile(path, []byte(token), 0600)
}

func getTokenFile() (string, error) {
	path, err := credentialsPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading credentials file: %w", err)
	}
	return string(data), nil
}

func deleteTokenFile() error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing credentials file: %w", err)
	}
	return nil
}
