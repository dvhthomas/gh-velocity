package format

import (
	"encoding/json"
	"io"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// WriteIndentedJSON writes v as indented JSON to w.
// Replaces the enc := json.NewEncoder(w); enc.SetIndent("", "  "); enc.Encode(v)
// boilerplate that appears in every render file.
func WriteIndentedJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// JSONInsight is the JSON representation of a model.Insight.
type JSONInsight struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// InsightsToJSON converts a slice of model.Insight to their JSON representation.
func InsightsToJSON(insights []model.Insight) []JSONInsight {
	if len(insights) == 0 {
		return nil
	}
	out := make([]JSONInsight, len(insights))
	for i, ins := range insights {
		out[i] = JSONInsight{Type: ins.Type, Message: ins.Message}
	}
	return out
}

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
	Q1Seconds            *int64        `json:"q1_seconds,omitempty"`
	Q3Seconds            *int64        `json:"q3_seconds,omitempty"`
	OutlierCutoffSeconds *int64        `json:"outlier_cutoff_seconds,omitempty"`
	OutlierCount         int           `json:"outlier_count,omitempty"`
	CV                   *float64      `json:"cv,omitempty"`
	Predictability       string        `json:"predictability,omitempty"`
	Insights             []JSONInsight `json:"insights,omitempty"`
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
	cv := ComputeCV(s)
	return JSONStats{
		Count:                s.Count,
		MeanSeconds:          DurationToSeconds(s.Mean),
		MedianSeconds:        DurationToSeconds(s.Median),
		StdDevSeconds:        DurationToSeconds(s.StdDev),
		P90Seconds:           DurationToSeconds(s.P90),
		P95Seconds:           DurationToSeconds(s.P95),
		Q1Seconds:            DurationToSeconds(s.Q1),
		Q3Seconds:            DurationToSeconds(s.Q3),
		OutlierCutoffSeconds: DurationToSeconds(s.OutlierCutoff),
		OutlierCount:         s.OutlierCount,
		CV:                   cv,
		Predictability:       PredictabilityLabel(cv),
	}
}
