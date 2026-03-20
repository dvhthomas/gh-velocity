package pr

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestProcessData_MergedPRWithReviews(t *testing.T) {
	merged := time.Date(2026, 3, 19, 2, 16, 0, 0, time.UTC)
	firstReview := time.Date(2026, 3, 19, 2, 1, 0, 0, time.UTC)

	p := &Pipeline{
		PRNumber: 125,
		PR: &model.PR{
			Number:    125,
			Title:     "feat: HTML format",
			Author:    "dvhthomas",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 19, 1, 49, 0, 0, time.UTC),
			MergedAt:  &merged,
			URL:       "https://github.com/dvhthomas/gh-velocity/pull/125",
		},
		Reviews: []model.Review{
			{Author: "reviewer1", State: "APPROVED", SubmittedAt: firstReview},
		},
		ClosedIssues: []model.Issue{
			{Number: 119, Title: "auto-detect noise labels", URL: "https://github.com/dvhthomas/gh-velocity/issues/119"},
		},
		CommitMessages: []string{"feat: add HTML format\n\nSome details"},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	// Cycle time: 27 minutes
	if p.CycleTime.Duration == nil {
		t.Fatal("expected cycle time duration, got nil")
	}
	if *p.CycleTime.Duration != 27*time.Minute {
		t.Errorf("cycle time: got %v, want 27m", *p.CycleTime.Duration)
	}

	// Time to first review: 12 minutes
	if p.ReviewSummary.TimeToFirstReview == nil {
		t.Fatal("expected time to first review, got nil")
	}
	if *p.ReviewSummary.TimeToFirstReview != 12*time.Minute {
		t.Errorf("time to first review: got %v, want 12m", *p.ReviewSummary.TimeToFirstReview)
	}

	// Review rounds: 1 (one APPROVED)
	if p.ReviewSummary.ReviewRounds != 1 {
		t.Errorf("review rounds: got %d, want 1", p.ReviewSummary.ReviewRounds)
	}

	// Author type: human
	if p.AuthorType != model.AuthorHuman {
		t.Errorf("author type: got %q, want %q", p.AuthorType, model.AuthorHuman)
	}
}

func TestProcessData_OpenPR(t *testing.T) {
	p := &Pipeline{
		PRNumber: 42,
		PR: &model.PR{
			Number:    42,
			Title:     "wip: something",
			Author:    "dev",
			State:     "open",
			CreatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	if p.CycleTime.Duration != nil {
		t.Errorf("expected nil cycle time for open PR, got %v", *p.CycleTime.Duration)
	}
	if p.AuthorType != model.AuthorHuman {
		t.Errorf("author type: got %q, want %q", p.AuthorType, model.AuthorHuman)
	}
}

func TestProcessData_BotAuthored(t *testing.T) {
	merged := time.Date(2026, 3, 19, 0, 5, 0, 0, time.UTC)
	p := &Pipeline{
		PRNumber: 10,
		PR: &model.PR{
			Number:    10,
			Title:     "chore: bump deps",
			Author:    "dependabot[bot]",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC),
			MergedAt:  &merged,
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	if p.AuthorType != model.AuthorBot {
		t.Errorf("author type: got %q, want %q", p.AuthorType, model.AuthorBot)
	}
}

func TestProcessData_AgentAssisted(t *testing.T) {
	merged := time.Date(2026, 3, 19, 0, 30, 0, 0, time.UTC)
	p := &Pipeline{
		PRNumber: 20,
		PR: &model.PR{
			Number:    20,
			Title:     "feat: new feature",
			Author:    "dvhthomas",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC),
			MergedAt:  &merged,
		},
		CommitMessages: []string{
			"feat: implement feature\n\nCo-Authored-By: Claude <noreply@anthropic.com>",
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	if p.AuthorType != model.AuthorAgentAssisted {
		t.Errorf("author type: got %q, want %q", p.AuthorType, model.AuthorAgentAssisted)
	}
}

func TestProcessData_NoReviewData(t *testing.T) {
	merged := time.Date(2026, 3, 19, 0, 10, 0, 0, time.UTC)
	p := &Pipeline{
		PRNumber: 30,
		PR: &model.PR{
			Number:    30,
			Title:     "fix: typo",
			Author:    "dev",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC),
			MergedAt:  &merged,
		},
		// No reviews, no commit messages
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	if p.ReviewSummary.TimeToFirstReview != nil {
		t.Errorf("expected nil time to first review, got %v", *p.ReviewSummary.TimeToFirstReview)
	}
	if p.ReviewSummary.ReviewRounds != 0 {
		t.Errorf("review rounds: got %d, want 0", p.ReviewSummary.ReviewRounds)
	}
}

func TestProcessData_MultipleClosedIssues(t *testing.T) {
	merged := time.Date(2026, 3, 19, 0, 30, 0, 0, time.UTC)
	p := &Pipeline{
		PRNumber: 50,
		PR: &model.PR{
			Number:    50,
			Title:     "feat: big change",
			Author:    "dev",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC),
			MergedAt:  &merged,
		},
		ClosedIssues: []model.Issue{
			{Number: 48, Title: "issue one"},
			{Number: 49, Title: "issue two"},
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	if len(p.ClosedIssues) != 2 {
		t.Errorf("closed issues: got %d, want 2", len(p.ClosedIssues))
	}
}

func TestProcessData_MultipleReviewRounds(t *testing.T) {
	merged := time.Date(2026, 3, 19, 3, 0, 0, 0, time.UTC)
	p := &Pipeline{
		PRNumber: 60,
		PR: &model.PR{
			Number:    60,
			Title:     "feat: contested change",
			Author:    "dev",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC),
			MergedAt:  &merged,
		},
		Reviews: []model.Review{
			{Author: "reviewer1", State: "CHANGES_REQUESTED", SubmittedAt: time.Date(2026, 3, 19, 1, 0, 0, 0, time.UTC)},
			{Author: "reviewer1", State: "COMMENTED", SubmittedAt: time.Date(2026, 3, 19, 1, 30, 0, 0, time.UTC)},
			{Author: "reviewer1", State: "CHANGES_REQUESTED", SubmittedAt: time.Date(2026, 3, 19, 2, 0, 0, 0, time.UTC)},
			{Author: "reviewer1", State: "APPROVED", SubmittedAt: time.Date(2026, 3, 19, 2, 30, 0, 0, time.UTC)},
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	// Time to first review: 1h (first CHANGES_REQUESTED)
	if p.ReviewSummary.TimeToFirstReview == nil {
		t.Fatal("expected time to first review, got nil")
	}
	if *p.ReviewSummary.TimeToFirstReview != time.Hour {
		t.Errorf("time to first review: got %v, want 1h", *p.ReviewSummary.TimeToFirstReview)
	}

	// Review rounds: 3 (2 CHANGES_REQUESTED + 1 APPROVED, COMMENTED excluded)
	if p.ReviewSummary.ReviewRounds != 3 {
		t.Errorf("review rounds: got %d, want 3", p.ReviewSummary.ReviewRounds)
	}
}

func TestRenderMarkdown(t *testing.T) {
	merged := time.Date(2026, 3, 19, 2, 16, 0, 0, time.UTC)
	ttfr := 12 * time.Minute
	ct := 27 * time.Minute

	p := &Pipeline{
		PRNumber: 125,
		PR: &model.PR{
			Number:    125,
			Title:     "feat: HTML format",
			Author:    "dvhthomas",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 19, 1, 49, 0, 0, time.UTC),
			MergedAt:  &merged,
			URL:       "https://github.com/dvhthomas/gh-velocity/pull/125",
		},
		CycleTime: model.NewMetric(
			&model.Event{Time: time.Date(2026, 3, 19, 1, 49, 0, 0, time.UTC), Signal: model.SignalPRCreated},
			&model.Event{Time: merged, Signal: model.SignalPRMerged},
		),
		ReviewSummary: model.ReviewSummary{
			TimeToFirstReview: &ttfr,
			ReviewRounds:      1,
		},
		AuthorType: model.AuthorHuman,
		ClosedIssues: []model.Issue{
			{Number: 119, Title: "auto-detect noise labels", URL: "https://github.com/dvhthomas/gh-velocity/issues/119"},
		},
	}
	_ = ct // duration computed by NewMetric

	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.Markdown, Owner: "dvhthomas", Repo: "gh-velocity"}
	if err := p.Render(rc); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	checks := []struct {
		label    string
		contains string
	}{
		{"Metrics header", "### Metrics"},
		{"author in facts", "dvhthomas"},
		{"opened timestamp", "opened 2026-03-19"},
		{"merged timestamp", "merged 2026-03-19"},
		{"cycle time row", "Cycle Time"},
		{"time to first review", "Time to First Review"},
		{"review rounds", "Review Rounds"},
		{"closed issues section", "Closed Issues"},
		{"issue link", "#119"},
		{"footer", "gh-velocity"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.contains) {
			t.Errorf("missing %s: expected %q in output:\n%s", c.label, c.contains, out)
		}
	}
	// Human author type should NOT show agent/bot suffix
	if strings.Contains(out, "(agent") || strings.Contains(out, "(bot") {
		t.Error("human author type should not show agent/bot label in markdown")
	}
}

func TestRenderMarkdown_BotAuthor(t *testing.T) {
	merged := time.Date(2026, 3, 19, 0, 5, 0, 0, time.UTC)
	p := &Pipeline{
		PRNumber: 10,
		PR: &model.PR{
			Number:    10,
			Title:     "chore: bump deps",
			Author:    "dependabot[bot]",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC),
			MergedAt:  &merged,
		},
		CycleTime: model.NewMetric(
			&model.Event{Time: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC), Signal: model.SignalPRCreated},
			&model.Event{Time: merged, Signal: model.SignalPRMerged},
		),
		AuthorType: model.AuthorBot,
	}

	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.Markdown}
	if err := p.Render(rc); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "bot") {
		t.Error("bot author type should be surfaced in markdown")
	}
}

func TestRenderJSON(t *testing.T) {
	merged := time.Date(2026, 3, 19, 2, 16, 0, 0, time.UTC)
	ttfr := 12 * time.Minute

	p := &Pipeline{
		PRNumber: 125,
		PR: &model.PR{
			Number:    125,
			Title:     "feat: HTML format",
			Author:    "dvhthomas",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 19, 1, 49, 0, 0, time.UTC),
			MergedAt:  &merged,
			URL:       "https://github.com/dvhthomas/gh-velocity/pull/125",
		},
		CycleTime: model.NewMetric(
			&model.Event{Time: time.Date(2026, 3, 19, 1, 49, 0, 0, time.UTC), Signal: model.SignalPRCreated},
			&model.Event{Time: merged, Signal: model.SignalPRMerged},
		),
		ReviewSummary: model.ReviewSummary{
			TimeToFirstReview: &ttfr,
			ReviewRounds:      1,
		},
		AuthorType: model.AuthorHuman,
		ClosedIssues: []model.Issue{
			{Number: 119, Title: "auto-detect noise labels", URL: "https://github.com/dvhthomas/gh-velocity/issues/119"},
		},
		Warnings: []string{},
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

	if out.PR.Number != 125 {
		t.Errorf("pr number: got %d, want 125", out.PR.Number)
	}
	if out.PR.Author != "dvhthomas" {
		t.Errorf("author: got %q, want %q", out.PR.Author, "dvhthomas")
	}
	if out.PR.AuthorType != "human" {
		t.Errorf("author type: got %q, want %q", out.PR.AuthorType, "human")
	}
	if out.Metrics.CycleTime.Status != "completed" {
		t.Errorf("cycle time status: got %q, want %q", out.Metrics.CycleTime.Status, "completed")
	}
	if out.Metrics.TimeToFirstReview.Status != "completed" {
		t.Errorf("ttfr status: got %q, want %q", out.Metrics.TimeToFirstReview.Status, "completed")
	}
	if out.Metrics.ReviewRounds != 1 {
		t.Errorf("review rounds: got %d, want 1", out.Metrics.ReviewRounds)
	}
	if len(out.ClosedIssues) != 1 {
		t.Errorf("closed issues: got %d, want 1", len(out.ClosedIssues))
	}
}

func TestRenderJSON_NoReviews(t *testing.T) {
	merged := time.Date(2026, 3, 19, 0, 10, 0, 0, time.UTC)
	p := &Pipeline{
		PRNumber: 30,
		PR: &model.PR{
			Number:    30,
			Title:     "fix: typo",
			Author:    "dev",
			State:     "closed",
			CreatedAt: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC),
			MergedAt:  &merged,
			URL:       "https://github.com/test/repo/pull/30",
		},
		CycleTime: model.NewMetric(
			&model.Event{Time: time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC), Signal: model.SignalPRCreated},
			&model.Event{Time: merged, Signal: model.SignalPRMerged},
		),
		AuthorType: model.AuthorHuman,
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

	if out.Metrics.TimeToFirstReview.Status != "not_applicable" {
		t.Errorf("ttfr status: got %q, want %q", out.Metrics.TimeToFirstReview.Status, "not_applicable")
	}
	if out.Metrics.ReviewRounds != 0 {
		t.Errorf("review rounds: got %d, want 0", out.Metrics.ReviewRounds)
	}
	if len(out.ClosedIssues) != 0 {
		t.Errorf("closed issues: got %d, want 0", len(out.ClosedIssues))
	}
}

func TestDetectAuthorType(t *testing.T) {
	tests := []struct {
		name     string
		author   string
		messages []string
		want     model.AuthorType
	}{
		{
			name:   "bot suffix",
			author: "dependabot[bot]",
			want:   model.AuthorBot,
		},
		{
			name:   "renovate bot",
			author: "renovate[bot]",
			want:   model.AuthorBot,
		},
		{
			name:     "agent-assisted anthropic",
			author:   "dvhthomas",
			messages: []string{"feat: thing\n\nCo-Authored-By: Claude <noreply@anthropic.com>"},
			want:     model.AuthorAgentAssisted,
		},
		{
			name:     "agent-assisted copilot",
			author:   "dvhthomas",
			messages: []string{"feat: thing\n\nCo-Authored-By: Copilot <noreply@github.com>"},
			want:     model.AuthorAgentAssisted,
		},
		{
			name:     "human with normal co-author",
			author:   "dvhthomas",
			messages: []string{"feat: thing\n\nCo-Authored-By: Alice <alice@example.com>"},
			want:     model.AuthorHuman,
		},
		{
			name:   "plain human",
			author: "dvhthomas",
			want:   model.AuthorHuman,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectAuthorType(tt.author, tt.messages)
			if got != tt.want {
				t.Errorf("detectAuthorType(%q, ...): got %q, want %q", tt.author, got, tt.want)
			}
		})
	}
}
