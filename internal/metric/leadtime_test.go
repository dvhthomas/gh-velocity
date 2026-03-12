package metric

import (
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestLeadTimeSingleProcessData(t *testing.T) {
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 1, 4, 12, 0, 0, 0, time.UTC)

	p := &LeadTimeSinglePipeline{
		Owner:       "org",
		Repo:        "repo",
		IssueNumber: 42,
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

	if p.LeadTime.Duration == nil {
		t.Fatal("expected non-nil duration")
	}

	want := 3 * 24 * time.Hour
	if *p.LeadTime.Duration != want {
		t.Errorf("duration = %v, want %v", *p.LeadTime.Duration, want)
	}
}

func TestLeadTimeSingleProcessData_OpenIssue(t *testing.T) {
	p := &LeadTimeSinglePipeline{
		Issue: &model.Issue{
			Number:    10,
			Title:     "Open issue",
			State:     "open",
			CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData() error: %v", err)
	}

	if p.LeadTime.Duration != nil {
		t.Errorf("expected nil duration for open issue, got %v", *p.LeadTime.Duration)
	}
}

func TestLeadTimeBulkProcessData(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	closed1 := now.Add(-24 * time.Hour)
	closed2 := now.Add(-48 * time.Hour)

	p := &LeadTimeBulkPipeline{
		Owner: "org",
		Repo:  "repo",
		Since: now.Add(-30 * 24 * time.Hour),
		Until: now,
		issues: []model.Issue{
			{
				Number:    1,
				Title:     "Issue 1",
				State:     "closed",
				CreatedAt: now.Add(-72 * time.Hour),
				ClosedAt:  &closed1,
			},
			{
				Number:    2,
				Title:     "Issue 2",
				State:     "closed",
				CreatedAt: now.Add(-96 * time.Hour),
				ClosedAt:  &closed2,
			},
			{
				Number:    3,
				Title:     "Open issue",
				State:     "open",
				CreatedAt: now.Add(-24 * time.Hour),
			},
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData() error: %v", err)
	}

	if len(p.Items) != 3 {
		t.Fatalf("got %d items, want 3", len(p.Items))
	}

	// Two closed issues should have durations
	durCount := 0
	for _, item := range p.Items {
		if item.Metric.Duration != nil {
			durCount++
		}
	}
	if durCount != 2 {
		t.Errorf("got %d items with duration, want 2", durCount)
	}

	// Stats should be computed from 2 durations
	if p.Stats.Count != 2 {
		t.Errorf("Stats.Count = %d, want 2", p.Stats.Count)
	}
}
