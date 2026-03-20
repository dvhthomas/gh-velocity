package format

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/metrics"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

var (
	testNow   = time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	testSince = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
)

// testMyWeekResult returns a comprehensive mock dataset that exercises
// every feature: lead time, cycle time, releases, stale issues, review
// pressure, new scope, and status annotations.
func testMyWeekResult() model.MyWeekResult {
	// Issue closed dates for lead time calculation:
	//   #1: created Feb 20, closed Mar 3 → 11 days lead time
	//   #5: created Mar 1, closed Mar 5  → 4 days lead time
	//   #8: created Feb 25, closed Mar 7 → 10 days lead time
	//   Median lead time: 10 days
	closedMar3 := time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC)
	closedMar5 := time.Date(2026, 3, 5, 14, 0, 0, 0, time.UTC)
	closedMar7 := time.Date(2026, 3, 7, 9, 0, 0, 0, time.UTC)

	// PR merged dates for cycle time calculation:
	//   #10: created Feb 28, merged Mar 2 → 2 days
	//   #11: created Mar 1, merged Mar 6  → 5 days
	//   #12: created Feb 26, merged Mar 4 → 6 days
	//   Median cycle time: 5 days
	mergedMar2 := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	mergedMar6 := time.Date(2026, 3, 6, 16, 0, 0, 0, time.UTC)
	mergedMar4 := time.Date(2026, 3, 4, 11, 0, 0, 0, time.UTC)

	// Release published date
	publishedMar5 := time.Date(2026, 3, 5, 18, 0, 0, 0, time.UTC)

	return model.MyWeekResult{
		Login: "testuser",
		Repo:  "owner/repo",
		Since: testSince,
		Until: testNow,

		// Lookback: closed issues with dates for lead time
		IssuesClosed: []model.Issue{
			{Number: 1, Title: "Fix crash", URL: "https://github.com/owner/repo/issues/1",
				CreatedAt: time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC), ClosedAt: &closedMar3},
			{Number: 5, Title: "Update docs", URL: "https://github.com/owner/repo/issues/5",
				CreatedAt: testSince, ClosedAt: &closedMar5},
			{Number: 8, Title: "Add validation", URL: "https://github.com/owner/repo/issues/8",
				CreatedAt: time.Date(2026, 2, 25, 0, 0, 0, 0, time.UTC), ClosedAt: &closedMar7},
		},

		// Lookback: merged PRs with dates for cycle time
		PRsMerged: []model.PR{
			{Number: 10, Title: "Add feature", URL: "https://github.com/owner/repo/pull/10",
				Author: "testuser", CreatedAt: time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), MergedAt: &mergedMar2},
			{Number: 11, Title: "Refactor models", URL: "https://github.com/owner/repo/pull/11",
				Author: "testuser", CreatedAt: testSince, MergedAt: &mergedMar6},
			{Number: 12, Title: "Fix edge case", URL: "https://github.com/owner/repo/pull/12",
				Author: "testuser", CreatedAt: time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC), MergedAt: &mergedMar4},
		},

		// Lookback: reviewed PRs
		PRsReviewed: []model.PR{
			{Number: 20, Title: "Refactor auth", URL: "https://github.com/owner/repo/pull/20",
				Author: "colleague1"},
			{Number: 21, Title: "Add logging", URL: "https://github.com/owner/repo/pull/21",
				Author: "colleague2"},
		},

		// Lookback: releases
		Releases: []model.Release{
			{TagName: "v1.2.0", Name: "v1.2.0 — March release",
				URL:       "https://github.com/owner/repo/releases/tag/v1.2.0",
				CreatedAt: publishedMar5, PublishedAt: &publishedMar5},
		},

		// Lookahead: open issues
		IssuesOpen: []model.Issue{
			// New: created within --since window
			{Number: 42, Title: "Implement caching", URL: "https://github.com/owner/repo/issues/42",
				CreatedAt: testSince.Add(2 * 24 * time.Hour), UpdatedAt: testSince.Add(2 * 24 * time.Hour)},
			// Stale: created 20 days ago, no update in 15 days
			{Number: 30, Title: "Old backlog item", URL: "https://github.com/owner/repo/issues/30",
				CreatedAt: testNow.Add(-20 * 24 * time.Hour), UpdatedAt: testNow.Add(-15 * 24 * time.Hour)},
			// Active: created before window, recently updated
			{Number: 35, Title: "Improve perf", URL: "https://github.com/owner/repo/issues/35",
				CreatedAt: testNow.Add(-14 * 24 * time.Hour), UpdatedAt: testNow.Add(-2 * 24 * time.Hour)},
		},

		// Lookahead: open PRs
		PRsOpen: []model.PR{
			// Needs review: created before --since, 10 days old, in PRsNeedingReview
			{Number: 43, Title: "WIP: add my-week", URL: "https://github.com/owner/repo/pull/43",
				Author: "testuser", CreatedAt: testSince.Add(-3 * 24 * time.Hour)},
			// Normal active PR
			{Number: 44, Title: "Fix typo", URL: "https://github.com/owner/repo/pull/44",
				Author: "testuser", CreatedAt: testSince.Add(-1 * 24 * time.Hour)},
		},
		PRsNeedingReview: []model.PR{
			{Number: 43, Title: "WIP: add my-week", URL: "https://github.com/owner/repo/pull/43",
				Author: "testuser", CreatedAt: testSince.Add(-3 * 24 * time.Hour)},
		},

		// Review pressure: PRs from others waiting on me
		PRsAwaitingMyReview: []model.PR{
			{Number: 50, Title: "Add dark mode", URL: "https://github.com/owner/repo/pull/50",
				Author: "alice", CreatedAt: testSince.Add(1 * 24 * time.Hour)},
			{Number: 51, Title: "Update deps", URL: "https://github.com/owner/repo/pull/51",
				Author: "bob", CreatedAt: testNow.Add(-10 * 24 * time.Hour)},
		},
	}
}

// testCycleTimeDurations returns PR-based cycle-time durations for test data.
func testCycleTimeDurations(r model.MyWeekResult) []time.Duration {
	var durations []time.Duration
	for _, pr := range r.PRsMerged {
		if pr.MergedAt != nil {
			d := pr.MergedAt.Sub(pr.CreatedAt)
			if d > 0 {
				durations = append(durations, d)
			}
		}
	}
	return durations
}

func TestWriteMyWeekPretty(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Pretty}
	r := testMyWeekResult()
	if err := WriteMyWeekPretty(rc, r, metrics.ComputeInsights(r, testCycleTimeDurations(r)), MyWeekSearchURLs{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"testuser",
		// Insights
		"Insights",
		"Shipped 6 items", "3 issues closed, 3 PRs merged", "in 7 days",
		"Reviewed 2 PRs",
		"1 release(s) published",
		"Median lead time:",
		"Median cycle time:",
		"WAITING:", "2 PR(s) awaiting your review", "1 of your PR(s) waiting for first review", "1 stale issue(s)",
		"New work picked up: 1 issue(s)",
		// Lookback
		"What I shipped",
		"Issues Closed: 3", "PRs Merged: 3", "PRs Reviewed: 2",
		"#1", "#10", "#20",
		"Releases: 1", "v1.2.0",
		// Lookahead
		"What's ahead",
		"Open Issues: 3", "#42", "Implement caching",
		"Open PRs: 2", "#43", "WIP: add my-week",
		"<- new",          // #42 should be annotated as new
		"<- needs review", // #43 should be annotated as needs review
		"<- stale",        // #30 should be annotated as stale
		// Review queue
		"Review queue",
		"Awaiting Your Review: 2",
		"@alice", "@bob", "Add dark mode", "Update deps",
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
	if err := WriteMyWeekPretty(rc, r, metrics.ComputeInsights(r, nil), MyWeekSearchURLs{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !contains(out, "No activity") {
		t.Error("expected 'No activity' for empty result")
	}
	// Lookback sections always show with 0 counts.
	if !contains(out, "Issues Closed: 0") {
		t.Error("expected 'Issues Closed: 0' for empty result")
	}
	if !contains(out, "PRs Merged: 0") {
		t.Error("expected 'PRs Merged: 0' for empty result")
	}
	if !contains(out, "PRs Reviewed: 0") {
		t.Error("expected 'PRs Reviewed: 0' for empty result")
	}
}

func TestWriteMyWeekPretty_EmptyWithVerifyURLs(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Pretty}
	r := model.MyWeekResult{
		Login: "testuser",
		Repo:  "owner/repo",
		Since: testSince,
		Until: testNow,
	}
	urls := MyWeekSearchURLs{
		IssuesClosed: "https://github.com/search?q=issues",
		PRsMerged:    "https://github.com/search?q=prs",
		PRsReviewed:  "https://github.com/search?q=reviews",
	}
	if err := WriteMyWeekPretty(rc, r, metrics.ComputeInsights(r, nil), urls); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !contains(out, "Verify: https://github.com/search?q=issues") {
		t.Error("expected verify URL for issues closed")
	}
	if !contains(out, "Verify: https://github.com/search?q=prs") {
		t.Error("expected verify URL for PRs merged")
	}
	if !contains(out, "Verify: https://github.com/search?q=reviews") {
		t.Error("expected verify URL for PRs reviewed")
	}
}

func TestWriteMyWeekMarkdown(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Markdown}
	r := testMyWeekResult()
	if err := WriteMyWeekMarkdown(rc, r, metrics.ComputeInsights(r, testCycleTimeDurations(r)), MyWeekSearchURLs{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"## My Week",
		// Insights
		"### Insights",
		"Shipped 6 items",
		"Median lead time:",
		"Median cycle time:",
		"WAITING:", "awaiting your review", "waiting for first review", "stale issue",
		"release(s) published",
		// Lookback
		"### What I shipped",
		"**Issues Closed (3)**", "**PRs Merged (3)**", "**PRs Reviewed (2)**",
		"[#1]", "[#10]",
		"**Releases (1)**", "v1.2.0",
		// Lookahead
		"### What's ahead",
		"**Open Issues (3)**", "[#42]",
		"**Open PRs (2)**", "[#43]",
		"`new ",          // #42 annotation
		"`needs review ", // #43 annotation
		"`stale ",        // #30 annotation
		// Review queue
		"### Review queue",
		"**Awaiting Your Review (2)**",
		"@alice", "@bob",
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
	if err := WriteMyWeekMarkdown(rc, r, metrics.ComputeInsights(r, nil), MyWeekSearchURLs{}); err != nil {
		t.Fatal(err)
	}
	if !contains(buf.String(), "_None_") {
		t.Error("expected '_None_' for empty sections")
	}
}

func TestWriteMyWeekJSON(t *testing.T) {
	var buf bytes.Buffer
	r := testMyWeekResult()
	if err := WriteMyWeekJSON(&buf, r, metrics.ComputeInsights(r, testCycleTimeDurations(r)), MyWeekSearchURLs{}, nil); err != nil {
		t.Fatal(err)
	}

	var parsed jsonMyWeekResult
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Login != "testuser" {
		t.Errorf("login = %q, want testuser", parsed.Login)
	}

	// Insights
	if len(parsed.Insights.Lines) == 0 {
		t.Error("expected insights lines to be non-empty")
	}
	if parsed.Insights.StaleIssues != 1 {
		t.Errorf("insights.stale_issues = %d, want 1", parsed.Insights.StaleIssues)
	}
	if parsed.Insights.PRsNeedingReview != 1 {
		t.Errorf("insights.prs_needing_review = %d, want 1", parsed.Insights.PRsNeedingReview)
	}
	if parsed.Insights.PRsAwaitingMyReview != 2 {
		t.Errorf("insights.prs_awaiting_my_review = %d, want 2", parsed.Insights.PRsAwaitingMyReview)
	}
	if parsed.Insights.Releases != 1 {
		t.Errorf("insights.releases = %d, want 1", parsed.Insights.Releases)
	}
	if parsed.Insights.LeadTimeHours == nil {
		t.Error("expected lead_time_hours to be set")
	}
	if parsed.Insights.CycleTimeHours == nil {
		t.Error("expected cycle_time_hours to be set")
	}

	// Summary
	if parsed.Summary.IssuesClosed != 3 {
		t.Errorf("summary.issues_closed = %d, want 3", parsed.Summary.IssuesClosed)
	}
	if parsed.Summary.PRsMerged != 3 {
		t.Errorf("summary.prs_merged = %d, want 3", parsed.Summary.PRsMerged)
	}
	if parsed.Summary.IssuesOpen != 3 {
		t.Errorf("summary.issues_open = %d, want 3", parsed.Summary.IssuesOpen)
	}
	if parsed.Summary.PRsOpen != 2 {
		t.Errorf("summary.prs_open = %d, want 2", parsed.Summary.PRsOpen)
	}

	// Lookback
	if len(parsed.Lookback.IssuesClosed) != 3 {
		t.Errorf("lookback.issues_closed length = %d, want 3", len(parsed.Lookback.IssuesClosed))
	}
	if len(parsed.Lookback.Releases) != 1 {
		t.Errorf("lookback.releases length = %d, want 1", len(parsed.Lookback.Releases))
	}
	if parsed.Lookback.Releases[0].Tag != "v1.2.0" {
		t.Errorf("lookback.releases[0].tag = %q, want 'v1.2.0'", parsed.Lookback.Releases[0].Tag)
	}

	// Ahead: issue statuses
	if len(parsed.Ahead.IssuesOpen) != 3 {
		t.Fatalf("ahead.issues_open length = %d, want 3", len(parsed.Ahead.IssuesOpen))
	}
	if parsed.Ahead.IssuesOpen[0].Status != "new" {
		t.Errorf("issue #42 status = %q, want 'new'", parsed.Ahead.IssuesOpen[0].Status)
	}
	if parsed.Ahead.IssuesOpen[1].Status != "stale" {
		t.Errorf("issue #30 status = %q, want 'stale'", parsed.Ahead.IssuesOpen[1].Status)
	}
	if parsed.Ahead.IssuesOpen[2].Status != "active" {
		t.Errorf("issue #35 status = %q, want 'active'", parsed.Ahead.IssuesOpen[2].Status)
	}

	// Ahead: PR statuses
	if len(parsed.Ahead.PRsOpen) != 2 {
		t.Fatalf("ahead.prs_open length = %d, want 2", len(parsed.Ahead.PRsOpen))
	}
	if parsed.Ahead.PRsOpen[0].Status != "needs_review" {
		t.Errorf("PR #43 status = %q, want 'needs_review'", parsed.Ahead.PRsOpen[0].Status)
	}
	if parsed.Ahead.PRsOpen[1].Status != "active" {
		t.Errorf("PR #44 status = %q, want 'active'", parsed.Ahead.PRsOpen[1].Status)
	}

	// Ahead: review queue
	if len(parsed.Ahead.PRsAwaitingMyReview) != 2 {
		t.Fatalf("ahead.prs_awaiting_my_review length = %d, want 2", len(parsed.Ahead.PRsAwaitingMyReview))
	}
	if parsed.Ahead.PRsAwaitingMyReview[0].Author != "alice" {
		t.Errorf("review[0].author = %q, want 'alice'", parsed.Ahead.PRsAwaitingMyReview[0].Author)
	}
	if parsed.Ahead.PRsAwaitingMyReview[1].Author != "bob" {
		t.Errorf("review[1].author = %q, want 'bob'", parsed.Ahead.PRsAwaitingMyReview[1].Author)
	}
}

func TestWriteMyWeekPretty_CycleTimeHintWhenNA(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Pretty}
	closedAt := testNow.Add(-24 * time.Hour)
	r := model.MyWeekResult{
		Login: "testuser",
		Repo:  "owner/repo",
		Since: testSince,
		Until: testNow,
		IssuesClosed: []model.Issue{
			{Number: 1, Title: "Fix bug", CreatedAt: testSince, ClosedAt: &closedAt},
		},
	}
	// Pass nil cycle time durations — simulates no strategy signal.
	ins := metrics.ComputeInsights(r, nil)
	if err := WriteMyWeekPretty(rc, r, ins, MyWeekSearchURLs{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !contains(out, "Cycle time not available") {
		t.Error("expected cycle time hint when closed issues exist but no cycle time data")
	}
	if !contains(out, "preflight") {
		t.Error("expected preflight guidance in hint")
	}
}

func TestWriteMyWeekPretty_CrossRepo(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Pretty}
	r := model.MyWeekResult{
		Login: "testuser",
		Repo:  "", // cross-repo mode
		Since: testSince,
		Until: testNow,
	}
	if err := WriteMyWeekPretty(rc, r, metrics.ComputeInsights(r, nil), MyWeekSearchURLs{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !contains(out, "all repositories") {
		t.Error("expected 'all repositories' in header when Repo is empty")
	}
	if contains(out, "( )") || contains(out, "()") {
		t.Error("header should not have empty parens")
	}
}

func TestWriteMyWeekMarkdown_CrossRepo(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Markdown}
	r := model.MyWeekResult{
		Login: "testuser",
		Repo:  "", // cross-repo mode
		Since: testSince,
		Until: testNow,
	}
	if err := WriteMyWeekMarkdown(rc, r, metrics.ComputeInsights(r, nil), MyWeekSearchURLs{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !contains(out, "all repositories") {
		t.Errorf("expected 'all repositories' in markdown when Repo is empty, got:\n%s", out)
	}
}

func TestWriteMyWeekJSON_CrossRepo(t *testing.T) {
	var buf bytes.Buffer
	r := model.MyWeekResult{
		Login: "testuser",
		Repo:  "", // cross-repo mode
		Since: testSince,
		Until: testNow,
	}
	if err := WriteMyWeekJSON(&buf, r, metrics.ComputeInsights(r, nil), MyWeekSearchURLs{}, nil); err != nil {
		t.Fatal(err)
	}

	// Parse as raw JSON to check repo is null.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	repoVal := string(raw["repo"])
	if repoVal != "null" {
		t.Errorf("expected repo to be null for cross-repo, got %s", repoVal)
	}
}

func TestWriteMyWeekJSON_SingleRepo(t *testing.T) {
	var buf bytes.Buffer
	r := model.MyWeekResult{
		Login: "testuser",
		Repo:  "owner/repo",
		Since: testSince,
		Until: testNow,
	}
	if err := WriteMyWeekJSON(&buf, r, metrics.ComputeInsights(r, nil), MyWeekSearchURLs{}, nil); err != nil {
		t.Fatal(err)
	}

	// Parse as raw JSON to check repo is a string.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	repoVal := string(raw["repo"])
	if repoVal != `"owner/repo"` {
		t.Errorf("expected repo to be %q, got %s", "owner/repo", repoVal)
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
