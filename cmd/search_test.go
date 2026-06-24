package cmd

import (
	"strings"
	"testing"

	"github.com/erickhilda/jt/internal/jira"
)

func TestValidateSearchFlags(t *testing.T) {
	cases := []struct {
		name        string
		status      string
		assignee    string
		mine        bool
		active      bool
		jql         string
		project     string
		allProjects bool
		wantErr     string // substring; "" means no error
	}{
		{name: "status only", status: "code review"},
		{name: "assignee only", assignee: "alice"},
		{name: "mine only", mine: true},
		{name: "active only", active: true},
		{name: "jql only", jql: "project = FOO"},
		{name: "status with project scope", status: "stage test", project: "FOO"},
		{name: "mine all-projects", mine: true, allProjects: true},
		{name: "mine and active", mine: true, active: true},

		{name: "no filter", wantErr: "at least one filter"},
		{name: "project alone is not a filter", project: "FOO", wantErr: "at least one filter"},
		{name: "jql plus status", jql: "x", status: "code review", wantErr: "--jql cannot be combined"},
		{name: "jql plus mine", jql: "x", mine: true, wantErr: "--jql cannot be combined"},
		{name: "jql plus active", jql: "x", active: true, wantErr: "--jql cannot be combined"},
		{name: "jql plus project", jql: "x", project: "FOO", wantErr: "--jql cannot be combined"},
		{name: "mine plus assignee", mine: true, assignee: "alice", wantErr: "mutually exclusive"},
		{name: "project plus all-projects", status: "x", project: "FOO", allProjects: true, wantErr: "mutually exclusive"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSearchFlags(tc.status, tc.assignee, tc.mine, tc.active, tc.jql, tc.project, tc.allProjects)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestBuildJQL(t *testing.T) {
	cases := []struct {
		name           string
		project        string
		status         string
		assigneeClause string
		active         bool
		want           string
	}{
		{
			name:    "project and single status",
			project: "PROJ",
			status:  "code review",
			want:    `project = "PROJ" AND status = "code review" ORDER BY updated DESC`,
		},
		{
			name:   "multiple statuses become in()",
			status: "code review, stage test",
			want:   `status in ("code review", "stage test") ORDER BY updated DESC`,
		},
		{
			name:           "assignee clause passed through",
			project:        "FOO",
			assigneeClause: "assignee = currentUser()",
			want:           `project = "FOO" AND assignee = currentUser() ORDER BY updated DESC`,
		},
		{
			name:           "all three clauses",
			project:        "FOO",
			status:         "Done",
			assigneeClause: `assignee = "5b10ac"`,
			want:           `project = "FOO" AND status = "Done" AND assignee = "5b10ac" ORDER BY updated DESC`,
		},
		{
			name:           "no project scope",
			assigneeClause: "assignee = currentUser()",
			want:           `assignee = currentUser() ORDER BY updated DESC`,
		},
		{
			name:           "active with mine and project",
			project:        "PROJ",
			assigneeClause: "assignee = currentUser()",
			active:         true,
			want:           `project = "PROJ" AND statusCategory != Done AND assignee = currentUser() ORDER BY updated DESC`,
		},
		{
			name:    "active alone (with project scope)",
			project: "FOO",
			active:  true,
			want:    `project = "FOO" AND statusCategory != Done ORDER BY updated DESC`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildJQL(tc.project, tc.status, tc.assigneeClause, tc.active)
			if got != tc.want {
				t.Fatalf("buildJQL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestQuoteJQL(t *testing.T) {
	cases := []struct{ in, want string }{
		{`code review`, `"code review"`},
		{`needs "review"`, `"needs \"review\""`},
		{`back\slash`, `"back\\slash"`},
	}
	for _, tc := range cases {
		if got := quoteJQL(tc.in); got != tc.want {
			t.Errorf("quoteJQL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPickUser(t *testing.T) {
	alice := jira.User{AccountID: "acc-alice", DisplayName: "Alice Example", Email: "alice@example.com"}
	chrisA := jira.User{AccountID: "acc-chris-a", DisplayName: "Chris Adams", Email: "chris.a@example.com"}
	chrisB := jira.User{AccountID: "acc-chris-b", DisplayName: "Chris Brown", Email: "chris.b@example.com"}
	appUser := jira.User{AccountID: "", DisplayName: "Automation"} // no accountId -> skipped

	t.Run("single match", func(t *testing.T) {
		got, err := pickUser("alice", []jira.User{alice})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "acc-alice" {
			t.Fatalf("accountId = %q, want acc-alice", got)
		}
	})

	t.Run("skips accounts without id", func(t *testing.T) {
		got, err := pickUser("alice", []jira.User{appUser, alice})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "acc-alice" {
			t.Fatalf("accountId = %q, want acc-alice", got)
		}
	})

	t.Run("no match", func(t *testing.T) {
		_, err := pickUser("nobody", nil)
		if err == nil || !strings.Contains(err.Error(), "no Jira user matches") {
			t.Fatalf("want no-match error, got %v", err)
		}
	})

	t.Run("ambiguous lists candidates", func(t *testing.T) {
		_, err := pickUser("chris", []jira.User{chrisA, chrisB})
		if err == nil || !strings.Contains(err.Error(), "refine") {
			t.Fatalf("want ambiguity error, got %v", err)
		}
		if !strings.Contains(err.Error(), "Chris Adams") || !strings.Contains(err.Error(), "Chris Brown") {
			t.Fatalf("ambiguity error should list both candidates: %v", err)
		}
	})

	t.Run("exact name breaks tie", func(t *testing.T) {
		got, err := pickUser("Chris Brown", []jira.User{chrisA, chrisB})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "acc-chris-b" {
			t.Fatalf("accountId = %q, want acc-chris-b", got)
		}
	})
}

func TestStatusClause(t *testing.T) {
	if got := statusClause("Done"); got != `status = "Done"` {
		t.Errorf("single status = %q", got)
	}
	if got := statusClause("a, b ,c"); got != `status in ("a", "b", "c")` {
		t.Errorf("multi status = %q", got)
	}
	// A stray trailing comma collapses to a single value, not a leaked one.
	if got := statusClause("Done,"); got != `status = "Done"` {
		t.Errorf("trailing comma = %q, want status = \"Done\"", got)
	}
	if got := statusClause("code review, "); got != `status = "code review"` {
		t.Errorf("trailing comma+space = %q", got)
	}
}
