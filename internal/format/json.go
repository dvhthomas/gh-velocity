package format

import (
	"encoding/json"
	"io"
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

func metricToJSON(m model.Metric) JSONMetric {
	jm := JSONMetric{
		DurationSeconds: durationToSeconds(m.Duration),
		Duration:        FormatMetricDuration(m),
	}
	if m.Start != nil {
		jm.Start = &JSONEvent{
			Time:   m.Start.Time,
			Signal: m.Start.Signal,
			Detail: m.Start.Detail,
		}
	}
	if m.End != nil {
		jm.End = &JSONEvent{
			Time:   m.End.Time,
			Signal: m.End.Signal,
			Detail: m.End.Detail,
		}
	}
	return jm
}

// JSONLeadTimeOutput is the JSON representation of lead-time metrics.
type JSONLeadTimeOutput struct {
	Repository string     `json:"repository"`
	Issue      int        `json:"issue"`
	Title      string     `json:"title"`
	State      string     `json:"state"`
	LeadTime   JSONMetric `json:"lead_time"`
	Warnings   []string   `json:"warnings,omitempty"`
}

// JSONCycleTimeOutput is the JSON representation of cycle-time metrics.
type JSONCycleTimeOutput struct {
	Repository string     `json:"repository"`
	Issue      int        `json:"issue,omitempty"`
	PR         int        `json:"pr,omitempty"`
	Title      string     `json:"title"`
	State      string     `json:"state"`
	Commits    int        `json:"commits"`
	CycleTime  JSONMetric `json:"cycle_time"`
	Warnings   []string   `json:"warnings,omitempty"`
}

// JSONReleaseOutput is the JSON representation of release metrics.
type JSONReleaseOutput struct {
	Repository   string             `json:"repository"`
	Tag          string             `json:"tag"`
	PreviousTag  string             `json:"previous_tag,omitempty"`
	Date         time.Time          `json:"date"`
	CadenceHours *float64           `json:"cadence_hours,omitempty"`
	IsHotfix     bool               `json:"is_hotfix"`
	Composition  JSONComposition    `json:"composition"`
	Issues       []JSONIssueMetrics `json:"issues"`
	Aggregates   JSONAggregates     `json:"aggregates"`
	Warnings     []string           `json:"warnings,omitempty"`
}

type JSONComposition struct {
	TotalIssues    int                `json:"total_issues"`
	CategoryCounts map[string]int     `json:"category_counts"`
	CategoryRatios map[string]float64 `json:"category_ratios"`
}

type JSONIssueMetrics struct {
	Number           int        `json:"number"`
	Title            string     `json:"title"`
	Category         string     `json:"category"`
	LeadTime         JSONMetric `json:"lead_time"`
	CycleTime        JSONMetric `json:"cycle_time"`
	ReleaseLag       JSONMetric `json:"release_lag"`
	CommitCount      int        `json:"commit_count"`
	LeadTimeOutlier  bool       `json:"lead_time_outlier,omitempty"`
	CycleTimeOutlier bool       `json:"cycle_time_outlier,omitempty"`
}

type JSONAggregates struct {
	LeadTime   JSONStats `json:"lead_time"`
	CycleTime  JSONStats `json:"cycle_time"`
	ReleaseLag JSONStats `json:"release_lag"`
}

type JSONStats struct {
	Count                int    `json:"count"`
	MeanSeconds          *int64 `json:"mean_seconds"`
	MedianSeconds        *int64 `json:"median_seconds"`
	StdDevSeconds        *int64 `json:"stddev_seconds,omitempty"`
	P90Seconds           *int64 `json:"p90_seconds,omitempty"`
	P95Seconds           *int64 `json:"p95_seconds,omitempty"`
	OutlierCutoffSeconds *int64 `json:"outlier_cutoff_seconds,omitempty"`
	OutlierCount         int    `json:"outlier_count,omitempty"`
}

// WriteLeadTimeJSON writes lead-time metrics as JSON to the writer.
func WriteLeadTimeJSON(w io.Writer, repo string, issueNumber int, title, state string, m model.Metric, warnings []string) error {
	out := JSONLeadTimeOutput{
		Repository: repo,
		Issue:      issueNumber,
		Title:      title,
		State:      state,
		LeadTime:   metricToJSON(m),
		Warnings:   warnings,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WriteCycleTimeJSON writes cycle-time metrics for an issue as JSON to the writer.
func WriteCycleTimeJSON(w io.Writer, repo string, issueNumber int, title, state string, commits int, m model.Metric, warnings []string) error {
	out := JSONCycleTimeOutput{
		Repository: repo,
		Issue:      issueNumber,
		Title:      title,
		State:      state,
		Commits:    commits,
		CycleTime:  metricToJSON(m),
		Warnings:   warnings,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WriteCycleTimePRJSON writes cycle-time metrics for a PR as JSON.
func WriteCycleTimePRJSON(w io.Writer, repo string, prNumber int, title, state string, m model.Metric, warnings []string) error {
	out := JSONCycleTimeOutput{
		Repository: repo,
		PR:         prNumber,
		Title:      title,
		State:      state,
		CycleTime:  metricToJSON(m),
		Warnings:   warnings,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WriteReleaseJSON writes release metrics as JSON to the writer.
func WriteReleaseJSON(w io.Writer, repo string, rm model.ReleaseMetrics, warnings []string) error {
	comp := JSONComposition{
		TotalIssues:    rm.TotalIssues,
		CategoryCounts: rm.CategoryCounts,
		CategoryRatios: rm.CategoryRatios,
	}

	out := JSONReleaseOutput{
		Repository:  repo,
		Tag:         rm.Tag,
		PreviousTag: rm.PreviousTag,
		Date:        rm.Date,
		IsHotfix:    rm.IsHotfix,
		Composition: comp,
		Aggregates: JSONAggregates{
			LeadTime:   statsToJSON(rm.LeadTimeStats),
			CycleTime:  statsToJSON(rm.CycleTimeStats),
			ReleaseLag: statsToJSON(rm.ReleaseLagStats),
		},
		Warnings: warnings,
	}

	if rm.Cadence != nil {
		h := rm.Cadence.Hours()
		out.CadenceHours = &h
	}

	for _, im := range rm.Issues {
		out.Issues = append(out.Issues, JSONIssueMetrics{
			Number:           im.Issue.Number,
			Title:            im.Issue.Title,
			Category:         im.Category,
			LeadTime:         metricToJSON(im.LeadTime),
			CycleTime:        metricToJSON(im.CycleTime),
			ReleaseLag:       metricToJSON(im.ReleaseLag),
			CommitCount:      im.CommitCount,
			LeadTimeOutlier:  im.LeadTimeOutlier,
			CycleTimeOutlier: im.CycleTimeOutlier,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func durationToSeconds(d *time.Duration) *int64 {
	if d == nil {
		return nil
	}
	s := int64(d.Seconds())
	return &s
}

func statsToJSON(s model.Stats) JSONStats {
	return JSONStats{
		Count:                s.Count,
		MeanSeconds:          durationToSeconds(s.Mean),
		MedianSeconds:        durationToSeconds(s.Median),
		StdDevSeconds:        durationToSeconds(s.StdDev),
		P90Seconds:           durationToSeconds(s.P90),
		P95Seconds:           durationToSeconds(s.P95),
		OutlierCutoffSeconds: durationToSeconds(s.OutlierCutoff),
		OutlierCount:         s.OutlierCount,
	}
}
