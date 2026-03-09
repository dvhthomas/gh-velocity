package format

import (
	"encoding/json"
	"io"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// JSONLeadTimeOutput is the JSON representation of lead-time metrics.
type JSONLeadTimeOutput struct {
	Repository      string   `json:"repository"`
	Issue           int      `json:"issue"`
	Title           string   `json:"title"`
	State           string   `json:"state"`
	LeadTimeSeconds *int64   `json:"lead_time_seconds"`
	LeadTime        string   `json:"lead_time"`
	Warnings        []string `json:"warnings,omitempty"`
}

// JSONCycleTimeOutput is the JSON representation of cycle-time metrics.
type JSONCycleTimeOutput struct {
	Repository       string   `json:"repository"`
	Issue            int      `json:"issue"`
	Title            string   `json:"title"`
	State            string   `json:"state"`
	Commits          int      `json:"commits"`
	CycleTimeSeconds *int64   `json:"cycle_time_seconds"`
	CycleTime        string   `json:"cycle_time"`
	Warnings         []string `json:"warnings,omitempty"`
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
	TotalIssues  int     `json:"total_issues"`
	BugCount     int     `json:"bug_count"`
	FeatureCount int     `json:"feature_count"`
	OtherCount   int     `json:"other_count"`
	BugRatio     float64 `json:"bug_ratio"`
	FeatureRatio float64 `json:"feature_ratio"`
	OtherRatio   float64 `json:"other_ratio"`
}

type JSONIssueMetrics struct {
	Number            int    `json:"number"`
	Title             string `json:"title"`
	LeadTimeSeconds   *int64 `json:"lead_time_seconds"`
	CycleTimeSeconds  *int64 `json:"cycle_time_seconds"`
	ReleaseLagSeconds *int64 `json:"release_lag_seconds"`
	CommitCount       int    `json:"commit_count"`
	LeadTimeOutlier   bool   `json:"lead_time_outlier,omitempty"`
	CycleTimeOutlier  bool   `json:"cycle_time_outlier,omitempty"`
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
func WriteLeadTimeJSON(w io.Writer, repo string, issueNumber int, title, state string, lt *time.Duration, warnings []string) error {
	out := JSONLeadTimeOutput{
		Repository: repo,
		Issue:      issueNumber,
		Title:      title,
		State:      state,
		Warnings:   warnings,
	}
	if lt != nil {
		s := int64(lt.Seconds())
		out.LeadTimeSeconds = &s
		out.LeadTime = FormatDuration(*lt)
	} else {
		out.LeadTime = "N/A"
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WriteCycleTimeJSON writes cycle-time metrics as JSON to the writer.
func WriteCycleTimeJSON(w io.Writer, repo string, issueNumber int, title, state string, commits int, ct *time.Duration, warnings []string) error {
	out := JSONCycleTimeOutput{
		Repository: repo,
		Issue:      issueNumber,
		Title:      title,
		State:      state,
		Commits:    commits,
		Warnings:   warnings,
	}
	if ct != nil {
		s := int64(ct.Seconds())
		out.CycleTimeSeconds = &s
		out.CycleTime = FormatDuration(*ct)
	} else {
		out.CycleTime = "N/A"
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WriteReleaseJSON writes release metrics as JSON to the writer.
func WriteReleaseJSON(w io.Writer, repo string, rm model.ReleaseMetrics, warnings []string) error {
	out := JSONReleaseOutput{
		Repository:  repo,
		Tag:         rm.Tag,
		PreviousTag: rm.PreviousTag,
		Date:        rm.Date,
		IsHotfix:    rm.IsHotfix,
		Composition: JSONComposition{
			TotalIssues:  rm.TotalIssues,
			BugCount:     rm.BugCount,
			FeatureCount: rm.FeatureCount,
			OtherCount:   rm.OtherCount,
			BugRatio:     rm.BugRatio,
			FeatureRatio: rm.FeatureRatio,
			OtherRatio:   rm.OtherRatio,
		},
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
			Number:            im.Issue.Number,
			Title:             im.Issue.Title,
			LeadTimeSeconds:   durationToSeconds(im.LeadTime),
			CycleTimeSeconds:  durationToSeconds(im.CycleTime),
			ReleaseLagSeconds: durationToSeconds(im.ReleaseLag),
			CommitCount:       im.CommitCount,
			LeadTimeOutlier:   im.LeadTimeOutlier,
			CycleTimeOutlier:  im.CycleTimeOutlier,
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
