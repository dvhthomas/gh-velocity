package metrics

import (
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// NewMetric creates a Metric from start and end events, computing Duration
// when both are present.
// Deprecated: Use model.NewMetric directly.
func NewMetric(start, end *model.Event) model.Metric {
	return model.NewMetric(start, end)
}

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
