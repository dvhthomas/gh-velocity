package metrics

import (
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestCycleTime(t *testing.T) {
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		start      *model.Event
		end        *model.Event
		wantNilDur bool
		wantDur    time.Duration
	}{
		{
			name:    "normal cycle",
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
			got := CycleTime(tt.start, tt.end)
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

func TestNewMetric(t *testing.T) {
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	t.Run("both events", func(t *testing.T) {
		start := &model.Event{Time: now, Signal: model.SignalIssueCreated}
		end := &model.Event{Time: now.Add(48 * time.Hour), Signal: model.SignalIssueClosed}
		m := NewMetric(start, end)
		if m.Start != start {
			t.Error("start not set")
		}
		if m.End != end {
			t.Error("end not set")
		}
		if m.Duration == nil || *m.Duration != 48*time.Hour {
			t.Errorf("expected 48h duration, got %v", m.Duration)
		}
	})

	t.Run("nil start", func(t *testing.T) {
		end := &model.Event{Time: now, Signal: model.SignalIssueClosed}
		m := NewMetric(nil, end)
		if m.Duration != nil {
			t.Errorf("expected nil duration, got %v", *m.Duration)
		}
	})

	t.Run("nil end", func(t *testing.T) {
		start := &model.Event{Time: now, Signal: model.SignalIssueCreated}
		m := NewMetric(start, nil)
		if m.Duration != nil {
			t.Errorf("expected nil duration, got %v", *m.Duration)
		}
	})

	t.Run("both nil", func(t *testing.T) {
		m := NewMetric(nil, nil)
		if m.Duration != nil {
			t.Errorf("expected nil duration, got %v", *m.Duration)
		}
	})
}
