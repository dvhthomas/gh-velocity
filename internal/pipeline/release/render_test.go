package release

import (
	"testing"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestReleaseFlag(t *testing.T) {
	tests := []struct {
		name     string
		im       model.IssueMetrics
		expected string
	}{
		{
			name:     "no flags",
			im:       model.IssueMetrics{Category: "feature"},
			expected: "",
		},
		{
			name:     "bug category gets bug emoji",
			im:       model.IssueMetrics{Category: "bug"},
			expected: format.FlagEmoji(format.FlagBug),
		},
		{
			name:     "outlier gets outlier emoji",
			im:       model.IssueMetrics{Category: "feature", LeadTimeOutlier: true},
			expected: format.FlagEmoji(format.FlagOutlier),
		},
		{
			name:     "cycle time outlier gets outlier emoji",
			im:       model.IssueMetrics{Category: "feature", CycleTimeOutlier: true},
			expected: format.FlagEmoji(format.FlagOutlier),
		},
		{
			name:     "bug and outlier gets both emojis",
			im:       model.IssueMetrics{Category: "bug", LeadTimeOutlier: true},
			expected: format.FlagEmoji(format.FlagBug) + format.FlagEmoji(format.FlagOutlier),
		},
		{
			name:     "chore category no flags",
			im:       model.IssueMetrics{Category: "chore"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := releaseFlag(tt.im)
			if got != tt.expected {
				t.Errorf("releaseFlag() = %q, want %q", got, tt.expected)
			}
		})
	}
}
