package cmd

import (
	"strings"
	"testing"
	"time"
)

func mustRFC3339(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("setup: parsing %q: %v", s, err)
	}
	return ts
}

func TestParseJiraTime(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		// Real Jira Cloud format: millis + numeric offset without a colon.
		// This is the case plain time.Parse(time.RFC3339, ...) could NOT parse.
		{"2026-06-10T13:59:55.000+0700", false},
		{"2026-06-10T13:59:55.123+0000", false},
		{"2026-06-10T13:59:55.000Z", false},
		{"2026-06-10T13:59:55-0700", false},
		{"2026-06-10T13:59:55Z", false},      // RFC3339, no fraction
		{"2026-06-10T06:59:55+00:00", false}, // RFC3339 with colon offset
		{"not a timestamp", true},
		{"", true},
	}
	for _, c := range cases {
		_, err := parseJiraTime(c.in)
		if c.wantErr && err == nil {
			t.Errorf("parseJiraTime(%q): expected error, got nil", c.in)
		}
		if !c.wantErr && err != nil {
			t.Errorf("parseJiraTime(%q): unexpected error: %v", c.in, err)
		}
	}
}

func TestCheckStale(t *testing.T) {
	fetched := mustRFC3339(t, "2026-06-10T00:00:00Z")

	// Regression case: remote updated AFTER the local pull, in real Jira Cloud
	// format. The previous RFC3339-only parse failed silently here, so the guard
	// never fired and the user could overwrite remote changes.
	if err := checkStale("ABC-1", "2026-06-10T13:59:55.000+0700", fetched); err == nil {
		t.Error("expected stale error when remote updated after local pull")
	} else if !strings.Contains(err.Error(), "after your last pull") {
		t.Errorf("unexpected stale message: %v", err)
	}

	// Remote updated BEFORE the local pull -> no conflict.
	// 2026-06-09T13:59:55+07:00 == 2026-06-09T06:59:55Z, before the fetch.
	if err := checkStale("ABC-1", "2026-06-09T13:59:55.000+0700", fetched); err != nil {
		t.Errorf("expected no error when remote older than pull, got: %v", err)
	}

	// No remote timestamp -> allow (no info to act on).
	if err := checkStale("ABC-1", "", fetched); err != nil {
		t.Errorf("expected no error for empty remote timestamp, got: %v", err)
	}

	// Unparseable remote timestamp -> fail closed (refuse the push).
	if err := checkStale("ABC-1", "garbage", fetched); err == nil {
		t.Error("expected error (fail-closed) for unparseable remote timestamp")
	}
}
