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
	}
}

func TestWriteMyWeekPretty(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Pretty}
	if err := WriteMyWeekPretty(rc, testMyWeekResult()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"testuser", "Issues Closed: 2", "PRs Merged: 1", "PRs Reviewed: 1", "#1", "#10", "#20"} {
		if !contains(out, want) {
			t.Errorf("expected %q in output", want)
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
}

func TestWriteMyWeekMarkdown(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Markdown}
	if err := WriteMyWeekMarkdown(rc, testMyWeekResult()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"## My Week", "### Issues Closed (2)", "### PRs Merged (1)", "### PRs Reviewed (1)", "[#1]", "[#10]"} {
		if !contains(out, want) {
			t.Errorf("expected %q in output", want)
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
	if len(parsed.IssuesClosed) != 2 {
		t.Errorf("issues_closed length = %d, want 2", len(parsed.IssuesClosed))
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
