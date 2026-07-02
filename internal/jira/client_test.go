package jira

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMyselfSuccess(t *testing.T) {
	want := User{
		AccountID:   "123abc",
		DisplayName: "Test User",
		Email:       "test@example.com",
		Active:      true,
		TimeZone:    "America/New_York",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/myself" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	got, err := client.Myself()
	if err != nil {
		t.Fatalf("Myself: %v", err)
	}

	if got.AccountID != want.AccountID {
		t.Errorf("AccountID = %q, want %q", got.AccountID, want.AccountID)
	}
	if got.DisplayName != want.DisplayName {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, want.DisplayName)
	}
	if got.Email != want.Email {
		t.Errorf("Email = %q, want %q", got.Email, want.Email)
	}
	if got.Active != want.Active {
		t.Errorf("Active = %v, want %v", got.Active, want.Active)
	}
}

func TestMyselfUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "bad@example.com", "wrongtoken")
	_, err := client.Myself()
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestMyselfServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	_, err := client.Myself()
	if err == nil {
		t.Fatal("expected error for 500 response")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got: %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", apiErr.StatusCode)
	}
}

func TestAuthHeader(t *testing.T) {
	email := "user@example.com"
	token := "api-token"
	wantCreds := base64.StdEncoding.EncodeToString([]byte(email + ":" + token))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		want := "Basic " + wantCreds
		if got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(User{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, email, token)
	_, _ = client.Myself()
}

func TestMyselfForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	_, err := client.Myself()
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized for 403, got: %v", err)
	}
}

// issueJSON is a realistic Jira issue response fixture for testing.
const issueJSON = `{
  "id": "10001",
  "key": "PROJ-123",
  "names": {
    "summary": "Summary",
    "status": "Status",
    "customfield_10020": "Sprint",
    "customfield_10014": "Epic Link"
  },
  "fields": {
    "summary": "Implement OAuth2 flow",
    "description": {
      "type": "doc",
      "version": 1,
      "content": [
        {
          "type": "paragraph",
          "content": [{"type": "text", "text": "Build the OAuth2 integration."}]
        }
      ]
    },
    "status": {"name": "In Progress"},
    "issuetype": {"name": "Story"},
    "priority": {"name": "High"},
    "assignee": {"accountId": "abc", "displayName": "Alice", "emailAddress": "alice@co.com"},
    "reporter": {"accountId": "def", "displayName": "Bob", "emailAddress": "bob@co.com"},
    "labels": ["backend", "security"],
    "created": "2026-02-01T10:00:00.000+0000",
    "updated": "2026-02-10T14:30:00.000+0000",
    "comment": {
      "total": 1,
      "comments": [
        {
          "author": {"accountId": "abc", "displayName": "Alice", "emailAddress": "alice@co.com"},
          "body": {
            "type": "doc",
            "version": 1,
            "content": [{"type": "paragraph", "content": [{"type": "text", "text": "LGTM"}]}]
          },
          "created": "2026-02-05T09:00:00.000+0000"
        }
      ]
    },
    "subtasks": [
      {"key": "PROJ-124", "fields": {"summary": "Research libraries", "status": {"name": "Done"}}},
      {"key": "PROJ-125", "fields": {"summary": "Implement storage", "status": {"name": "In Progress"}}}
    ],
    "issuelinks": [
      {
        "type": {"name": "Blocks", "outward": "blocks", "inward": "is blocked by"},
        "outwardIssue": {"key": "PROJ-130", "fields": {"summary": "Protected endpoints", "status": {"name": "To Do"}}}
      }
    ],
    "parent": {"key": "PROJ-80", "fields": {"summary": "Authentication Epic", "status": {"name": "In Progress"}}},
    "customfield_10020": [{"name": "Sprint 14"}],
    "customfield_10014": "PROJ-80"
  }
}`

func TestGetIssueSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/PROJ-123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("expand") != "names" {
			t.Errorf("expected expand=names query param, got: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(issueJSON))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	issue, err := client.GetIssue("PROJ-123")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}

	if issue.Key != "PROJ-123" {
		t.Errorf("Key = %q, want PROJ-123", issue.Key)
	}
	if issue.ID != "10001" {
		t.Errorf("ID = %q, want 10001", issue.ID)
	}
	if issue.Fields.Summary != "Implement OAuth2 flow" {
		t.Errorf("Summary = %q", issue.Fields.Summary)
	}
	if issue.Fields.Status == nil || issue.Fields.Status.Name != "In Progress" {
		t.Errorf("Status = %v", issue.Fields.Status)
	}
	if issue.Fields.IssueType == nil || issue.Fields.IssueType.Name != "Story" {
		t.Errorf("IssueType = %v", issue.Fields.IssueType)
	}
	if issue.Fields.Priority == nil || issue.Fields.Priority.Name != "High" {
		t.Errorf("Priority = %v", issue.Fields.Priority)
	}
	if issue.Fields.Assignee == nil || issue.Fields.Assignee.DisplayName != "Alice" {
		t.Errorf("Assignee = %v", issue.Fields.Assignee)
	}
	if issue.Fields.Reporter == nil || issue.Fields.Reporter.DisplayName != "Bob" {
		t.Errorf("Reporter = %v", issue.Fields.Reporter)
	}
	if len(issue.Fields.Labels) != 2 {
		t.Errorf("Labels = %v", issue.Fields.Labels)
	}
	if issue.Fields.Description == nil {
		t.Error("Description is nil")
	}
	if issue.Fields.Comment == nil || issue.Fields.Comment.Total != 1 {
		t.Errorf("Comment = %v", issue.Fields.Comment)
	}
	if len(issue.Fields.Subtasks) != 2 {
		t.Errorf("Subtasks count = %d, want 2", len(issue.Fields.Subtasks))
	}
	if len(issue.Fields.IssueLinks) != 1 {
		t.Errorf("IssueLinks count = %d, want 1", len(issue.Fields.IssueLinks))
	}
	if issue.Fields.Parent == nil || issue.Fields.Parent.Key != "PROJ-80" {
		t.Errorf("Parent = %v", issue.Fields.Parent)
	}
}

func TestGetIssueWithFieldsQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(issueJSON))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	if _, err := client.GetIssueWithFields("PROJ-123", "*all,-comment"); err != nil {
		t.Fatalf("GetIssueWithFields: %v", err)
	}

	// expand=names must still be present, and the fields param must be URL-encoded.
	if !strings.Contains(gotQuery, "expand=names") {
		t.Errorf("expected expand=names in query, got: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "fields=") {
		t.Errorf("expected fields= in query, got: %s", gotQuery)
	}
	// The literal request value "*all,-comment" contains a comma which survives
	// QueryEscape unchanged, but the "*" must remain (not mangled).
	if !strings.Contains(gotQuery, "%2Aall%2C-comment") && !strings.Contains(gotQuery, "*all%2C-comment") && !strings.Contains(gotQuery, "*all,-comment") {
		t.Errorf("expected fields=*all,-comment (any reasonable encoding) in query, got: %s", gotQuery)
	}
}

func TestGetIssueEmptyFieldsOmitsParam(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(issueJSON))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	if _, err := client.GetIssue("PROJ-123"); err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if strings.Contains(gotQuery, "fields=") {
		t.Errorf("expected no fields param for GetIssue, got: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "expand=names") {
		t.Errorf("expected expand=names, got: %s", gotQuery)
	}
}

func TestGetIssueCustomFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(issueJSON))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	issue, err := client.GetIssue("PROJ-123")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}

	if issue.Sprint == nil {
		t.Fatal("Sprint is nil, expected Sprint 14")
	}
	if issue.Sprint.Name != "Sprint 14" {
		t.Errorf("Sprint.Name = %q, want 'Sprint 14'", issue.Sprint.Name)
	}

	if issue.Epic == nil {
		t.Fatal("Epic is nil, expected PROJ-80")
	}
	if issue.Epic.Key != "PROJ-80" {
		t.Errorf("Epic.Key = %q, want 'PROJ-80'", issue.Epic.Key)
	}
}

func TestGetIssueNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errorMessages":["Issue does not exist"]}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	_, err := client.GetIssue("NOPE-999")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetIssueUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "bad@example.com", "wrong")
	_, err := client.GetIssue("PROJ-1")
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestGetIssueForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	_, err := client.GetIssue("PROJ-1")
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized for 403, got: %v", err)
	}
}

func TestGetIssueServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	_, err := client.GetIssue("PROJ-1")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got: %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", apiErr.StatusCode)
	}
}

// searchJSON is a JQL search response fixture for /rest/api/3/search/jql.
const searchJSON = `{
  "names": {
    "summary": "Summary",
    "status": "Status",
    "customfield_10020": "Sprint"
  },
  "issues": [
    {
      "key": "PROJ-123",
      "fields": {
        "summary": "Implement OAuth2 flow",
        "status": {"name": "In Progress"},
        "issuetype": {"name": "Story"},
        "priority": {"name": "High"},
        "created": "2026-02-01T10:00:00.000+0000",
        "updated": "2026-02-10T14:30:00.000+0000",
        "customfield_10020": [{"name": "Sprint 14"}]
      }
    }
  ],
  "isLast": true
}`

func TestSearchIssuesSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("jql") == "" {
			t.Error("expected jql query param")
		}
		if r.URL.Query().Get("expand") != "names" {
			t.Errorf("expected expand=names, got: %s", r.URL.Query().Get("expand"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(searchJSON))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	result, err := client.SearchIssues("key = PROJ-123", nil, 0)
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if !result.IsLast {
		t.Error("expected IsLast=true")
	}
	if len(result.Issues) != 1 {
		t.Fatalf("Issues count = %d, want 1", len(result.Issues))
	}
	if result.Issues[0].Key != "PROJ-123" {
		t.Errorf("Key = %q, want PROJ-123", result.Issues[0].Key)
	}
	if result.Issues[0].Fields.Summary != "Implement OAuth2 flow" {
		t.Errorf("Summary = %q", result.Issues[0].Fields.Summary)
	}
}

func TestSearchIssuesEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issues":[],"isLast":true}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	result, err := client.SearchIssues("key = NOPE-1", nil, 0)
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if len(result.Issues) != 0 {
		t.Errorf("Issues count = %d, want 0", len(result.Issues))
	}
}

func TestSearchIssuesPagination(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// First page: return one issue with nextPageToken.
			if r.URL.Query().Get("nextPageToken") != "" {
				t.Error("first request should not have nextPageToken")
			}
			_, _ = w.Write([]byte(`{
				"issues": [{"key": "PROJ-1", "fields": {"summary": "First"}}],
				"nextPageToken": "page2token",
				"isLast": false
			}`))
		} else {
			// Second page: return one issue, last page.
			if r.URL.Query().Get("nextPageToken") != "page2token" {
				t.Errorf("expected nextPageToken=page2token, got: %s", r.URL.Query().Get("nextPageToken"))
			}
			_, _ = w.Write([]byte(`{
				"issues": [{"key": "PROJ-2", "fields": {"summary": "Second"}}],
				"isLast": true
			}`))
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	result, err := client.SearchIssues("project = PROJ", nil, 0)
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
	if len(result.Issues) != 2 {
		t.Fatalf("Issues count = %d, want 2", len(result.Issues))
	}
	if result.Issues[0].Key != "PROJ-1" {
		t.Errorf("Issues[0].Key = %q, want PROJ-1", result.Issues[0].Key)
	}
	if result.Issues[1].Key != "PROJ-2" {
		t.Errorf("Issues[1].Key = %q, want PROJ-2", result.Issues[1].Key)
	}
}

func TestSearchIssuesUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "bad@example.com", "wrong")
	_, err := client.SearchIssues("key = PROJ-1", nil, 0)
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestSearchIssuesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	_, err := client.SearchIssues("key = PROJ-1", nil, 0)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got: %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", apiErr.StatusCode)
	}
}

func TestSearchIssuesLimitStopsPagination(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// The first (and only expected) page should request exactly maxResults=1.
		if got := r.URL.Query().Get("maxResults"); got != "1" {
			t.Errorf("maxResults = %q, want 1", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"issues": [
				{"key": "PROJ-1", "fields": {"summary": "First"}},
				{"key": "PROJ-2", "fields": {"summary": "Second"}}
			],
			"nextPageToken": "page2token",
			"isLast": false
		}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	result, err := client.SearchIssues("project = PROJ", nil, 1)
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 API call (limit reached), got %d", callCount)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("Issues count = %d, want 1 (trimmed to limit)", len(result.Issues))
	}
	if result.Issues[0].Key != "PROJ-1" {
		t.Errorf("Issues[0].Key = %q, want PROJ-1", result.Issues[0].Key)
	}
}

func TestSearchUsersSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/user/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "alice" {
			t.Errorf("query = %q, want alice", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"accountId": "5b10ac8d82e05b22cc7d4ef5", "displayName": "Alice Example", "emailAddress": "alice@example.com", "active": true}
		]`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	users, err := client.SearchUsers("alice")
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("users count = %d, want 1", len(users))
	}
	if users[0].AccountID != "5b10ac8d82e05b22cc7d4ef5" {
		t.Errorf("AccountID = %q", users[0].AccountID)
	}
	if users[0].DisplayName != "Alice Example" {
		t.Errorf("DisplayName = %q", users[0].DisplayName)
	}
}

func TestSearchUsersUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "bad@example.com", "wrong")
	_, err := client.SearchUsers("alice")
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

// devStatusSummaryJSON reports one bitbucket PR for the issue.
const devStatusSummaryJSON = `{
  "summary": {
    "pullrequest": {
      "overall": {"count": 1},
      "byInstanceType": {"bitbucket": {"count": 1, "name": "Bitbucket Cloud"}}
    }
  }
}`

const devStatusDetailJSON = `{
  "errors": [],
  "detail": [
    {
      "pullRequests": [
        {
          "id": "#42",
          "name": "Add feature",
          "url": "https://bitbucket.org/{1111aaaa-2222-bbbb-3333-cccc4444dddd}/{5555eeee-6666-ffff-7777-aaaa8888bbbb}/pull-requests/42",
          "status": "MERGED",
          "author": {"name": "alice"},
          "source": {"branch": "feature/PROJ-123"},
          "destination": {"branch": "develop"},
          "reviewers": [{"name": "bob", "approved": true}],
          "repositoryName": "acme/widget"
        }
      ]
    }
  ]
}`

func TestGetPullRequestsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/dev-status/latest/issue/summary":
			if got := r.URL.Query().Get("issueId"); got != "10001" {
				t.Errorf("summary issueId = %q, want 10001", got)
			}
			_, _ = w.Write([]byte(devStatusSummaryJSON))
		case "/rest/dev-status/latest/issue/detail":
			q := r.URL.Query()
			if q.Get("applicationType") != "bitbucket" {
				t.Errorf("applicationType = %q, want bitbucket", q.Get("applicationType"))
			}
			if q.Get("dataType") != "pullrequest" {
				t.Errorf("dataType = %q, want pullrequest", q.Get("dataType"))
			}
			_, _ = w.Write([]byte(devStatusDetailJSON))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	prs, err := client.GetPullRequests("10001")
	if err != nil {
		t.Fatalf("GetPullRequests: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d PRs, want 1", len(prs))
	}
	pr := prs[0]
	if pr.ID != "#42" || pr.Status != "MERGED" || pr.Name != "Add feature" {
		t.Errorf("unexpected PR: %+v", pr)
	}
	if pr.AppType != "bitbucket" {
		t.Errorf("AppType = %q, want bitbucket", pr.AppType)
	}
	if pr.Author == nil || pr.Author.Name != "alice" {
		t.Errorf("expected author alice, got %+v", pr.Author)
	}
	if pr.RepositoryName != "acme/widget" {
		t.Errorf("RepositoryName = %q, want acme/widget", pr.RepositoryName)
	}
	if want := "https://bitbucket.org/acme/widget/pull-requests/42"; pr.URL != want {
		t.Errorf("URL = %q, want %q (uuid segments should be humanized)", pr.URL, want)
	}
}

func TestHumanizePRURL(t *testing.T) {
	tests := []struct {
		name, rawURL, repoName, want string
	}{
		{
			name:     "uuid segments replaced",
			rawURL:   "https://bitbucket.org/{1111aaaa-2222-bbbb-3333-cccc4444dddd}/{5555eeee-6666-ffff-7777-aaaa8888bbbb}/pull-requests/42",
			repoName: "acme/widget",
			want:     "https://bitbucket.org/acme/widget/pull-requests/42",
		},
		{
			name:     "empty workspace uuid",
			rawURL:   "https://bitbucket.org/{}/{5555eeee-6666-ffff-7777-aaaa8888bbbb}/pull-requests/42",
			repoName: "acme/widget",
			want:     "https://bitbucket.org/acme/widget/pull-requests/42",
		},
		{
			name:     "already readable passes through",
			rawURL:   "https://bitbucket.org/acme/widget/pull-requests/42",
			repoName: "acme/widget",
			want:     "https://bitbucket.org/acme/widget/pull-requests/42",
		},
		{
			name:     "no repository name",
			rawURL:   "https://bitbucket.org/{1111aaaa-2222-bbbb-3333-cccc4444dddd}/{5555eeee-6666-ffff-7777-aaaa8888bbbb}/pull-requests/42",
			repoName: "",
			want:     "https://bitbucket.org/{1111aaaa-2222-bbbb-3333-cccc4444dddd}/{5555eeee-6666-ffff-7777-aaaa8888bbbb}/pull-requests/42",
		},
		{
			name:     "unexpected repo name shape left alone",
			rawURL:   "https://bitbucket.org/{1111aaaa-2222-bbbb-3333-cccc4444dddd}/{5555eeee-6666-ffff-7777-aaaa8888bbbb}/pull-requests/42",
			repoName: "widget",
			want:     "https://bitbucket.org/{1111aaaa-2222-bbbb-3333-cccc4444dddd}/{5555eeee-6666-ffff-7777-aaaa8888bbbb}/pull-requests/42",
		},
		{
			name:     "github url untouched",
			rawURL:   "https://github.com/acme/widget/pull/42",
			repoName: "acme/widget",
			want:     "https://github.com/acme/widget/pull/42",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := humanizePRURL(tt.rawURL, tt.repoName); got != tt.want {
				t.Errorf("humanizePRURL(%q, %q) = %q, want %q", tt.rawURL, tt.repoName, got, tt.want)
			}
		})
	}
}

func TestGetPullRequestsNoneSkipsDetail(t *testing.T) {
	detailCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/dev-status/latest/issue/summary":
			_, _ = w.Write([]byte(`{"summary":{"pullrequest":{"overall":{"count":0},"byInstanceType":{}}}}`))
		case "/rest/dev-status/latest/issue/detail":
			detailCalled = true
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	prs, err := client.GetPullRequests("10001")
	if err != nil {
		t.Fatalf("GetPullRequests: %v", err)
	}
	if len(prs) != 0 {
		t.Errorf("got %d PRs, want 0", len(prs))
	}
	if detailCalled {
		t.Error("detail endpoint should not be called when summary count is 0")
	}
}

func TestGetPullRequestsEmptyIssueID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request expected for empty issueID, got %s", r.URL.Path)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test@example.com", "token123")
	prs, err := client.GetPullRequests("")
	if err != nil || prs != nil {
		t.Errorf("expected (nil, nil) for empty issueID, got (%v, %v)", prs, err)
	}
}
