package format

import (
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// JSONEvent is the JSON representation of a metric event.
type JSONEvent struct {
	Time   time.Time `json:"time"`
	Signal string    `json:"signal"`
	Detail string    `json:"detail,omitempty"`
}

// JSONMetric is the JSON representation of a metric with start/end events.
type JSONMetric struct {
	Start           *JSONEvent `json:"start"`
	End             *JSONEvent `json:"end"`
	DurationSeconds *int64     `json:"duration_seconds"`
	Duration        string     `json:"duration"` // human-readable
}

// MetricToJSON converts a model.Metric to its JSON representation.
func MetricToJSON(m model.Metric) JSONMetric {
	jm := JSONMetric{
		DurationSeconds: DurationToSeconds(m.Duration),
		Duration:        FormatMetricDuration(m),
	}
	if m.Start != nil {
		jm.Start = &JSONEvent{
			Time:   m.Start.Time.UTC(),
			Signal: m.Start.Signal,
			Detail: m.Start.Detail,
		}
	}
	if m.End != nil {
		jm.End = &JSONEvent{
			Time:   m.End.Time.UTC(),
			Signal: m.End.Signal,
			Detail: m.End.Detail,
		}
	}
	return jm
}

type JSONStats struct {
	Count                int           `json:"count"`
	MeanSeconds          *int64        `json:"mean_seconds"`
	MedianSeconds        *int64        `json:"median_seconds"`
	StdDevSeconds        *int64        `json:"stddev_seconds,omitempty"`
	P90Seconds           *int64        `json:"p90_seconds,omitempty"`
	P95Seconds           *int64        `json:"p95_seconds,omitempty"`
	OutlierCutoffSeconds *int64        `json:"outlier_cutoff_seconds,omitempty"`
	OutlierCount         int           `json:"outlier_count,omitempty"`
	Insights             []jsonInsight `json:"insights,omitempty"`
}

// DurationToSeconds converts a duration pointer to seconds.
func DurationToSeconds(d *time.Duration) *int64 {
	if d == nil {
		return nil
	}
	s := int64(d.Seconds())
	return &s
}

// StatsToJSON converts model.Stats to its JSON representation.
func StatsToJSON(s model.Stats) JSONStats {
	return JSONStats{
		Count:                s.Count,
		MeanSeconds:          DurationToSeconds(s.Mean),
		MedianSeconds:        DurationToSeconds(s.Median),
		StdDevSeconds:        DurationToSeconds(s.StdDev),
		P90Seconds:           DurationToSeconds(s.P90),
		P95Seconds:           DurationToSeconds(s.P95),
		OutlierCutoffSeconds: DurationToSeconds(s.OutlierCutoff),
		OutlierCount:         s.OutlierCount,
	}
}
