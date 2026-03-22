package throughput

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// mockSearcher is a test double for the searcher interface.
type mockSearcher struct {
	issueResults map[string][]model.Issue // query -> results
	prResults    map[string][]model.PR    // query -> results
	issueErrors  map[string]error         // query -> error
	prErrors     map[string]error         // query -> error
}

func (m *mockSearcher) SearchIssues(_ context.Context, query string) ([]model.Issue, error) {
	if err, ok := m.issueErrors[query]; ok {
		return nil, err
	}
	return m.issueResults[query], nil
}

func (m *mockSearcher) SearchPRs(_ context.Context, query string) ([]model.PR, error) {
	if err, ok := m.prErrors[query]; ok {
		return nil, err
	}
	return m.prResults[query], nil
}

func TestGatherData_RetainsItems(t *testing.T) {
	issues := []model.Issue{
		{Number: 1, Title: "issue one"},
		{Number: 2, Title: "issue two"},
		{Number: 3, Title: "issue three"},
	}
	prs := []model.PR{
		{Number: 10, Title: "pr one"},
		{Number: 11, Title: "pr two"},
	}

	mock := &mockSearcher{
		issueResults: map[string][]model.Issue{"closed-issues": issues},
		prResults:    map[string][]model.PR{"merged-prs": prs},
	}

	p := &Pipeline{
		Client:     mock,
		Owner:      "org",
		Repo:       "repo",
		IssueQuery: "closed-issues",
		PRQuery:    "merged-prs",
	}

	if err := p.GatherData(context.Background()); err != nil {
		t.Fatalf("GatherData() error: %v", err)
	}

	if len(p.ClosedIssues) != 3 {
		t.Errorf("ClosedIssues count = %d, want 3", len(p.ClosedIssues))
	}
	if len(p.MergedPRs) != 2 {
		t.Errorf("MergedPRs count = %d, want 2", len(p.MergedPRs))
	}

	// Verify actual items are retained, not just counts
	if p.ClosedIssues[0].Title != "issue one" {
		t.Errorf("ClosedIssues[0].Title = %q, want %q", p.ClosedIssues[0].Title, "issue one")
	}
	if p.MergedPRs[1].Title != "pr two" {
		t.Errorf("MergedPRs[1].Title = %q, want %q", p.MergedPRs[1].Title, "pr two")
	}
}

func TestGatherData_OpenItemsExecutedAndDeduplicated(t *testing.T) {
	// Issue #5 appears in both queries — should be deduplicated
	mock := &mockSearcher{
		issueResults: map[string][]model.Issue{
			"closed-q":   {},
			"open-q1":    {{Number: 1, Title: "a"}, {Number: 5, Title: "dup"}},
			"open-q2":    {{Number: 5, Title: "dup"}, {Number: 9, Title: "b"}},
		},
		prResults: map[string][]model.PR{
			"merged-q":     {},
			"open-pr-q1":   {{Number: 20, Title: "pr-a"}, {Number: 25, Title: "pr-dup"}},
			"open-pr-q2":   {{Number: 25, Title: "pr-dup"}, {Number: 30, Title: "pr-b"}},
		},
	}

	p := &Pipeline{
		Client:           mock,
		Owner:            "org",
		Repo:             "repo",
		IssueQuery:       "closed-q",
		PRQuery:          "merged-q",
		OpenIssueQueries: []string{"open-q1", "open-q2"},
		OpenPRQueries:    []string{"open-pr-q1", "open-pr-q2"},
	}

	if err := p.GatherData(context.Background()); err != nil {
		t.Fatalf("GatherData() error: %v", err)
	}

	// 3 unique issues (1, 5, 9) — #5 deduplicated
	if len(p.OpenIssues) != 3 {
		t.Errorf("OpenIssues count = %d, want 3", len(p.OpenIssues))
	}
	// 3 unique PRs (20, 25, 30) — #25 deduplicated
	if len(p.OpenPRs) != 3 {
		t.Errorf("OpenPRs count = %d, want 3", len(p.OpenPRs))
	}

	// Verify deduplication kept the first occurrence
	seen := make(map[int]bool)
	for _, issue := range p.OpenIssues {
		if seen[issue.Number] {
			t.Errorf("duplicate issue %d in OpenIssues", issue.Number)
		}
		seen[issue.Number] = true
	}
	seenPR := make(map[int]bool)
	for _, pr := range p.OpenPRs {
		if seenPR[pr.Number] {
			t.Errorf("duplicate PR %d in OpenPRs", pr.Number)
		}
		seenPR[pr.Number] = true
	}
}

func TestGatherData_PartialFailure_OpenFetchWarning(t *testing.T) {
	mock := &mockSearcher{
		issueResults: map[string][]model.Issue{
			"closed-q": {{Number: 1}},
		},
		prResults: map[string][]model.PR{
			"merged-q": {{Number: 10}},
		},
		issueErrors: map[string]error{
			"open-fail": fmt.Errorf("API error"),
		},
		prErrors: map[string]error{
			"open-pr-fail": fmt.Errorf("PR API error"),
		},
	}

	p := &Pipeline{
		Client:           mock,
		Owner:            "org",
		Repo:             "repo",
		IssueQuery:       "closed-q",
		PRQuery:          "merged-q",
		OpenIssueQueries: []string{"open-fail"},
		OpenPRQueries:    []string{"open-pr-fail"},
	}

	err := p.GatherData(context.Background())
	if err != nil {
		t.Fatalf("GatherData() should not fail on open-item errors, got: %v", err)
	}

	// Closed items should still be retained
	if len(p.ClosedIssues) != 1 {
		t.Errorf("ClosedIssues = %d, want 1", len(p.ClosedIssues))
	}
	if len(p.MergedPRs) != 1 {
		t.Errorf("MergedPRs = %d, want 1", len(p.MergedPRs))
	}

	// Warnings should be present for failed open fetches
	if len(p.Warnings()) < 2 {
		t.Fatalf("expected at least 2 warnings, got %d: %v", len(p.Warnings()), p.Warnings())
	}
	foundIssueWarn, foundPRWarn := false, false
	for _, w := range p.Warnings() {
		if containsStr(w, "open issue search failed") {
			foundIssueWarn = true
		}
		if containsStr(w, "open PR search failed") {
			foundPRWarn = true
		}
	}
	if !foundIssueWarn {
		t.Error("missing warning for open issue search failure")
	}
	if !foundPRWarn {
		t.Error("missing warning for open PR search failure")
	}
}

func TestGatherData_TruncationWarning(t *testing.T) {
	// Build exactly 1000 issues to trigger the warning
	bigIssueSet := make([]model.Issue, 1000)
	for i := range bigIssueSet {
		bigIssueSet[i] = model.Issue{Number: i + 1}
	}
	bigPRSet := make([]model.PR, 1000)
	for i := range bigPRSet {
		bigPRSet[i] = model.PR{Number: i + 1}
	}

	mock := &mockSearcher{
		issueResults: map[string][]model.Issue{
			"closed-q":  {},
			"open-big":  bigIssueSet,
		},
		prResults: map[string][]model.PR{
			"merged-q":    {},
			"open-pr-big": bigPRSet,
		},
	}

	p := &Pipeline{
		Client:           mock,
		Owner:            "org",
		Repo:             "repo",
		IssueQuery:       "closed-q",
		PRQuery:          "merged-q",
		OpenIssueQueries: []string{"open-big"},
		OpenPRQueries:    []string{"open-pr-big"},
	}

	if err := p.GatherData(context.Background()); err != nil {
		t.Fatalf("GatherData() error: %v", err)
	}

	truncationWarnings := 0
	for _, w := range p.Warnings() {
		if containsStr(w, "truncated (1000 results)") {
			truncationWarnings++
		}
	}
	if truncationWarnings != 2 {
		t.Errorf("expected 2 truncation warnings (issues + PRs), got %d: %v", truncationWarnings, p.Warnings())
	}
}

func TestGatherData_NoOpenQueries_NoOpenItems(t *testing.T) {
	mock := &mockSearcher{
		issueResults: map[string][]model.Issue{
			"closed-q": {{Number: 1}},
		},
		prResults: map[string][]model.PR{
			"merged-q": {{Number: 10}},
		},
	}

	p := &Pipeline{
		Client:     mock,
		Owner:      "org",
		Repo:       "repo",
		IssueQuery: "closed-q",
		PRQuery:    "merged-q",
		// No OpenIssueQueries or OpenPRQueries
	}

	if err := p.GatherData(context.Background()); err != nil {
		t.Fatalf("GatherData() error: %v", err)
	}

	if len(p.OpenIssues) != 0 {
		t.Errorf("OpenIssues = %d, want 0 when no queries configured", len(p.OpenIssues))
	}
	if len(p.OpenPRs) != 0 {
		t.Errorf("OpenPRs = %d, want 0 when no queries configured", len(p.OpenPRs))
	}
}

func TestProcessData(t *testing.T) {
	p := &Pipeline{
		Owner:        "org",
		Repo:         "repo",
		Since:        time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Until:        time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
		ClosedIssues: make([]model.Issue, 5),
		MergedPRs:    make([]model.PR, 3),
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData() error: %v", err)
	}

	if p.Result.Repository != "org/repo" {
		t.Errorf("Repository = %q, want org/repo", p.Result.Repository)
	}
	if p.Result.IssuesClosed != 5 {
		t.Errorf("IssuesClosed = %d, want 5", p.Result.IssuesClosed)
	}
	if p.Result.PRsMerged != 3 {
		t.Errorf("PRsMerged = %d, want 3", p.Result.PRsMerged)
	}
}

func TestProcessData_ZeroCounts(t *testing.T) {
	p := &Pipeline{
		Owner: "org",
		Repo:  "repo",
		Since: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData() error: %v", err)
	}

	if p.Result.IssuesClosed != 0 {
		t.Errorf("IssuesClosed = %d, want 0", p.Result.IssuesClosed)
	}
	if p.Result.PRsMerged != 0 {
		t.Errorf("PRsMerged = %d, want 0", p.Result.PRsMerged)
	}
}

func TestRender_JSON(t *testing.T) {
	p := &Pipeline{
		SearchURL: "https://github.com/search?q=test",
		Result: model.ThroughputResult{
			Repository:   "org/repo",
			Since:        time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Until:        time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
			IssuesClosed: 5,
			PRsMerged:    3,
		},
	}

	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.JSON}
	if err := p.Render(rc); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	var parsed jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	if parsed.Issues != 5 {
		t.Errorf("issues_closed = %d, want 5", parsed.Issues)
	}
	if parsed.PRs != 3 {
		t.Errorf("prs_merged = %d, want 3", parsed.PRs)
	}
	if parsed.Total != 8 {
		t.Errorf("total = %d, want 8", parsed.Total)
	}
	if parsed.SearchURL != "https://github.com/search?q=test" {
		t.Errorf("search_url = %q, want test URL", parsed.SearchURL)
	}
}

func TestRender_Pretty(t *testing.T) {
	p := &Pipeline{
		Result: model.ThroughputResult{
			Repository:   "org/repo",
			Since:        time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Until:        time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
			IssuesClosed: 5,
			PRsMerged:    3,
		},
	}

	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.Pretty}
	if err := p.Render(rc); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"org/repo", "Issues closed: 5", "PRs merged:    3", "Total:         8"} {
		if !containsStr(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestRender_Pretty_EmptyWithVerifyURL(t *testing.T) {
	p := &Pipeline{
		SearchURL: "https://github.com/search?q=test",
		Result: model.ThroughputResult{
			Repository: "org/repo",
			Since:      time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Until:      time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.Pretty}
	if err := p.Render(rc); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	out := buf.String()
	if !containsStr(out, "No activity") {
		t.Error("expected 'No activity' for empty result")
	}
	if !containsStr(out, "Verify: https://github.com/search?q=test") {
		t.Error("expected verify URL for empty result")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
