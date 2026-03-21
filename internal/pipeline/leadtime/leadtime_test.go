package leadtime

import (
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// ============================================================
// Compute tests (from metrics/leadtime_test.go)
// ============================================================

func TestCompute(t *testing.T) {
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	closed := now.Add(48 * time.Hour)

	tests := []struct {
		name       string
		issue      model.Issue
		wantNilDur bool
		wantDur    time.Duration
		wantStart  string
		wantEnd    string
	}{
		{
			name: "closed issue",
			issue: model.Issue{
				Number:    1,
				State:     "closed",
				CreatedAt: now,
				ClosedAt:  &closed,
			},
			wantDur:   48 * time.Hour,
			wantStart: model.SignalIssueCreated,
			wantEnd:   model.SignalIssueClosed,
		},
		{
			name: "open issue",
			issue: model.Issue{
				Number:    2,
				State:     "open",
				CreatedAt: now,
				ClosedAt:  nil,
			},
			wantNilDur: true,
			wantStart:  model.SignalIssueCreated,
		},
		{
			name: "zero duration",
			issue: model.Issue{
				Number:    3,
				State:     "closed",
				CreatedAt: now,
				ClosedAt:  &now,
			},
			wantDur:   0,
			wantStart: model.SignalIssueCreated,
			wantEnd:   model.SignalIssueClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compute(tt.issue)

			if got.Start == nil {
				t.Fatal("expected non-nil Start event")
			}
			if got.Start.Signal != tt.wantStart {
				t.Errorf("start signal: want %q, got %q", tt.wantStart, got.Start.Signal)
			}

			if tt.wantNilDur {
				if got.Duration != nil {
					t.Errorf("expected nil duration, got %v", *got.Duration)
				}
				if got.End != nil {
					t.Error("expected nil End event for open issue")
				}
				return
			}

			if got.End == nil {
				t.Fatal("expected non-nil End event")
			}
			if got.End.Signal != tt.wantEnd {
				t.Errorf("end signal: want %q, got %q", tt.wantEnd, got.End.Signal)
			}
			if got.Duration == nil {
				t.Fatal("expected non-nil duration")
			}
			if *got.Duration != tt.wantDur {
				t.Errorf("expected %v, got %v", tt.wantDur, *got.Duration)
			}
		})
	}
}

// ============================================================
// Pipeline tests (from metric/leadtime_test.go)
// ============================================================

func TestSinglePipelineProcessData(t *testing.T) {
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 1, 4, 12, 0, 0, 0, time.UTC)

	p := &SinglePipeline{
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

func TestSinglePipelineProcessData_OpenIssue(t *testing.T) {
	p := &SinglePipeline{
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

func TestBulkPipelineProcessData(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	closed1 := now.Add(-24 * time.Hour)
	closed2 := now.Add(-48 * time.Hour)

	p := &BulkPipeline{
		Owner: "org",
		Repo:  "repo",
		Since: now.Add(-30 * 24 * time.Hour),
		Until: now,
		Issues: []model.Issue{
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

	durCount := 0
	for _, item := range p.Items {
		if item.Metric.Duration != nil {
			durCount++
		}
	}
	if durCount != 2 {
		t.Errorf("got %d items with duration, want 2", durCount)
	}

	if p.Stats.Count != 2 {
		t.Errorf("Stats.Count = %d, want 2", p.Stats.Count)
	}
}
