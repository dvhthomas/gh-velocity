package cycletime

import (
	"context"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/metrics"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestIssuePipelineProcessData(t *testing.T) {
	// Use PR strategy for testing since it doesn't require an API client.
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	merged := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)

	p := &IssuePipeline{
		Strategy:    &metrics.PRStrategy{},
		StrategyStr: model.StrategyPR,
		Issue: &model.Issue{
			Number:    42,
			Title:     "Fix bug",
			State:     "closed",
			CreatedAt: created,
			ClosedAt:  &merged,
		},
		PR: &model.PR{
			Number:    100,
			CreatedAt: created,
			MergedAt:  &merged,
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData() error: %v", err)
	}

	if p.CycleTime.Duration == nil {
		t.Fatal("expected non-nil duration")
	}

	want := 2 * 24 * time.Hour
	if *p.CycleTime.Duration != want {
		t.Errorf("duration = %v, want %v", *p.CycleTime.Duration, want)
	}
}

func TestIssuePipelineProcessData_NoProject(t *testing.T) {
	// IssueStrategy without project returns zero metrics.
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)

	p := &IssuePipeline{
		Strategy:    &metrics.IssueStrategy{}, // no client/project
		StrategyStr: model.StrategyIssue,
		Issue: &model.Issue{
			Number:    42,
			Title:     "Fix bug",
			State:     "closed",
			CreatedAt: created,
			ClosedAt:  &closed,
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData() error: %v", err)
	}

	// No project configured, so no cycle time signal.
	if p.CycleTime.Duration != nil {
		t.Errorf("expected nil duration (no project), got %v", *p.CycleTime.Duration)
	}
}

func TestIssuePipelineProcessData_WarnsOnNA_IssueStrategy(t *testing.T) {
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)

	p := &IssuePipeline{
		Strategy:    &metrics.IssueStrategy{},
		StrategyStr: model.StrategyIssue,
		Issue: &model.Issue{
			Number: 42, Title: "Fix bug", State: "closed",
			CreatedAt: created, ClosedAt: &closed,
		},
	}

	_ = p.ProcessData()

	if len(p.Warnings) == 0 {
		t.Fatal("expected warning for issue strategy with no project")
	}
	if !containsStr(p.Warnings[0], "lifecycle.in-progress") {
		t.Errorf("warning should mention lifecycle.in-progress config, got: %s", p.Warnings[0])
	}
}

func TestIssuePipelineProcessData_WarnsOnNA_PRStrategyNoPR(t *testing.T) {
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)

	p := &IssuePipeline{
		Strategy:    &metrics.PRStrategy{},
		StrategyStr: model.StrategyPR,
		Issue: &model.Issue{
			Number: 42, Title: "Fix bug", State: "closed",
			CreatedAt: created, ClosedAt: &closed,
		},
		PR: nil, // no closing PR
	}

	_ = p.ProcessData()

	if len(p.Warnings) == 0 {
		t.Fatal("expected warning for PR strategy with no closing PR")
	}
	if !containsStr(p.Warnings[0], "closing PR") {
		t.Errorf("warning should mention closing PR, got: %s", p.Warnings[0])
	}
}

func TestBulkPipelineProcessData_WarnsOnAllNA(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	closed := now.Add(-24 * time.Hour)

	p := &BulkPipeline{
		Owner:       "org",
		Repo:        "repo",
		Since:       now.Add(-30 * 24 * time.Hour),
		Until:       now,
		Strategy:    &metrics.IssueStrategy{}, // no project = all N/A
		StrategyStr: model.StrategyIssue,
		issues: []model.Issue{
			{Number: 1, CreatedAt: now.Add(-72 * time.Hour), ClosedAt: &closed},
			{Number: 2, CreatedAt: now.Add(-96 * time.Hour), ClosedAt: &closed},
		},
	}

	_ = p.ProcessData()

	if len(p.Warnings) == 0 {
		t.Fatal("expected warning when all items have N/A cycle time")
	}
	if !containsStr(p.Warnings[0], "lifecycle.in-progress") {
		t.Errorf("warning should mention lifecycle.in-progress config, got: %s", p.Warnings[0])
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexStr(s, sub) >= 0)
}

func indexStr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// negativeDurationStrategy is a test strategy that always returns a negative duration.
type negativeDurationStrategy struct{}

func (s *negativeDurationStrategy) Name() string { return model.StrategyIssue }
func (s *negativeDurationStrategy) Compute(_ context.Context, input metrics.CycleTimeInput) model.Metric {
	if input.Issue == nil || input.Issue.ClosedAt == nil {
		return model.Metric{}
	}
	// Simulate: project board updatedAt is after issue close.
	start := &model.Event{Time: input.Issue.ClosedAt.Add(time.Hour), Signal: model.SignalStatusChange}
	end := &model.Event{Time: *input.Issue.ClosedAt, Signal: model.SignalIssueClosed}
	return model.NewMetric(start, end)
}

func TestIssuePipelineProcessData_WarnsOnNegativeDuration(t *testing.T) {
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)

	p := &IssuePipeline{
		Strategy:    &negativeDurationStrategy{},
		StrategyStr: model.StrategyIssue,
		Issue: &model.Issue{
			Number: 42, Title: "Fix bug", State: "closed",
			CreatedAt: created, ClosedAt: &closed,
		},
	}

	_ = p.ProcessData()

	if p.CycleTime.Duration == nil {
		t.Fatal("expected non-nil (negative) duration")
	}
	if *p.CycleTime.Duration >= 0 {
		t.Errorf("expected negative duration, got %v", *p.CycleTime.Duration)
	}
	found := false
	for _, w := range p.Warnings {
		if containsStr(w, "Negative cycle time") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about negative cycle time, got: %v", p.Warnings)
	}
}

func TestBulkPipelineProcessData_NegativeDurationsFiltered(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	closed1 := now.Add(-24 * time.Hour)
	closed2 := now.Add(-48 * time.Hour)

	p := &BulkPipeline{
		Owner:       "org",
		Repo:        "repo",
		Since:       now.Add(-30 * 24 * time.Hour),
		Until:       now,
		Strategy:    &negativeDurationStrategy{},
		StrategyStr: model.StrategyIssue,
		issues: []model.Issue{
			{Number: 1, CreatedAt: now.Add(-72 * time.Hour), ClosedAt: &closed1},
			{Number: 2, CreatedAt: now.Add(-96 * time.Hour), ClosedAt: &closed2},
		},
	}

	_ = p.ProcessData()

	if p.Stats.NegativeCount != 2 {
		t.Errorf("expected NegativeCount 2, got %d", p.Stats.NegativeCount)
	}
	if p.Stats.Count != 0 {
		t.Errorf("expected Count 0 (all negatives filtered), got %d", p.Stats.Count)
	}
	found := false
	for _, w := range p.Warnings {
		if containsStr(w, "negative cycle times") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about negative durations, got: %v", p.Warnings)
	}
}

func TestPRPipelineProcessData(t *testing.T) {
	created := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)
	merged := time.Date(2026, 1, 6, 14, 0, 0, 0, time.UTC)

	p := &PRPipeline{
		PR: &model.PR{
			Number:    99,
			Title:     "Add feature",
			State:     "merged",
			CreatedAt: created,
			MergedAt:  &merged,
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData() error: %v", err)
	}

	if p.CycleTime.Duration == nil {
		t.Fatal("expected non-nil duration")
	}

	want := 28 * time.Hour
	if *p.CycleTime.Duration != want {
		t.Errorf("duration = %v, want %v", *p.CycleTime.Duration, want)
	}
}

func TestBulkPipelineProcessData(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	merged1 := now.Add(-24 * time.Hour)
	merged2 := now.Add(-48 * time.Hour)

	// Use PR strategy for bulk test since issue strategy needs API.
	p := &BulkPipeline{
		Owner:       "org",
		Repo:        "repo",
		Since:       now.Add(-30 * 24 * time.Hour),
		Until:       now,
		Strategy:    &metrics.PRStrategy{},
		StrategyStr: model.StrategyPR,
		issues: []model.Issue{
			{
				Number:    1,
				CreatedAt: now.Add(-72 * time.Hour),
				ClosedAt:  &merged1,
			},
			{
				Number:    2,
				CreatedAt: now.Add(-96 * time.Hour),
				ClosedAt:  &merged2,
			},
		},
		ClosingPRs: map[int]*model.PR{
			1: {Number: 10, CreatedAt: now.Add(-72 * time.Hour), MergedAt: &merged1},
			2: {Number: 11, CreatedAt: now.Add(-96 * time.Hour), MergedAt: &merged2},
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData() error: %v", err)
	}

	if len(p.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(p.Items))
	}

	if p.Stats.Count != 2 {
		t.Errorf("Stats.Count = %d, want 2", p.Stats.Count)
	}
}
