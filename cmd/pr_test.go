package cmd

import (
	"strings"
	"testing"
	"time"

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

func TestMapPRStates(t *testing.T) {
	cases := []struct {
		in     string
		states string // comma-joined
		label  string
		err    bool
	}{
		{"", "OPEN", "Open", false},
		{"open", "OPEN", "Open", false},
		{"OPEN", "OPEN", "Open", false},
		{"merged", "MERGED", "Merged", false},
		{"declined", "DECLINED", "Declined", false},
		{"all", "", "All", false},
		{"bogus", "", "", true},
	}
	for _, tc := range cases {
		states, label, err := mapPRStates(tc.in)
		if tc.err {
			if err == nil {
				t.Errorf("%q: expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tc.in, err)
			continue
		}
		if strings.Join(states, ",") != tc.states || label != tc.label {
			t.Errorf("%q: got %v/%q, want %s/%s", tc.in, states, label, tc.states, tc.label)
		}
	}
}

func TestResolveRepoRef(t *testing.T) {
	cfg := &config.Config{BitbucketWorkspace: "acme"}

	ws, repo, err := resolveRepoRef("other/gadget", cfg)
	if err != nil || ws != "other" || repo != "gadget" {
		t.Errorf("ws/repo: %s/%s err=%v", ws, repo, err)
	}

	ws, repo, err = resolveRepoRef("widget", cfg)
	if err != nil || ws != "acme" || repo != "widget" {
		t.Errorf("repo + config ws: %s/%s err=%v", ws, repo, err)
	}

	if _, _, err := resolveRepoRef("a/b/c", cfg); err == nil {
		t.Error("expected error for 3-part reference")
	}
	if _, _, err := resolveRepoRef("/gadget", cfg); err == nil {
		t.Error("expected error for empty workspace")
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"short", 10, "short"},
		{"exactlyten", 10, "exactlyten"},
		{"this is way too long", 10, "this is w…"},
		{"héllo wörld", 6, "héllo…"}, // rune-safe: multibyte not split mid-byte
		{"x", 0, ""},
	}
	for _, tc := range cases {
		if got := truncate(tc.in, tc.max); got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
		}
	}
}

func TestFormatPRUpdated(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	// Bitbucket-style timestamp: colon offset + fractional seconds, ~2.5d before now.
	if got := formatPRUpdated(now, "2026-06-19T00:00:00.123456+00:00"); got != "2d ago" {
		t.Errorf("relative age = %q, want 2d ago", got)
	}
	if got := formatPRUpdated(now, "not-a-time"); got != "not-a-time" {
		t.Errorf("fallback = %q, want raw passthrough", got)
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
