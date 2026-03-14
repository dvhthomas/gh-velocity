package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

var testNow = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func TestIssueStrategy_NoSignalSource(t *testing.T) {
	// IssueStrategy without any signal source (no project, no match) returns zero.
	closed := testNow.Add(48 * time.Hour)
	tests := []struct {
		name  string
		input CycleTimeInput
	}{
		{name: "nil issue", input: CycleTimeInput{}},
		{name: "closed issue", input: CycleTimeInput{Issue: &model.Issue{Number: 1, CreatedAt: testNow, ClosedAt: &closed}}},
		{name: "open issue", input: CycleTimeInput{Issue: &model.Issue{Number: 2, CreatedAt: testNow}}},
	}

	s := &IssueStrategy{} // no client, no project, no match — signal unavailable
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Compute(context.Background(), tt.input)
			if got.Duration != nil {
				t.Errorf("expected nil duration (no signal source), got %v", *got.Duration)
			}
			if got.Start != nil {
				t.Error("expected nil Start (no signal source)")
			}
		})
	}
}

func TestIssueStrategy_MatchWithoutClient(t *testing.T) {
	// InProgressMatch is set but no client — should return zero (can't query API).
	closed := testNow.Add(48 * time.Hour)
	s := &IssueStrategy{InProgressMatch: []string{"label:in-progress"}}
	got := s.Compute(context.Background(), CycleTimeInput{
		Issue: &model.Issue{Number: 1, CreatedAt: testNow, ClosedAt: &closed},
	})
	if got.Duration != nil {
		t.Errorf("expected nil duration (no client), got %v", *got.Duration)
	}
}

func TestPRStrategy(t *testing.T) {
	merged := testNow.Add(24 * time.Hour)

	tests := []struct {
		name       string
		input      CycleTimeInput
		wantNilDur bool
		wantDur    time.Duration
		wantStart  string
		wantEnd    string
	}{
		{
			name:       "nil PR",
			input:      CycleTimeInput{},
			wantNilDur: true,
		},
		{
			name: "merged PR",
			input: CycleTimeInput{PR: &model.PR{
				Number:    42,
				CreatedAt: testNow,
				MergedAt:  &merged,
			}},
			wantDur:   24 * time.Hour,
			wantStart: model.SignalPRCreated,
			wantEnd:   model.SignalPRMerged,
		},
		{
			name: "open PR — in progress",
			input: CycleTimeInput{PR: &model.PR{
				Number:    43,
				CreatedAt: testNow,
				MergedAt:  nil,
			}},
			wantNilDur: true,
			wantStart:  model.SignalPRCreated,
		},
		{
			name: "PR with issue — only uses PR",
			input: CycleTimeInput{
				Issue: &model.Issue{Number: 1, CreatedAt: testNow},
				PR:    &model.PR{Number: 42, CreatedAt: testNow, MergedAt: &merged},
			},
			wantDur:   24 * time.Hour,
			wantStart: model.SignalPRCreated,
			wantEnd:   model.SignalPRMerged,
		},
	}

	s := &PRStrategy{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Compute(context.Background(), tt.input)
			assertMetric(t, got, tt.wantNilDur, tt.wantDur, tt.wantStart, tt.wantEnd)
		})
	}
}

func assertMetric(t *testing.T, got model.Metric, wantNilDur bool, wantDur time.Duration, wantStart, wantEnd string) {
	t.Helper()

	if wantStart != "" {
		if got.Start == nil {
			t.Fatal("expected non-nil Start")
		}
		if got.Start.Signal != wantStart {
			t.Errorf("start signal: want %q, got %q", wantStart, got.Start.Signal)
		}
	}

	if wantNilDur {
		if got.Duration != nil {
			t.Errorf("expected nil duration, got %v", *got.Duration)
		}
		return
	}

	if got.Duration == nil {
		t.Fatal("expected non-nil duration")
	}
	if *got.Duration != wantDur {
		t.Errorf("duration: want %v, got %v", wantDur, *got.Duration)
	}

	if wantEnd != "" {
		if got.End == nil {
			t.Fatal("expected non-nil End")
		}
		if got.End.Signal != wantEnd {
			t.Errorf("end signal: want %q, got %q", wantEnd, got.End.Signal)
		}
	}
}
