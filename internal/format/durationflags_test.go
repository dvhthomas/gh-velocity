package format

import (
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestClassifyDurationFlags(t *testing.T) {
	outlierCutoff := 48 * time.Hour

	tests := []struct {
		name     string
		duration *time.Duration
		stats    model.Stats
		want     []string
	}{
		{
			name:     "nil duration",
			duration: nil,
			want:     nil,
		},
		{
			name:     "zero duration is noise",
			duration: durationPtr(0),
			want:     []string{FlagNoise},
		},
		{
			name:     "30 seconds is noise",
			duration: durationPtr(30 * time.Second),
			want:     []string{FlagNoise},
		},
		{
			name:     "exactly 1 minute is hotfix (boundary)",
			duration: durationPtr(time.Minute),
			want:     []string{FlagHotfix},
		},
		{
			name:     "24 hours is hotfix",
			duration: durationPtr(24 * time.Hour),
			want:     []string{FlagHotfix},
		},
		{
			name:     "exactly 72 hours is hotfix (boundary)",
			duration: durationPtr(72 * time.Hour),
			want:     []string{FlagHotfix},
		},
		{
			name:     "73 hours is normal (no flags)",
			duration: durationPtr(73 * time.Hour),
			want:     nil,
		},
		{
			name:     "outlier (over cutoff with stats)",
			duration: durationPtr(100 * time.Hour),
			stats:    model.Stats{OutlierCutoff: &outlierCutoff},
			want:     []string{FlagOutlier},
		},
		{
			name:     "hotfix AND outlier (combined flags)",
			duration: durationPtr(50 * time.Hour),
			stats:    model.Stats{OutlierCutoff: durationPtr(24 * time.Hour)},
			want:     []string{FlagHotfix, FlagOutlier},
		},
		{
			name:     "noise is not outlier even with stats (too short for cutoff)",
			duration: durationPtr(30 * time.Second),
			stats:    model.Stats{OutlierCutoff: durationPtr(10 * time.Second)},
			want:     []string{FlagNoise, FlagOutlier},
		},
		{
			name:     "nil outlier cutoff means no outlier flag",
			duration: durationPtr(1000 * time.Hour),
			stats:    model.Stats{},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model.Metric{Duration: tt.duration}
			got := ClassifyDurationFlags(tt.duration, m, tt.stats)
			if len(got) != len(tt.want) {
				t.Fatalf("ClassifyDurationFlags() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ClassifyDurationFlags()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFlagEmojis(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
		want  string
	}{
		{"nil flags", nil, ""},
		{"empty flags", []string{}, ""},
		{"single flag", []string{FlagOutlier}, "🚩"},
		{"multiple flags", []string{FlagOutlier, FlagNoise}, "🚩🤖"},
		{"all duration flags", []string{FlagNoise, FlagHotfix, FlagOutlier}, "🤖⚡🚩"},
		{"unknown flag", []string{"unknown"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FlagEmojis(tt.flags)
			if got != tt.want {
				t.Errorf("FlagEmojis(%v) = %q, want %q", tt.flags, got, tt.want)
			}
		})
	}
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}
