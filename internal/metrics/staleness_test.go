package metrics

import (
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestComputeStaleness(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		updatedAt time.Time
		want      model.StalenessLevel
	}{
		{"just now", now, model.StalenessActive},
		{"1 day ago", now.Add(-24 * time.Hour), model.StalenessActive},
		{"2 days ago", now.Add(-48 * time.Hour), model.StalenessActive},
		{"3 days ago", now.Add(-72 * time.Hour), model.StalenessActive}, // exactly 3 = still active
		{"3.5 days ago", now.Add(-84 * time.Hour), model.StalenessAging},
		{"5 days ago", now.Add(-120 * time.Hour), model.StalenessAging},
		{"7 days ago", now.Add(-168 * time.Hour), model.StalenessAging}, // exactly 7 = still aging
		{"7.5 days ago", now.Add(-180 * time.Hour), model.StalenessStale},
		{"30 days ago", now.Add(-720 * time.Hour), model.StalenessStale},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeStaleness(tt.updatedAt, now)
			if got != tt.want {
				t.Errorf("ComputeStaleness() = %q, want %q", got, tt.want)
			}
		})
	}
}
