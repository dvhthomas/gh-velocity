package metrics

import (
	"testing"
	"time"
)

func TestCycleTime(t *testing.T) {
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		firstCommit time.Time
		end         time.Time
		wantNil     bool
		wantDur     time.Duration
	}{
		{
			name:        "normal cycle",
			firstCommit: now,
			end:         now.Add(24 * time.Hour),
			wantDur:     24 * time.Hour,
		},
		{
			name:        "zero first commit",
			firstCommit: time.Time{},
			end:         now,
			wantNil:     true,
		},
		{
			name:        "zero end",
			firstCommit: now,
			end:         time.Time{},
			wantNil:     true,
		},
		{
			name:        "both zero",
			firstCommit: time.Time{},
			end:         time.Time{},
			wantNil:     true,
		},
		{
			name:        "same time",
			firstCommit: now,
			end:         now,
			wantDur:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CycleTime(tt.firstCommit, tt.end)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil")
			}
			if *got != tt.wantDur {
				t.Errorf("expected %v, got %v", tt.wantDur, *got)
			}
		})
	}
}
