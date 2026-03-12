package metrics

import (
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// MetricDurations extracts non-nil durations from a slice of Metrics.
func MetricDurations(metrics []model.Metric) []time.Duration {
	var ds []time.Duration
	for _, m := range metrics {
		if m.Duration != nil {
			ds = append(ds, *m.Duration)
		}
	}
	return ds
}
