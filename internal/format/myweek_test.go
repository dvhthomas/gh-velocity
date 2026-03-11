package format

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

var (
	testNow   = time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	testSince = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
)

func testMyWeekResult() model.MyWeekResult {
	return model.MyWeekResult{
		Login: "testuser",
		Repo:  "owner/repo",
		Since: testSince,
		Until: testNow,
		IssuesClosed: []model.Issue{
			{Number: 1, Title: "Fix crash", URL: "https://github.com/owner/repo/issues/1"},
			{Number: 5, Title: "Update docs", URL: "https://github.com/owner/repo/issues/5"},
		},
		PRsMerged: []model.PR{
			{Number: 10, Title: "Add feature", URL: "https://github.com/owner/repo/pull/10"},
		},
		PRsReviewed: []model.PR{
			{Number: 20, Title: "Refactor auth", URL: "https://github.com/owner/repo/pull/20"},
		},
		IssuesOpen: []model.Issue{
			// New: created within --since window
			{Number: 42, Title: "Implement caching", URL: "https://github.com/owner/repo/issues/42",
				CreatedAt: testSince.Add(2 * 24 * time.Hour), UpdatedAt: testSince.Add(2 * 24 * time.Hour)},
			// Stale: created 20 days ago, no update in 15 days
			{Number: 30, Title: "Old backlog item", URL: "https://github.com/owner/repo/issues/30",
				CreatedAt: testNow.Add(-20 * 24 * time.Hour), UpdatedAt: testNow.Add(-15 * 24 * time.Hour)},
		},
		PRsOpen: []model.PR{
			// Needs review: created before --since, 10 days old, in PRsNeedingReview
			{Number: 43, Title: "WIP: add my-week", URL: "https://github.com/owner/repo/pull/43",
				CreatedAt: testSince.Add(-3 * 24 * time.Hour)},
			// Normal active PR: created before --since, 2 days old
			{Number: 44, Title: "Fix typo", URL: "https://github.com/owner/repo/pull/44",
				CreatedAt: testSince.Add(-1 * 24 * time.Hour)},
		},
		PRsNeedingReview: []model.PR{
			{Number: 43, Title: "WIP: add my-week", URL: "https://github.com/owner/repo/pull/43",
				CreatedAt: testSince.Add(-3 * 24 * time.Hour)},
		},
	}
}

func TestWriteMyWeekPretty(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Pretty}
	if err := WriteMyWeekPretty(rc, testMyWeekResult()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"testuser",
		"Insights",
		"Shipped 3 items", "in 7 days",
		"Reviewed 1 PRs",
		"1 of your open PR(s) waiting for first review",
		"1 open issue(s) stale",
		"New work picked up: 1 issue(s).",
		"What I shipped",
		"Issues Closed: 2", "PRs Merged: 1", "PRs Reviewed: 1",
		"#1", "#10", "#20",
		"What's ahead",
		"Open Issues: 2", "#42", "Implement caching",
		"Open PRs: 2", "#43", "WIP: add my-week",
		"<- new",          // #42 should be annotated as new
		"<- needs review", // #43 should be annotated as needs review
		"<- stale",        // #30 should be annotated as stale
	} {
		if !contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestWriteMyWeekPretty_Empty(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Pretty}
	r := model.MyWeekResult{
		Login: "testuser",
		Repo:  "owner/repo",
		Since: testSince,
		Until: testNow,
	}
	if err := WriteMyWeekPretty(rc, r); err != nil {
		t.Fatal(err)
	}
	if !contains(buf.String(), "No activity") {
		t.Error("expected 'No activity' for empty result")
	}
	if contains(buf.String(), "What I shipped") {
		t.Error("should not show 'What I shipped' when empty")
	}
}

func TestWriteMyWeekMarkdown(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Markdown}
	if err := WriteMyWeekMarkdown(rc, testMyWeekResult()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"## My Week",
		"### Insights",
		"Shipped 3 items",
		"waiting for first review",
		"stale",
		"### What I shipped",
		"**Issues Closed (2)**", "**PRs Merged (1)**", "**PRs Reviewed (1)**",
		"[#1]", "[#10]",
		"### What's ahead",
		"**Open Issues (2)**", "[#42]",
		"**Open PRs (2)**", "[#43]",
		"`new ",          // #42 annotation
		"`needs review ", // #43 annotation
		"`stale ",        // #30 annotation
	} {
		if !contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestWriteMyWeekMarkdown_Empty(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Markdown}
	r := model.MyWeekResult{Login: "u", Repo: "o/r", Since: testSince, Until: testNow}
	if err := WriteMyWeekMarkdown(rc, r); err != nil {
		t.Fatal(err)
	}
	if !contains(buf.String(), "_None_") {
		t.Error("expected '_None_' for empty sections")
	}
}

func TestWriteMyWeekJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteMyWeekJSON(&buf, testMyWeekResult()); err != nil {
		t.Fatal(err)
	}

	var parsed jsonMyWeekResult
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Login != "testuser" {
		t.Errorf("login = %q, want testuser", parsed.Login)
	}
	if len(parsed.Insights.Lines) == 0 {
		t.Error("expected insights lines to be non-empty")
	}
	if parsed.Insights.StaleIssues != 1 {
		t.Errorf("insights.stale_issues = %d, want 1", parsed.Insights.StaleIssues)
	}
	if parsed.Insights.PRsNeedingReview != 1 {
		t.Errorf("insights.prs_needing_review = %d, want 1", parsed.Insights.PRsNeedingReview)
	}
	if parsed.Summary.IssuesClosed != 2 {
		t.Errorf("summary.issues_closed = %d, want 2", parsed.Summary.IssuesClosed)
	}
	if parsed.Summary.IssuesOpen != 2 {
		t.Errorf("summary.issues_open = %d, want 2", parsed.Summary.IssuesOpen)
	}
	if parsed.Summary.PRsOpen != 2 {
		t.Errorf("summary.prs_open = %d, want 2", parsed.Summary.PRsOpen)
	}
	if len(parsed.Lookback.IssuesClosed) != 2 {
		t.Errorf("lookback.issues_closed length = %d, want 2", len(parsed.Lookback.IssuesClosed))
	}

	// Check annotations on ahead items.
	if len(parsed.Ahead.IssuesOpen) != 2 {
		t.Fatalf("ahead.issues_open length = %d, want 2", len(parsed.Ahead.IssuesOpen))
	}
	if parsed.Ahead.IssuesOpen[0].Status != "new" {
		t.Errorf("issue #42 status = %q, want 'new'", parsed.Ahead.IssuesOpen[0].Status)
	}
	if parsed.Ahead.IssuesOpen[1].Status != "stale" {
		t.Errorf("issue #30 status = %q, want 'stale'", parsed.Ahead.IssuesOpen[1].Status)
	}

	if len(parsed.Ahead.PRsOpen) != 2 {
		t.Fatalf("ahead.prs_open length = %d, want 2", len(parsed.Ahead.PRsOpen))
	}
	if parsed.Ahead.PRsOpen[0].Status != "needs_review" {
		t.Errorf("PR #43 status = %q, want 'needs_review'", parsed.Ahead.PRsOpen[0].Status)
	}
	// PR #44: created before since, has reviews — should be "active"
	if parsed.Ahead.PRsOpen[1].Status != "active" {
		t.Errorf("PR #44 status = %q, want 'active'", parsed.Ahead.PRsOpen[1].Status)
	}
}

// Status logic tests are in internal/model/status_test.go.
// Formatter tests above verify the rendering of status annotations.

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
