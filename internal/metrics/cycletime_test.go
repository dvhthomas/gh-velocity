package metrics

import (
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestNewMetric(t *testing.T) {
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		start      *model.Event
		end        *model.Event
		wantNilDur bool
		wantDur    time.Duration
	}{
		{
			name:    "both events",
			start:   &model.Event{Time: now, Signal: model.SignalCommit},
			end:     &model.Event{Time: now.Add(24 * time.Hour), Signal: model.SignalIssueClosed},
			wantDur: 24 * time.Hour,
		},
		{
			name:       "nil start",
			start:      nil,
			end:        &model.Event{Time: now, Signal: model.SignalIssueClosed},
			wantNilDur: true,
		},
		{
			name:       "nil end",
			start:      &model.Event{Time: now, Signal: model.SignalCommit},
			end:        nil,
			wantNilDur: true,
		},
		{
			name:       "both nil",
			start:      nil,
			end:        nil,
			wantNilDur: true,
		},
		{
			name:    "same time",
			start:   &model.Event{Time: now, Signal: model.SignalPRCreated},
			end:     &model.Event{Time: now, Signal: model.SignalPRMerged},
			wantDur: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.NewMetric(tt.start, tt.end)
			if tt.wantNilDur {
				if got.Duration != nil {
					t.Errorf("expected nil duration, got %v", *got.Duration)
				}
				return
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
