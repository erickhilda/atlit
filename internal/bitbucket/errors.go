package bitbucket

import (
	"errors"
	"fmt"
)

// ErrUnauthorized indicates invalid or missing credentials (HTTP 401).
var ErrUnauthorized = errors.New("unauthorized: check your email and Bitbucket API token")

// ErrForbidden indicates the token is valid but lacks a required scope (HTTP 403).
var ErrForbidden = errors.New("forbidden: token missing scope (need read:pullrequest:bitbucket and read:repository:bitbucket)")

// ErrNotFound indicates the requested resource does not exist (HTTP 404).
var ErrNotFound = errors.New("not found")

// APIError represents a non-success HTTP response from Bitbucket.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("bitbucket API error (HTTP %d): %s", e.StatusCode, e.Message)
}
