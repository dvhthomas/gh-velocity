package format

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func testMyWeekResult() model.MyWeekResult {
	return model.MyWeekResult{
		Login: "testuser",
		Repo:  "owner/repo",
		Since: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
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
			{Number: 42, Title: "Implement caching", URL: "https://github.com/owner/repo/issues/42"},
		},
		PRsOpen: []model.PR{
			{Number: 43, Title: "WIP: add my-week command", URL: "https://github.com/owner/repo/pull/43"},
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
		"What I shipped",
		"Issues Closed: 2", "PRs Merged: 1", "PRs Reviewed: 1",
		"#1", "#10", "#20",
		"What's ahead",
		"Open Issues: 1", "#42", "Implement caching",
		"Open PRs: 1", "#43", "WIP: add my-week",
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
		Since: time.Now(),
		Until: time.Now(),
	}
	if err := WriteMyWeekPretty(rc, r); err != nil {
		t.Fatal(err)
	}
	if !contains(buf.String(), "No activity") {
		t.Error("expected 'No activity' for empty result")
	}
	// Should NOT show section headers when everything is empty.
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
		"### What I shipped",
		"**Issues Closed (2)**", "**PRs Merged (1)**", "**PRs Reviewed (1)**",
		"[#1]", "[#10]",
		"### What's ahead",
		"**Open Issues (1)**", "[#42]",
		"**Open PRs (1)**", "[#43]",
	} {
		if !contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestWriteMyWeekMarkdown_Empty(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Markdown}
	r := model.MyWeekResult{Login: "u", Repo: "o/r", Since: time.Now(), Until: time.Now()}
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
	if parsed.Summary.IssuesClosed != 2 {
		t.Errorf("summary.issues_closed = %d, want 2", parsed.Summary.IssuesClosed)
	}
	if parsed.Summary.PRsMerged != 1 {
		t.Errorf("summary.prs_merged = %d, want 1", parsed.Summary.PRsMerged)
	}
	if parsed.Summary.IssuesOpen != 1 {
		t.Errorf("summary.issues_open = %d, want 1", parsed.Summary.IssuesOpen)
	}
	if parsed.Summary.PRsOpen != 1 {
		t.Errorf("summary.prs_open = %d, want 1", parsed.Summary.PRsOpen)
	}
	if len(parsed.Lookback.IssuesClosed) != 2 {
		t.Errorf("lookback.issues_closed length = %d, want 2", len(parsed.Lookback.IssuesClosed))
	}
	if len(parsed.Ahead.IssuesOpen) != 1 {
		t.Errorf("ahead.issues_open length = %d, want 1", len(parsed.Ahead.IssuesOpen))
	}
	if len(parsed.Ahead.PRsOpen) != 1 {
		t.Errorf("ahead.prs_open length = %d, want 1", len(parsed.Ahead.PRsOpen))
	}
}

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
