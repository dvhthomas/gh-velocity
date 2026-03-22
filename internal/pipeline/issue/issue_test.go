package issue

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/classify"
	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestProcessData_ClosedIssueWithPR(t *testing.T) {
	closed := time.Date(2026, 3, 18, 18, 56, 0, 0, time.UTC)
	merged := time.Date(2026, 3, 18, 18, 55, 0, 0, time.UTC)

	classifier, err := classify.NewClassifier([]model.CategoryConfig{
		{Name: "feature", Matchers: []string{"label:enhancement"}},
		{Name: "bug", Matchers: []string{"label:bug"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	p := &Pipeline{
		IssueNumber: 119,
		Issue: &model.Issue{
			Number:    119,
			Title:     "feat(preflight): auto-detect noise labels",
			State:     "closed",
			Labels:    []string{"enhancement"},
			CreatedAt: time.Date(2026, 3, 18, 18, 55, 0, 0, time.UTC),
			ClosedAt:  &closed,
			URL:       "https://github.com/dvhthomas/gh-velocity/issues/119",
		},
		ClosingPRs: []*model.PR{
			{
				Number:    120,
				Title:     "docs: noise exclusion guide",
				CreatedAt: time.Date(2026, 3, 18, 18, 41, 0, 0, time.UTC),
				MergedAt:  &merged,
				URL:       "https://github.com/dvhthomas/gh-velocity/pull/120",
			},
		},
		Classifier: classifier,
		// No strategy — cycle time will be N/A
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	// Lead time should be 1 minute
	if p.LeadTime.Duration == nil {
		t.Fatal("expected lead time duration, got nil")
	}
	if *p.LeadTime.Duration != time.Minute {
		t.Errorf("lead time: got %v, want 1m", *p.LeadTime.Duration)
	}

	// Category should be feature
	if p.Category != "feature" {
		t.Errorf("category: got %q, want %q", p.Category, "feature")
	}

	// Cycle time should be N/A (no strategy)
	if p.CycleTime.Duration != nil {
		t.Errorf("expected nil cycle time duration, got %v", *p.CycleTime.Duration)
	}

	// Linked PR should have cycle time
	if len(p.LinkedPRs) != 1 {
		t.Fatalf("linked PRs: got %d, want 1", len(p.LinkedPRs))
	}
	if p.LinkedPRs[0].CycleTime.Duration == nil {
		t.Fatal("expected linked PR cycle time, got nil")
	}
	if *p.LinkedPRs[0].CycleTime.Duration != 14*time.Minute {
		t.Errorf("linked PR cycle time: got %v, want 14m", *p.LinkedPRs[0].CycleTime.Duration)
	}
}

func TestProcessData_OpenIssue(t *testing.T) {
	p := &Pipeline{
		IssueNumber: 42,
		Issue: &model.Issue{
			Number:    42,
			Title:     "open issue",
			State:     "open",
			CreatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	// Lead time should be nil (open issue)
	if p.LeadTime.Duration != nil {
		t.Errorf("expected nil lead time for open issue, got %v", *p.LeadTime.Duration)
	}

	// Category defaults to "other"
	if p.Category != "other" {
		t.Errorf("category: got %q, want %q", p.Category, "other")
	}
}

func TestProcessData_NoClosingPRs(t *testing.T) {
	closed := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)

	p := &Pipeline{
		IssueNumber: 99,
		Issue: &model.Issue{
			Number:    99,
			Title:     "no linked PR",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC),
			ClosedAt:  &closed,
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	if len(p.LinkedPRs) != 0 {
		t.Errorf("expected no linked PRs, got %d", len(p.LinkedPRs))
	}
}

func TestRenderMarkdown(t *testing.T) {
	closed := time.Date(2026, 3, 18, 18, 56, 0, 0, time.UTC)
	merged := time.Date(2026, 3, 18, 18, 55, 0, 0, time.UTC)

	p := &Pipeline{
		IssueNumber: 119,
		Issue: &model.Issue{
			Number:    119,
			Title:     "test issue",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 18, 18, 55, 0, 0, time.UTC),
			ClosedAt:  &closed,
			URL:       "https://github.com/test/repo/issues/119",
		},
		LeadTime: model.NewMetric(
			&model.Event{Time: time.Date(2026, 3, 18, 18, 55, 0, 0, time.UTC), Signal: model.SignalIssueCreated},
			&model.Event{Time: closed, Signal: model.SignalIssueClosed},
		),
		Category: "feature",
		LinkedPRs: []LinkedPR{
			{
				PR: model.PR{
					Number:    120,
					Title:     "fix: something",
					CreatedAt: time.Date(2026, 3, 18, 18, 41, 0, 0, time.UTC),
					MergedAt:  &merged,
					URL:       "https://github.com/test/repo/pull/120",
				},
				CycleTime: model.NewMetric(
					&model.Event{Time: time.Date(2026, 3, 18, 18, 41, 0, 0, time.UTC), Signal: model.SignalPRCreated},
					&model.Event{Time: merged, Signal: model.SignalPRMerged},
				),
			},
		},
	}

	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.Markdown, Owner: "test", Repo: "repo"}
	if err := p.Render(rc); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	// Check key elements
	checks := []struct {
		label, contains string
	}{
		{"Metrics header", "### Metrics"},
		{"opened timestamp", "Opened 2026-03-18"},
		{"closed timestamp", "Closed 2026-03-18"},
		{"lead time row", "Lead Time"},
		{"eng cycle time row", "Eng Cycle Time"},
		{"PR link", "#120"},
		{"footer", "gh-velocity"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.contains) {
			t.Errorf("missing %s: expected %q in output:\n%s", c.label, c.contains, out)
		}
	}
}

func TestRenderJSON(t *testing.T) {
	closed := time.Date(2026, 3, 18, 18, 56, 0, 0, time.UTC)

	p := &Pipeline{
		IssueNumber: 42,
		Issue: &model.Issue{
			Number:    42,
			Title:     "test",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC),
			ClosedAt:  &closed,
			URL:       "https://github.com/test/repo/issues/42",
		},
		LeadTime: model.NewMetric(
			&model.Event{Time: time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC), Signal: model.SignalIssueCreated},
			&model.Event{Time: closed, Signal: model.SignalIssueClosed},
		),
		Category: "bug",
	}

	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.JSON}
	if err := p.Render(rc); err != nil {
		t.Fatalf("Render: %v", err)
	}

	var out jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}

	if out.Issue.Number != 42 {
		t.Errorf("issue number: got %d, want 42", out.Issue.Number)
	}
	if out.Issue.Category != "bug" {
		t.Errorf("category: got %q, want %q", out.Issue.Category, "bug")
	}
	if out.Metrics.LeadTime.Status != "completed" {
		t.Errorf("lead time status: got %q, want %q", out.Metrics.LeadTime.Status, "completed")
	}
	if out.Metrics.CycleTime.Status != "not_applicable" {
		t.Errorf("cycle time status: got %q, want %q", out.Metrics.CycleTime.Status, "not_applicable")
	}
	if len(out.LinkedPRs) != 0 {
		t.Errorf("linked PRs: got %d, want 0", len(out.LinkedPRs))
	}
}
