package bitbucket

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.bitbucket.org/2.0"

// Client is an authenticated Bitbucket Cloud REST API client.
type Client struct {
	baseURL    string
	authHeader string
	http       *http.Client
}

// NewClient creates a Bitbucket client using Basic auth (email:apiToken), the
// Atlassian API-token scheme for api.bitbucket.org. The username is the
// Atlassian account email (not the Bitbucket username) for API access.
func NewClient(email, token string) *Client {
	creds := base64.StdEncoding.EncodeToString([]byte(email + ":" + token))
	return &Client{
		baseURL:    defaultBaseURL,
		authHeader: "Basic " + creds,
		http:       &http.Client{Timeout: 30 * time.Second},
	}
}

// GetPullRequest fetches a pull request's core fields.
func (c *Client) GetPullRequest(workspace, repo string, id int) (*PullRequest, error) {
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", workspace, repo, id)
	body, err := c.getJSON(path)
	if err != nil {
		return nil, err
	}
	var pr PullRequest
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("decoding pull request: %w", err)
	}
	return &pr, nil
}

// GetPullRequestDiff fetches the raw unified diff (text/plain).
func (c *Client) GetPullRequestDiff(workspace, repo string, id int) (string, error) {
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/diff", workspace, repo, id)
	resp, err := c.do(http.MethodGet, path, "text/plain")
	if err != nil {
		return "", err
	}
	body, status, err := readAndClose(resp)
	if err != nil {
		return "", err
	}
	if err := classify(status, body); err != nil {
		return "", err
	}
	return string(body), nil
}

// GetPullRequestDiffstat returns per-file change stats, following pagination.
func (c *Client) GetPullRequestDiffstat(workspace, repo string, id int) ([]DiffstatEntry, error) {
	next := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/diffstat?pagelen=100", workspace, repo, id)
	var all []DiffstatEntry
	for next != "" {
		body, err := c.getJSON(next)
		if err != nil {
			return nil, err
		}
		var page struct {
			Values []DiffstatEntry `json:"values"`
			Next   string          `json:"next"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decoding diffstat: %w", err)
		}
		all = append(all, page.Values...)
		next = page.Next
	}
	return all, nil
}

// GetPullRequestComments returns all PR comments, following pagination.
func (c *Client) GetPullRequestComments(workspace, repo string, id int) ([]Comment, error) {
	next := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/comments?pagelen=100", workspace, repo, id)
	var all []Comment
	for next != "" {
		body, err := c.getJSON(next)
		if err != nil {
			return nil, err
		}
		var page struct {
			Values []Comment `json:"values"`
			Next   string    `json:"next"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decoding comments: %w", err)
		}
		all = append(all, page.Values...)
		next = page.Next
	}
	return all, nil
}

// ListPullRequests returns pull requests for a repo, newest-updated first,
// following pagination up to limit results. states filters by PR state
// ("OPEN"/"MERGED"/"DECLINED"/"SUPERSEDED"); an empty slice returns all states.
// A limit <= 0 means no cap. Each list entry is an abbreviated PR object, but it
// carries every field atlit's table needs, so no per-PR follow-up call is made.
func (c *Client) ListPullRequests(workspace, repo string, states []string, limit int) ([]PullRequest, error) {
	q := url.Values{}
	q.Set("pagelen", "50")
	q.Set("sort", "-updated_on")
	// The list endpoint omits participants from its abbreviated PR objects; the
	// "+" prefix adds them back without dropping the other default fields, so the
	// review column can be filled without a per-PR follow-up call.
	q.Set("fields", "+values.participants")
	for _, s := range states {
		q.Add("state", s)
	}
	next := fmt.Sprintf("/repositories/%s/%s/pullrequests?%s", workspace, repo, q.Encode())
	var all []PullRequest
	for next != "" {
		body, err := c.getJSON(next)
		if err != nil {
			return nil, err
		}
		var page struct {
			Values []PullRequest `json:"values"`
			Next   string        `json:"next"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decoding pull requests: %w", err)
		}
		all = append(all, page.Values...)
		if limit > 0 && len(all) >= limit {
			all = all[:limit]
			break
		}
		next = page.Next
	}
	return all, nil
}

// VerifyWorkspace checks the token can read the given workspace's repositories.
func (c *Client) VerifyWorkspace(workspace string) error {
	_, err := c.getJSON("/repositories/" + url.PathEscape(workspace) + "?pagelen=1")
	return err
}

// getJSON performs a GET expecting JSON, returning the body after status checks.
// pathOrURL may be a path (prefixed with baseURL) or a full URL (pagination next).
func (c *Client) getJSON(pathOrURL string) ([]byte, error) {
	resp, err := c.do(http.MethodGet, pathOrURL, "application/json")
	if err != nil {
		return nil, err
	}
	body, status, err := readAndClose(resp)
	if err != nil {
		return nil, err
	}
	if err := classify(status, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (c *Client) do(method, pathOrURL, accept string) (*http.Response, error) {
	target := pathOrURL
	if !strings.HasPrefix(target, "http") {
		target = c.baseURL + pathOrURL
	}
	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	return resp, nil
}

func readAndClose(resp *http.Response) ([]byte, int, error) {
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response body: %w", err)
	}
	return data, resp.StatusCode, nil
}

func classify(status int, body []byte) error {
	switch status {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusNotFound:
		return ErrNotFound
	}
	if status < 200 || status >= 300 {
		return &APIError{StatusCode: status, Message: string(body)}
	}
	return nil
}
