package metric

import (
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/cycletime"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestCycleTimeIssuePipelineProcessData(t *testing.T) {
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)

	p := &CycleTimeIssuePipeline{
		Strategy:    &cycletime.IssueStrategy{},
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

	if p.CycleTime.Duration == nil {
		t.Fatal("expected non-nil duration")
	}

	want := 2 * 24 * time.Hour
	if *p.CycleTime.Duration != want {
		t.Errorf("duration = %v, want %v", *p.CycleTime.Duration, want)
	}
}

func TestCycleTimePRPipelineProcessData(t *testing.T) {
	created := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)
	merged := time.Date(2026, 1, 6, 14, 0, 0, 0, time.UTC)

	p := &CycleTimePRPipeline{
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

func TestCycleTimeBulkPipelineProcessData(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	closed1 := now.Add(-24 * time.Hour)
	closed2 := now.Add(-48 * time.Hour)

	p := &CycleTimeBulkPipeline{
		Owner:       "org",
		Repo:        "repo",
		Since:       now.Add(-30 * 24 * time.Hour),
		Until:       now,
		Strategy:    &cycletime.IssueStrategy{},
		StrategyStr: "issue",
		issues: []model.Issue{
			{
				Number:    1,
				CreatedAt: now.Add(-72 * time.Hour),
				ClosedAt:  &closed1,
			},
			{
				Number:    2,
				CreatedAt: now.Add(-96 * time.Hour),
				ClosedAt:  &closed2,
			},
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
