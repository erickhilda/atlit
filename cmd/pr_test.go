package cmd

import (
	"testing"

	"github.com/erickhilda/jt/internal/bitbucket"
	"github.com/erickhilda/jt/internal/config"
)

func TestParseBitbucketRemote(t *testing.T) {
	cases := []struct {
		in       string
		ws, repo string
		wantErr  bool
	}{
		{"git@bitbucket.org:acme/widget.git", "acme", "widget", false},
		// Repo slugs may contain dots; only a trailing ".git" must be stripped.
		{"git@bitbucket.org:acme/svc.payments-worker.git", "acme", "svc.payments-worker", false},
		{"git@bitbucket.org:acme/archetype.api.git", "acme", "archetype.api", false},
		{"git@bitbucket.org:acme/edge-fn.add_headers.git", "acme", "edge-fn.add_headers", false},
		{"https://user@bitbucket.org/acme/widget.git", "acme", "widget", false},
		{"https://bitbucket.org/acme/web-frontend", "acme", "web-frontend", false},
		{"git@github.com:acme/widget.git", "", "", true},
		{"not-a-url", "", "", true},
	}
	for _, tc := range cases {
		ws, repo, err := parseBitbucketRemote(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: expected error, got %s/%s", tc.in, ws, repo)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tc.in, err)
			continue
		}
		if ws != tc.ws || repo != tc.repo {
			t.Errorf("%q: got %s/%s, want %s/%s", tc.in, ws, repo, tc.ws, tc.repo)
		}
	}
}

func TestDetectJiraKey(t *testing.T) {
	branchKey := &bitbucket.PullRequest{}
	branchKey.Source.Branch.Name = "feature/PROJ-1234_add-thing"
	if got := detectJiraKey(branchKey); got != "PROJ-1234" {
		t.Errorf("branch: got %q", got)
	}

	titleKey := &bitbucket.PullRequest{Title: "PROJ-99 fix the thing"}
	titleKey.Source.Branch.Name = "hotfix/no-key-here"
	if got := detectJiraKey(titleKey); got != "PROJ-99" {
		t.Errorf("title fallback: got %q", got)
	}

	none := &bitbucket.PullRequest{Title: "just a fix"}
	none.Source.Branch.Name = "dependabot/bump-lib"
	if got := detectJiraKey(none); got != "" {
		t.Errorf("no key: got %q", got)
	}
}

func TestPrFileKey(t *testing.T) {
	if got := prFileKey("acme", "widget", 42); got != "acme__widget__42" {
		t.Errorf("got %q", got)
	}
}

func TestResolvePRRefExplicit(t *testing.T) {
	cfg := &config.Config{BitbucketWorkspace: "acme"}

	ws, repo, id, err := resolvePRRef("other/gadget/7", cfg)
	if err != nil || ws != "other" || repo != "gadget" || id != 7 {
		t.Errorf("3-part: %s/%s/%d err=%v", ws, repo, id, err)
	}

	ws, repo, id, err = resolvePRRef("widget/15", cfg)
	if err != nil || ws != "acme" || repo != "widget" || id != 15 {
		t.Errorf("2-part: %s/%s/%d err=%v", ws, repo, id, err)
	}

	if _, _, _, err := resolvePRRef("widget/notanum", cfg); err == nil {
		t.Error("expected error for non-numeric id")
	}
	if _, _, _, err := resolvePRRef("widget/0", cfg); err == nil {
		t.Error("expected error for non-positive id")
	}
}
