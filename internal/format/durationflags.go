package format

import (
	"strings"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/metrics"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// Duration classification thresholds for bulk metric detail tables.
const (
	NoiseThreshold  = time.Minute    // items resolved faster than this are likely noise/automation
	HotfixThreshold = 72 * time.Hour // items resolved within this window are hotfixes
)

// ClassifyDurationFlags returns the applicable flag constants for a
// duration-based metric item. Used by cycletime and leadtime bulk rendering.
// Reviews and other non-duration metrics use different classification logic.
func ClassifyDurationFlags(duration *time.Duration, metric model.Metric, stats model.Stats) []string {
	var flags []string
	if duration != nil && *duration < NoiseThreshold {
		flags = append(flags, FlagNoise)
	}
	if duration != nil && *duration <= HotfixThreshold && *duration >= NoiseThreshold {
		flags = append(flags, FlagHotfix)
	}
	if metrics.IsOutlier(metric, stats) {
		flags = append(flags, FlagOutlier)
	}
	return flags
}

// FlagEmojis concatenates emoji for a set of flags into a single string.
func FlagEmojis(flags []string) string {
	var s strings.Builder
	for _, f := range flags {
		s.WriteString(FlagEmoji(f))
	}
	return s.String()
}
