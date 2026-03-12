package cycletime

import (
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestIssuePipelineProcessData(t *testing.T) {
	// Use PR strategy for testing since it doesn't require an API client.
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	merged := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)

	p := &IssuePipeline{
		Strategy:    &metrics.PRStrategy{},
		StrategyStr: "pr",
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
		StrategyStr: "issue",
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
		StrategyStr: "pr",
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
