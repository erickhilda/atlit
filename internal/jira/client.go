package jira

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Client is an authenticated Jira Cloud HTTP client.
type Client struct {
	baseURL    string
	authHeader string
	http       *http.Client
}

// NewClient creates a Jira client with Basic auth (email:apiToken).
func NewClient(baseURL, email, token string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	creds := base64.StdEncoding.EncodeToString([]byte(email + ":" + token))
	return &Client{
		baseURL:    baseURL,
		authHeader: "Basic " + creds,
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Myself calls GET /rest/api/3/myself to verify credentials and retrieve
// the authenticated user's profile.
func (c *Client) Myself() (*User, error) {
	resp, err := c.do(http.MethodGet, "/rest/api/3/myself", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
		}
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decoding user response: %w", err)
	}
	return &user, nil
}

// GetIssue fetches a Jira issue by key with all default fields, using a
// two-pass decode to handle custom fields (sprint, epic) via expand=names.
func (c *Client) GetIssue(key string) (*Issue, error) {
	return c.GetIssueWithFields(key, "")
}

// GetIssueWithFields fetches a Jira issue by key, optionally narrowing the
// returned fields via the Jira "fields" query param (e.g. "*all,-comment" to
// request everything except comments). An empty fields string requests the
// server default. Custom field extraction (sprint, epic) uses expand=names.
func (c *Client) GetIssueWithFields(key, fieldsQuery string) (*Issue, error) {
	path := "/rest/api/3/issue/" + key + "?expand=names"
	if fieldsQuery != "" {
		path += "&fields=" + url.QueryEscape(fieldsQuery)
	}
	resp, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
		}
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	// First pass: decode into raw to preserve fields JSON and get names map.
	var raw IssueRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decoding issue (raw): %w", err)
	}

	// Second pass: decode the known fields.
	var fields IssueFields
	if err := json.Unmarshal(raw.Fields, &fields); err != nil {
		return nil, fmt.Errorf("decoding issue fields: %w", err)
	}

	issue := &Issue{
		ID:     raw.ID,
		Key:    raw.Key,
		Fields: fields,
	}

	// Extract custom fields using the names map.
	extractCustomFields(issue, raw.Fields, raw.Names)

	return issue, nil
}

// extractCustomFields scans the names map to find Sprint and Epic custom
// field IDs, then parses their values from the raw fields JSON.
func extractCustomFields(issue *Issue, rawFields json.RawMessage, names map[string]string) {
	if len(names) == 0 || len(rawFields) == 0 {
		return
	}

	// Find custom field IDs by their display name.
	var sprintFieldID, epicFieldID string
	for id, name := range names {
		switch strings.ToLower(name) {
		case "sprint":
			sprintFieldID = id
		case "epic link":
			epicFieldID = id
		}
	}

	// Parse the entire fields object into a generic map for custom field access.
	var allFields map[string]json.RawMessage
	if err := json.Unmarshal(rawFields, &allFields); err != nil {
		return
	}

	if sprintFieldID != "" {
		if raw, ok := allFields[sprintFieldID]; ok {
			issue.Sprint = parseSprint(raw)
		}
	}
	if epicFieldID != "" {
		if raw, ok := allFields[epicFieldID]; ok {
			issue.Epic = parseEpic(raw)
		}
	}
}

// parseSprint extracts sprint info from a custom field value.
// The field can be a single object or an array; we take the last (active) sprint.
func parseSprint(raw json.RawMessage) *Sprint {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	// Try as array first (common format).
	var sprints []Sprint
	if err := json.Unmarshal(raw, &sprints); err == nil && len(sprints) > 0 {
		return &sprints[len(sprints)-1]
	}

	// Try as single object.
	var sprint Sprint
	if err := json.Unmarshal(raw, &sprint); err == nil && sprint.Name != "" {
		return &sprint
	}
	return nil
}

// parseEpic extracts epic info from a custom field value.
// It can be a string (epic key) or an object with key+summary.
func parseEpic(raw json.RawMessage) *Epic {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	// Try as object first.
	var epic Epic
	if err := json.Unmarshal(raw, &epic); err == nil && epic.Key != "" {
		return &epic
	}

	// Try as string (epic key only).
	var key string
	if err := json.Unmarshal(raw, &key); err == nil && key != "" {
		return &Epic{Key: key}
	}
	return nil
}

// SearchIssues performs a JQL search using the /rest/api/3/search/jql endpoint
// and returns matching issues (handling pagination automatically).
//
// The fields parameter controls which fields are returned; pass nil for
// defaults. maxResults caps the number of issues returned: pagination stops
// once that many have been collected, so a broad query does not fetch the whole
// result set. Pass 0 (or a negative value) to fetch every matching issue.
func (c *Client) SearchIssues(jql string, fields []string, maxResults int) (*SearchResult, error) {
	result := &SearchResult{}
	var nextPageToken string

	for {
		params := url.Values{}
		params.Set("jql", jql)
		params.Set("expand", "names")
		if len(fields) > 0 {
			params.Set("fields", strings.Join(fields, ","))
		}
		if maxResults > 0 {
			// Request only what is still needed, capped at the API page max (100).
			remaining := maxResults - len(result.Issues)
			if remaining > 100 {
				remaining = 100
			}
			params.Set("maxResults", strconv.Itoa(remaining))
		}
		if nextPageToken != "" {
			params.Set("nextPageToken", nextPageToken)
		}

		resp, err := c.do(http.MethodGet, "/rest/api/3/search/jql?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}

		data, statusCode, err := readAndClose(resp)
		if err != nil {
			return nil, err
		}

		switch statusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, ErrUnauthorized
		}
		if statusCode != http.StatusOK {
			return nil, &APIError{
				StatusCode: statusCode,
				Message:    string(data),
			}
		}

		// Decode the search response page.
		var page struct {
			Issues        []json.RawMessage `json:"issues"`
			Names         map[string]string `json:"names"`
			NextPageToken string            `json:"nextPageToken"`
			IsLast        bool              `json:"isLast"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, fmt.Errorf("decoding search response: %w", err)
		}

		// Two-pass decode each issue, reusing the top-level names map.
		for _, rawIssue := range page.Issues {
			issue, err := decodeIssue(rawIssue, page.Names)
			if err != nil {
				return nil, err
			}
			result.Issues = append(result.Issues, *issue)
		}

		// Stop once the caller's cap is reached (trim any overshoot from the
		// final page) or the server reports the last page.
		if maxResults > 0 && len(result.Issues) >= maxResults {
			result.Issues = result.Issues[:maxResults]
			break
		}
		if page.IsLast || page.NextPageToken == "" {
			result.IsLast = true
			break
		}
		nextPageToken = page.NextPageToken
	}

	return result, nil
}

// SearchUsers calls GET /rest/api/3/user/search to find users whose name or
// email matches the query. It is used to resolve a human-friendly assignee name
// (e.g. "alice") to an accountId for use in JQL, since Jira Cloud matches the
// assignee field by accountId rather than display name.
func (c *Client) SearchUsers(query string) ([]User, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("maxResults", "50")

	resp, err := c.do(http.MethodGet, "/rest/api/3/user/search?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	data, statusCode, err := readAndClose(resp)
	if err != nil {
		return nil, err
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	}
	if statusCode != http.StatusOK {
		return nil, &APIError{
			StatusCode: statusCode,
			Message:    string(data),
		}
	}

	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("decoding user search response: %w", err)
	}
	return users, nil
}

// decodeIssue performs the two-pass JSON decode for a single issue.
func decodeIssue(rawIssue json.RawMessage, fallbackNames map[string]string) (*Issue, error) {
	var raw IssueRaw
	if err := json.Unmarshal(rawIssue, &raw); err != nil {
		return nil, fmt.Errorf("decoding issue (raw): %w", err)
	}

	var fields IssueFields
	if err := json.Unmarshal(raw.Fields, &fields); err != nil {
		return nil, fmt.Errorf("decoding issue fields: %w", err)
	}

	issue := &Issue{
		ID:     raw.ID,
		Key:    raw.Key,
		Fields: fields,
	}

	names := raw.Names
	if len(names) == 0 {
		names = fallbackNames
	}
	extractCustomFields(issue, raw.Fields, names)
	return issue, nil
}

// UpdateDescription replaces the description of a Jira issue using
// PUT /rest/api/3/issue/{key}. Returns nil on 204 No Content.
func (c *Client) UpdateDescription(key string, doc *ADFDoc) error {
	payload := struct {
		Fields struct {
			Description *ADFDoc `json:"description"`
		} `json:"fields"`
	}{}
	payload.Fields.Description = doc

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling update payload: %w", err)
	}

	resp, err := c.do(http.MethodPut, "/rest/api/3/issue/"+key, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	_, statusCode, err := readAndClose(resp)
	if err != nil {
		return err
	}

	switch statusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return &APIError{StatusCode: statusCode, Message: fmt.Sprintf("unexpected status %d", statusCode)}
	}
}

// GetPullRequests returns the pull requests linked to an issue via Jira's
// development panel, using the dev-status API. It first queries the summary
// endpoint to discover which application types (bitbucket, github, ...) host
// PRs for this issue, then fetches the detail for each. This avoids hardcoding
// a provider and skips the detail round-trip when there are no linked PRs.
//
// issueID is the numeric issue id (Issue.ID), not the issue key. The dev-status
// API is unofficial (it backs the Jira UI) and may be unavailable on some
// instances; callers should treat errors as non-fatal.
func (c *Client) GetPullRequests(issueID string) ([]PullRequest, error) {
	if issueID == "" {
		return nil, nil
	}

	appTypes, err := c.devStatusPRAppTypes(issueID)
	if err != nil {
		return nil, err
	}

	var prs []PullRequest
	for _, appType := range appTypes {
		detail, err := c.devStatusPRDetail(issueID, appType)
		if err != nil {
			return nil, err
		}
		for i := range detail {
			detail[i].AppType = appType
		}
		prs = append(prs, detail...)
	}
	return prs, nil
}

// devStatusPRAppTypes calls the dev-status summary endpoint and returns the
// application types that have at least one pull request for the issue.
func (c *Client) devStatusPRAppTypes(issueID string) ([]string, error) {
	params := url.Values{}
	params.Set("issueId", issueID)
	resp, err := c.do(http.MethodGet, "/rest/dev-status/latest/issue/summary?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	data, statusCode, err := readAndClose(resp)
	if err != nil {
		return nil, err
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		return nil, ErrNotFound
	}
	if statusCode != http.StatusOK {
		return nil, &APIError{StatusCode: statusCode, Message: string(data)}
	}

	var summary devStatusSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("decoding dev-status summary: %w", err)
	}
	if summary.Summary.PullRequest.Overall.Count == 0 {
		return nil, nil
	}
	var appTypes []string
	for appType, info := range summary.Summary.PullRequest.ByInstanceType {
		if info.Count > 0 {
			appTypes = append(appTypes, appType)
		}
	}
	return appTypes, nil
}

// devStatusPRDetail fetches the pull-request detail for one application type.
func (c *Client) devStatusPRDetail(issueID, appType string) ([]PullRequest, error) {
	params := url.Values{}
	params.Set("issueId", issueID)
	params.Set("applicationType", appType)
	params.Set("dataType", "pullrequest")
	resp, err := c.do(http.MethodGet, "/rest/dev-status/latest/issue/detail?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	data, statusCode, err := readAndClose(resp)
	if err != nil {
		return nil, err
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		return nil, ErrNotFound
	}
	if statusCode != http.StatusOK {
		return nil, &APIError{StatusCode: statusCode, Message: string(data)}
	}

	var detail devStatusDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil, fmt.Errorf("decoding dev-status detail: %w", err)
	}
	var prs []PullRequest
	for _, d := range detail.Detail {
		prs = append(prs, d.PullRequests...)
	}
	for i := range prs {
		prs[i].URL = humanizePRURL(prs[i].URL, prs[i].RepositoryName)
	}
	return prs, nil
}

// uuidPRURLPattern matches dev-status PR URLs whose workspace/repo path
// segments are UUIDs in braces, as Bitbucket Cloud returns them, e.g.
// https://bitbucket.org/{ws-uuid}/{repo-uuid}/pull-requests/42.
var uuidPRURLPattern = regexp.MustCompile(`^(https?://[^/]+)/\{[^}/]*\}/\{[^}/]*\}(/.*)$`)

// humanizePRURL replaces the {uuid}/{uuid} workspace/repo segments in a
// dev-status PR URL with the human-readable "workspace/repo" name when the
// payload provides one. URLs that are already readable pass through unchanged.
func humanizePRURL(rawURL, repoFullName string) string {
	if repoFullName == "" || strings.Count(repoFullName, "/") != 1 {
		return rawURL
	}
	m := uuidPRURLPattern.FindStringSubmatch(rawURL)
	if m == nil {
		return rawURL
	}
	return m[1] + "/" + repoFullName + m[2]
}

// readAndClose reads the full body and closes it, returning data and status code.
func readAndClose(resp *http.Response) ([]byte, int, error) {
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response body: %w", err)
	}
	return data, resp.StatusCode, nil
}

func (c *Client) do(method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	return resp, nil
}
