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
		name    string
		issue   model.Issue
		wantNil bool
		wantDur time.Duration
	}{
		{
			name: "closed issue",
			issue: model.Issue{
				Number:    1,
				State:     "closed",
				CreatedAt: now,
				ClosedAt:  &closed,
			},
			wantDur: 48 * time.Hour,
		},
		{
			name: "open issue",
			issue: model.Issue{
				Number:    2,
				State:     "open",
				CreatedAt: now,
				ClosedAt:  nil,
			},
			wantNil: true,
		},
		{
			name: "zero duration",
			issue: model.Issue{
				Number:    3,
				State:     "closed",
				CreatedAt: now,
				ClosedAt:  &now,
			},
			wantDur: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LeadTime(tt.issue)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil duration")
			}
			if *got != tt.wantDur {
				t.Errorf("expected %v, got %v", tt.wantDur, *got)
			}
		})
	}
}
