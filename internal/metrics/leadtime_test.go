package metrics

import (
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestLeadTime(t *testing.T) {
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
			got := LeadTime(tt.issue)

			// Start event should always be present
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
