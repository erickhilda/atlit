package bitbucket

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(ts *httptest.Server) *Client {
	return &Client{
		baseURL:    ts.URL,
		authHeader: "Basic " + base64.StdEncoding.EncodeToString([]byte("me@example.com:tok")),
		http:       ts.Client(),
	}
}

func TestGetPullRequest(t *testing.T) {
	var gotAuth, gotAccept, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"id":42,"title":"Fix bug","state":"OPEN",
			"author":{"display_name":"Alice"},
			"source":{"branch":{"name":"feature/PROJ-1-x"}},
			"destination":{"branch":{"name":"develop"}}}`))
	}))
	defer ts.Close()

	pr, err := testClient(ts).GetPullRequest("ws", "repo", 42)
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	if pr.ID != 42 || pr.Title != "Fix bug" || pr.State != "OPEN" {
		t.Errorf("unexpected PR: %+v", pr)
	}
	if pr.Author.DisplayName != "Alice" {
		t.Errorf("author = %q", pr.Author.DisplayName)
	}
	if pr.Source.Branch.Name != "feature/PROJ-1-x" || pr.Destination.Branch.Name != "develop" {
		t.Errorf("branches = %q -> %q", pr.Source.Branch.Name, pr.Destination.Branch.Name)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Errorf("auth header = %q", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Errorf("accept = %q", gotAccept)
	}
	if gotPath != "/repositories/ws/repo/pullrequests/42" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestGetPullRequestDiff(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/diff") {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "text/plain" {
			t.Errorf("accept = %q, want text/plain", got)
		}
		_, _ = w.Write([]byte("diff --git a/x b/x\n+added\n"))
	}))
	defer ts.Close()

	diff, err := testClient(ts).GetPullRequestDiff("ws", "repo", 1)
	if err != nil {
		t.Fatalf("GetPullRequestDiff: %v", err)
	}
	if !strings.Contains(diff, "diff --git a/x b/x") {
		t.Errorf("diff = %q", diff)
	}
}

func TestGetPullRequestCommentsPagination(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"values":[{"id":2,"content":{"raw":"second"},"user":{"display_name":"Bob"}}]}`))
			return
		}
		// First page points "next" at a full URL for page 2.
		_, _ = fmt.Fprintf(w, `{"values":[{"id":1,"content":{"raw":"first"},"user":{"display_name":"Alice"}}],"next":"%s/repositories/ws/repo/pullrequests/1/comments?page=2"}`, ts.URL)
	}))
	defer ts.Close()

	comments, err := testClient(ts).GetPullRequestComments("ws", "repo", 1)
	if err != nil {
		t.Fatalf("GetPullRequestComments: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("got %d comments, want 2", len(comments))
	}
	if comments[0].Content.Raw != "first" || comments[1].Content.Raw != "second" {
		t.Errorf("comments = %q, %q", comments[0].Content.Raw, comments[1].Content.Raw)
	}
}

func TestListPullRequestsPagination(t *testing.T) {
	var gotStates []string
	var gotSort, gotPagelen string
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"values":[{"id":2,"title":"Two"}]}`))
			return
		}
		q := r.URL.Query()
		gotStates = q["state"]
		gotSort = q.Get("sort")
		gotPagelen = q.Get("pagelen")
		_, _ = fmt.Fprintf(w, `{"values":[{"id":1,"title":"One"}],"next":"%s/repositories/ws/repo/pullrequests?page=2"}`, ts.URL)
	}))
	defer ts.Close()

	prs, err := testClient(ts).ListPullRequests("ws", "repo", []string{"OPEN", "MERGED"}, 0)
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) != 2 || prs[0].ID != 1 || prs[1].ID != 2 {
		t.Fatalf("got %d PRs %v, want ids 1,2", len(prs), prs)
	}
	if strings.Join(gotStates, ",") != "OPEN,MERGED" {
		t.Errorf("state params = %v, want [OPEN MERGED]", gotStates)
	}
	if gotSort != "-updated_on" {
		t.Errorf("sort = %q, want -updated_on", gotSort)
	}
	if gotPagelen != "50" {
		t.Errorf("pagelen = %q, want 50", gotPagelen)
	}
}

func TestListPullRequestsLimit(t *testing.T) {
	var gotStates []string
	pages := 0
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		if pages == 1 {
			gotStates = r.URL.Query()["state"]
		}
		// Always return 3 with a next link, so only the limit stops the loop.
		_, _ = fmt.Fprintf(w, `{"values":[{"id":1},{"id":2},{"id":3}],"next":"%s/repositories/ws/repo/pullrequests?page=%d"}`, ts.URL, pages+1)
	}))
	defer ts.Close()

	prs, err := testClient(ts).ListPullRequests("ws", "repo", nil, 2)
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("got %d PRs, want 2 (limit)", len(prs))
	}
	if pages != 1 {
		t.Errorf("made %d page requests, want 1 (limit hit on first page)", pages)
	}
	if len(gotStates) != 0 {
		t.Errorf("state params = %v, want none for nil states (all)", gotStates)
	}
}

func TestStatusErrors(t *testing.T) {
	cases := []struct {
		code int
		want error
	}{
		{http.StatusUnauthorized, ErrUnauthorized},
		{http.StatusForbidden, ErrForbidden},
		{http.StatusNotFound, ErrNotFound},
	}
	for _, tc := range cases {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.code)
		}))
		_, err := testClient(ts).GetPullRequest("ws", "repo", 1)
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d: got %v, want %v", tc.code, err, tc.want)
		}
		ts.Close()
	}
}
