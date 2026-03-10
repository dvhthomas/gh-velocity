package cycletime

import (
	"context"
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

var now = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func TestIssueStrategy(t *testing.T) {
	closed := now.Add(48 * time.Hour)

	tests := []struct {
		name       string
		input      Input
		wantNilDur bool
		wantDur    time.Duration
		wantStart  string
		wantEnd    string
	}{
		{
			name:       "nil issue",
			input:      Input{},
			wantNilDur: true,
		},
		{
			name: "closed issue",
			input: Input{Issue: &model.Issue{
				Number:    1,
				CreatedAt: now,
				ClosedAt:  &closed,
			}},
			wantDur:   48 * time.Hour,
			wantStart: model.SignalIssueCreated,
			wantEnd:   model.SignalIssueClosed,
		},
		{
			name: "open issue — in progress",
			input: Input{Issue: &model.Issue{
				Number:    2,
				CreatedAt: now,
				ClosedAt:  nil,
			}},
			wantNilDur: true,
			wantStart:  model.SignalIssueCreated,
		},
		{
			name: "zero duration",
			input: Input{Issue: &model.Issue{
				Number:    3,
				CreatedAt: now,
				ClosedAt:  &now,
			}},
			wantDur:   0,
			wantStart: model.SignalIssueCreated,
			wantEnd:   model.SignalIssueClosed,
		},
	}

	s := &IssueStrategy{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Compute(context.Background(), tt.input)
			assertMetric(t, got, tt.wantNilDur, tt.wantDur, tt.wantStart, tt.wantEnd)
		})
	}
}

func TestPRStrategy(t *testing.T) {
	merged := now.Add(24 * time.Hour)

	tests := []struct {
		name       string
		input      Input
		wantNilDur bool
		wantDur    time.Duration
		wantStart  string
		wantEnd    string
	}{
		{
			name:       "nil PR",
			input:      Input{},
			wantNilDur: true,
		},
		{
			name: "merged PR",
			input: Input{PR: &model.PR{
				Number:    42,
				CreatedAt: now,
				MergedAt:  &merged,
			}},
			wantDur:   24 * time.Hour,
			wantStart: model.SignalPRCreated,
			wantEnd:   model.SignalPRMerged,
		},
		{
			name: "open PR — in progress",
			input: Input{PR: &model.PR{
				Number:    43,
				CreatedAt: now,
				MergedAt:  nil,
			}},
			wantNilDur: true,
			wantStart:  model.SignalPRCreated,
		},
		{
			name: "PR with issue — only uses PR",
			input: Input{
				Issue: &model.Issue{Number: 1, CreatedAt: now},
				PR:    &model.PR{Number: 42, CreatedAt: now, MergedAt: &merged},
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
