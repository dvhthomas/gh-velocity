package format

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// BulkLeadTimeItem holds a single issue's lead time result for bulk output.
type BulkLeadTimeItem struct {
	Issue  model.Issue
	Metric model.Metric
}

// --- JSON ---

type jsonBulkLeadTimeOutput struct {
	Repository string             `json:"repository"`
	Window     jsonWindow         `json:"window"`
	Items      []jsonBulkLeadItem `json:"items"`
	Stats      JSONStats          `json:"stats"`
	Capped     bool               `json:"capped,omitempty"`
}

type jsonWindow struct {
	Since string `json:"since"`
	Until string `json:"until"`
}

type jsonBulkLeadItem struct {
	Number   int        `json:"number"`
	Title    string     `json:"title"`
	LeadTime JSONMetric `json:"lead_time"`
}

// WriteLeadTimeBulkJSON writes bulk lead-time results as JSON.
func WriteLeadTimeBulkJSON(w io.Writer, repo string, since, until time.Time, items []BulkLeadTimeItem, stats model.Stats) error {
	out := jsonBulkLeadTimeOutput{
		Repository: repo,
		Window: jsonWindow{
			Since: since.UTC().Format(time.RFC3339),
			Until: until.UTC().Format(time.RFC3339),
		},
		Stats:  statsToJSON(stats),
		Capped: len(items) >= 1000,
	}

	for _, item := range items {
		out.Items = append(out.Items, jsonBulkLeadItem{
			Number:   item.Issue.Number,
			Title:    item.Issue.Title,
			LeadTime: metricToJSON(item.Metric),
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- Markdown ---

// WriteLeadTimeBulkMarkdown writes bulk lead-time results as a markdown table.
func WriteLeadTimeBulkMarkdown(w io.Writer, repo string, since, until time.Time, items []BulkLeadTimeItem, stats model.Stats) error {
	sorted := sortByCloseDateDesc(items)

	fmt.Fprintf(w, "## Lead Time: %s (%s – %s UTC)\n\n",
		repo, since.UTC().Format(time.DateOnly), until.UTC().Format(time.DateOnly))

	fmt.Fprintf(w, "| Issue | Title | Created (UTC) | Closed (UTC) | Lead Time |\n")
	fmt.Fprintf(w, "| ---: | --- | --- | --- | --- |\n")
	for _, item := range sorted {
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		fmt.Fprintf(w, "| #%d | %s | %s | %s | %s |\n",
			item.Issue.Number,
			sanitizeMarkdown(item.Issue.Title),
			item.Issue.CreatedAt.UTC().Format(time.DateOnly),
			closedStr,
			FormatMetricDuration(item.Metric),
		)
	}

	fmt.Fprintf(w, "\n**Summary:** %s\n", formatStatsSummary(stats))
	return nil
}

// --- Pretty ---

// WriteLeadTimeBulkPretty writes bulk lead-time results as a formatted table.
func WriteLeadTimeBulkPretty(w io.Writer, isTTY bool, width int, repo string, since, until time.Time, items []BulkLeadTimeItem, stats model.Stats) error {
	sorted := sortByCloseDateDesc(items)

	fmt.Fprintf(w, "Lead Time: %s (%s – %s UTC)\n\n",
		repo, since.UTC().Format(time.DateOnly), until.UTC().Format(time.DateOnly))

	tp := NewTable(w, isTTY, width)
	tp.AddHeader([]string{"#", "Title", "Created", "Closed", "Lead Time"})
	for _, item := range sorted {
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		tp.AddField(fmt.Sprintf("%d", item.Issue.Number))
		tp.AddField(item.Issue.Title)
		tp.AddField(item.Issue.CreatedAt.UTC().Format(time.DateOnly))
		tp.AddField(closedStr)
		tp.AddField(FormatMetricDuration(item.Metric))
		tp.EndRow()
	}
	if err := tp.Render(); err != nil {
		return err
	}

	fmt.Fprintf(w, "\n%s\n", formatStatsSummary(stats))
	return nil
}

// ============================================================
// Cycle Time Bulk
// ============================================================

// BulkCycleTimeItem holds a single issue's cycle time result for bulk output.
type BulkCycleTimeItem struct {
	Issue  model.Issue
	Metric model.Metric
}

// --- Cycle Time JSON ---

type jsonBulkCycleTimeOutput struct {
	Repository string              `json:"repository"`
	Window     jsonWindow          `json:"window"`
	Strategy   string              `json:"strategy"`
	Items      []jsonBulkCycleItem `json:"items"`
	Stats      JSONStats           `json:"stats"`
	Capped     bool                `json:"capped,omitempty"`
}

type jsonBulkCycleItem struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	CycleTime JSONMetric `json:"cycle_time"`
}

// WriteCycleTimeBulkJSON writes bulk cycle-time results as JSON.
func WriteCycleTimeBulkJSON(w io.Writer, repo string, since, until time.Time, strategy string, items []BulkCycleTimeItem, stats model.Stats) error {
	out := jsonBulkCycleTimeOutput{
		Repository: repo,
		Window: jsonWindow{
			Since: since.UTC().Format(time.RFC3339),
			Until: until.UTC().Format(time.RFC3339),
		},
		Strategy: strategy,
		Stats:    statsToJSON(stats),
		Capped:   len(items) >= 1000,
	}

	for _, item := range items {
		out.Items = append(out.Items, jsonBulkCycleItem{
			Number:    item.Issue.Number,
			Title:     item.Issue.Title,
			CycleTime: metricToJSON(item.Metric),
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- Cycle Time Markdown ---

// WriteCycleTimeBulkMarkdown writes bulk cycle-time results as a markdown table.
func WriteCycleTimeBulkMarkdown(w io.Writer, repo string, since, until time.Time, strategy string, items []BulkCycleTimeItem, stats model.Stats) error {
	sorted := sortCycleByCloseDateDesc(items)

	fmt.Fprintf(w, "## Cycle Time: %s (%s – %s UTC) [%s]\n\n",
		repo, since.UTC().Format(time.DateOnly), until.UTC().Format(time.DateOnly), strategy)

	fmt.Fprintf(w, "| Issue | Title | Started (UTC) | Closed (UTC) | Cycle Time |\n")
	fmt.Fprintf(w, "| ---: | --- | --- | --- | --- |\n")
	for _, item := range sorted {
		startedStr := "N/A"
		if item.Metric.Start != nil {
			startedStr = item.Metric.Start.Time.UTC().Format(time.DateOnly)
		}
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		fmt.Fprintf(w, "| #%d | %s | %s | %s | %s |\n",
			item.Issue.Number,
			sanitizeMarkdown(item.Issue.Title),
			startedStr,
			closedStr,
			FormatMetricDuration(item.Metric),
		)
	}

	fmt.Fprintf(w, "\n**Summary:** %s\n", formatStatsSummary(stats))
	return nil
}

// --- Cycle Time Pretty ---

// WriteCycleTimeBulkPretty writes bulk cycle-time results as a formatted table.
func WriteCycleTimeBulkPretty(w io.Writer, isTTY bool, width int, repo string, since, until time.Time, strategy string, items []BulkCycleTimeItem, stats model.Stats) error {
	sorted := sortCycleByCloseDateDesc(items)

	fmt.Fprintf(w, "Cycle Time: %s (%s – %s UTC) [%s]\n\n",
		repo, since.UTC().Format(time.DateOnly), until.UTC().Format(time.DateOnly), strategy)

	tp := NewTable(w, isTTY, width)
	tp.AddHeader([]string{"#", "Title", "Started", "Closed", "Cycle Time"})
	for _, item := range sorted {
		startedStr := "N/A"
		if item.Metric.Start != nil {
			startedStr = item.Metric.Start.Time.UTC().Format(time.DateOnly)
		}
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		tp.AddField(fmt.Sprintf("%d", item.Issue.Number))
		tp.AddField(item.Issue.Title)
		tp.AddField(startedStr)
		tp.AddField(closedStr)
		tp.AddField(FormatMetricDuration(item.Metric))
		tp.EndRow()
	}
	if err := tp.Render(); err != nil {
		return err
	}

	fmt.Fprintf(w, "\n%s\n", formatStatsSummary(stats))
	return nil
}

func sortCycleByCloseDateDesc(items []BulkCycleTimeItem) []BulkCycleTimeItem {
	sorted := make([]BulkCycleTimeItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		ci := sorted[i].Issue.ClosedAt
		cj := sorted[j].Issue.ClosedAt
		if ci == nil && cj == nil {
			return false
		}
		if ci == nil {
			return false
		}
		if cj == nil {
			return true
		}
		return ci.After(*cj)
	})
	return sorted
}

// --- Helpers ---

func sortByCloseDateDesc(items []BulkLeadTimeItem) []BulkLeadTimeItem {
	sorted := make([]BulkLeadTimeItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		ci := sorted[i].Issue.ClosedAt
		cj := sorted[j].Issue.ClosedAt
		if ci == nil && cj == nil {
			return false
		}
		if ci == nil {
			return false
		}
		if cj == nil {
			return true
		}
		return ci.After(*cj)
	})
	return sorted
}

// formatStatsSummary returns a one-line stats summary.
func formatStatsSummary(stats model.Stats) string {
	if stats.Count == 0 {
		return "No items"
	}
	s := fmt.Sprintf("n=%d", stats.Count)
	if stats.Median != nil {
		s += fmt.Sprintf(", median %s", FormatDuration(*stats.Median))
	}
	if stats.Mean != nil {
		s += fmt.Sprintf(", mean %s", FormatDuration(*stats.Mean))
	}
	if stats.P90 != nil {
		s += fmt.Sprintf(", P90 %s", FormatDuration(*stats.P90))
	}
	return s
}
