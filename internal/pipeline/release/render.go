package release

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

//go:embed templates/*.md.tmpl
var templateFS embed.FS

var markdownTmpl = template.Must(
	template.New("release.md.tmpl").Funcs(format.TemplateFuncMap()).ParseFS(templateFS, "templates/release.md.tmpl"),
)

// ============================================================
// JSON
// ============================================================

type jsonReleaseOutput struct {
	Repository   string             `json:"repository"`
	Tag          string             `json:"tag"`
	PreviousTag  string             `json:"previous_tag,omitempty"`
	Date         time.Time          `json:"date"`
	CadenceHours *float64           `json:"cadence_hours,omitempty"`
	IsHotfix     bool               `json:"is_hotfix"`
	Composition  jsonComposition    `json:"composition"`
	Issues       []jsonIssueMetrics `json:"issues"`
	Aggregates   jsonAggregates     `json:"aggregates"`
	Warnings     []string           `json:"warnings,omitempty"`
}

type jsonComposition struct {
	TotalIssues int                       `json:"total_issues"`
	Categories  []jsonCategoryComposition `json:"categories"`
}

type jsonCategoryComposition struct {
	Name  string  `json:"name"`
	Count int     `json:"count"`
	Ratio float64 `json:"ratio"`
}

type jsonIssueMetrics struct {
	Number           int               `json:"number"`
	Title            string            `json:"title"`
	URL              string            `json:"url,omitempty"`
	Labels           []string          `json:"labels,omitempty"`
	Category         string            `json:"category"`
	LeadTime         format.JSONMetric `json:"lead_time"`
	CycleTime        format.JSONMetric `json:"cycle_time"`
	ReleaseLag       format.JSONMetric `json:"release_lag"`
	CommitCount      int               `json:"commit_count"`
	LeadTimeOutlier  bool              `json:"lead_time_outlier,omitempty"`
	CycleTimeOutlier bool              `json:"cycle_time_outlier,omitempty"`
}

type jsonAggregates struct {
	LeadTime   format.JSONStats `json:"lead_time"`
	CycleTime  format.JSONStats `json:"cycle_time"`
	ReleaseLag format.JSONStats `json:"release_lag"`
}

// WriteJSON writes release metrics as JSON to the writer.
func WriteJSON(w io.Writer, repo string, rm model.ReleaseMetrics, warnings []string) error {
	comp := jsonComposition{TotalIssues: rm.TotalIssues}
	for _, name := range rm.CategoryNames {
		comp.Categories = append(comp.Categories, jsonCategoryComposition{
			Name:  name,
			Count: rm.CategoryCounts[name],
			Ratio: rm.CategoryRatios[name],
		})
	}

	out := jsonReleaseOutput{
		Repository:  repo,
		Tag:         rm.Tag,
		PreviousTag: rm.PreviousTag,
		Date:        rm.Date.UTC(),
		IsHotfix:    rm.IsHotfix,
		Composition: comp,
		Issues:      make([]jsonIssueMetrics, 0, len(rm.Issues)),
		Aggregates: jsonAggregates{
			LeadTime:   format.StatsToJSON(rm.LeadTimeStats),
			CycleTime:  format.StatsToJSON(rm.CycleTimeStats),
			ReleaseLag: format.StatsToJSON(rm.ReleaseLagStats),
		},
		Warnings: warnings,
	}

	if rm.Cadence != nil {
		h := rm.Cadence.Hours()
		out.CadenceHours = &h
	}

	for _, im := range rm.Issues {
		out.Issues = append(out.Issues, jsonIssueMetrics{
			Number:           im.Issue.Number,
			Title:            im.Issue.Title,
			URL:              im.Issue.URL,
			Labels:           im.Issue.Labels,
			Category:         im.Category,
			LeadTime:         format.MetricToJSON(im.LeadTime),
			CycleTime:        format.MetricToJSON(im.CycleTime),
			ReleaseLag:       format.MetricToJSON(im.ReleaseLag),
			CommitCount:      im.CommitCount,
			LeadTimeOutlier:  im.LeadTimeOutlier,
			CycleTimeOutlier: im.CycleTimeOutlier,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// ============================================================
// Markdown
// ============================================================

type markdownTemplateData struct {
	Tag         string
	PreviousTag string
	Cadence     string
	IsHotfix    bool
	Categories  []categoryRow
	TotalIssues int
	Issues      []issueRow
	LeadTime    string
	CycleTime   string
	ReleaseLag  string
	Warnings    []string
}

type categoryRow struct {
	Name  string
	Count int
	Ratio string
}

type issueRow struct {
	Link      string
	Title     string
	Labels    string
	LeadTime  string
	CycleTime string
	RelLag    string
	Commits   int
	Flag      string
}

// WriteMarkdown writes release metrics as a markdown table using an embedded template.
func WriteMarkdown(rc format.RenderContext, rm model.ReleaseMetrics, warnings []string) error {
	data := markdownTemplateData{
		Tag:         rm.Tag,
		PreviousTag: rm.PreviousTag,
		Cadence:     format.FormatDurationPtr(rm.Cadence),
		IsHotfix:    rm.IsHotfix,
		TotalIssues: rm.TotalIssues,
	}
	for _, name := range rm.CategoryNames {
		label := strings.ToUpper(name[:1]) + name[1:]
		data.Categories = append(data.Categories, categoryRow{
			Name:  label,
			Count: rm.CategoryCounts[name],
			Ratio: fmt.Sprintf("%.0f%%", rm.CategoryRatios[name]*100),
		})
	}
	for _, im := range rm.Issues {
		flag := ""
		if im.LeadTimeOutlier || im.CycleTimeOutlier {
			flag = "OUTLIER"
		}
		data.Issues = append(data.Issues, issueRow{
			Link:      format.FormatItemLink(im.Issue.Number, im.Issue.URL, rc),
			Title:     format.SanitizeMarkdown(im.Issue.Title),
			Labels:    format.FormatLabels(im.Issue.Labels),
			LeadTime:  format.FormatDurationPtr(im.LeadTime.Duration),
			CycleTime: format.FormatDurationPtr(im.CycleTime.Duration),
			RelLag:    format.FormatDurationPtr(im.ReleaseLag.Duration),
			Commits:   im.CommitCount,
			Flag:      flag,
		})
	}
	data.LeadTime = formatStatsRow(rm.LeadTimeStats)
	data.CycleTime = formatStatsRow(rm.CycleTimeStats)
	data.ReleaseLag = formatStatsRow(rm.ReleaseLagStats)
	data.Warnings = warnings
	return markdownTmpl.Execute(rc.Writer, data)
}

func formatStatsRow(s model.Stats) string {
	sd, p90, p95, outliers := "--", "--", "--", "--"
	if s.StdDev != nil {
		sd = format.FormatDuration(*s.StdDev)
	}
	if s.P90 != nil {
		p90 = format.FormatDuration(*s.P90)
	}
	if s.P95 != nil {
		p95 = format.FormatDuration(*s.P95)
	}
	if s.OutlierCutoff != nil {
		outliers = fmt.Sprintf("%d", s.OutlierCount)
	}
	return fmt.Sprintf("| %s | %s | %s | %s | %s | %s |",
		format.FormatDurationPtr(s.Mean), format.FormatDurationPtr(s.Median), sd, p90, p95, outliers)
}

// ============================================================
// Pretty
// ============================================================

// WritePretty writes release metrics as a formatted table to the writer.
func WritePretty(rc format.RenderContext, rm model.ReleaseMetrics, warnings []string) error {
	w := rc.Writer
	fmt.Fprintf(w, "Release %s\n", rm.Tag)
	fmt.Fprintln(w, strings.Repeat("=", 60))
	fmt.Fprintln(w)

	if rm.PreviousTag != "" {
		fmt.Fprintf(w, "  Previous:  %s\n", rm.PreviousTag)
		fmt.Fprintf(w, "  Cadence:   %s\n", format.FormatDurationPtr(rm.Cadence))
		if rm.IsHotfix {
			fmt.Fprintln(w, "  ** HOTFIX RELEASE **")
		}
		fmt.Fprintln(w)
	}

	// Composition
	fmt.Fprintln(w, "Composition")
	for _, name := range rm.CategoryNames {
		label := strings.ToUpper(name[:1]) + name[1:] + ":"
		fmt.Fprintf(w, "  %-10s %d (%.0f%%)\n", label, rm.CategoryCounts[name], rm.CategoryRatios[name]*100)
	}
	fmt.Fprintf(w, "  Total:     %d\n\n", rm.TotalIssues)

	// Per-issue table
	if len(rm.Issues) > 0 {
		fmt.Fprintln(w, "Issues")
		tp := format.NewTable(w, rc.IsTTY, rc.Width)
		tp.AddHeader([]string{"#", "Title", "Labels", "Lead Time", "Cycle Time", "Rel. Lag", "Commits", ""})
		for _, im := range rm.Issues {
			flag := ""
			if im.LeadTimeOutlier || im.CycleTimeOutlier {
				flag = "OUTLIER"
			}
			tp.AddField(format.FormatItemLink(im.Issue.Number, im.Issue.URL, rc))
			tp.AddField(im.Issue.Title)
			tp.AddField(format.FormatLabels(im.Issue.Labels))
			tp.AddField(format.FormatDurationPtr(im.LeadTime.Duration))
			tp.AddField(format.FormatDurationPtr(im.CycleTime.Duration))
			tp.AddField(format.FormatDurationPtr(im.ReleaseLag.Duration))
			tp.AddField(fmt.Sprintf("%d", im.CommitCount))
			tp.AddField(flag)
			tp.EndRow()
		}
		if err := tp.Render(); err != nil {
			return err
		}
		fmt.Fprintln(w)
	}

	// Aggregates
	fmt.Fprintln(w, "Aggregates")
	tp := format.NewTable(w, rc.IsTTY, rc.Width)
	tp.AddHeader([]string{"Metric", "Mean", "Median", "Std Dev", "P90", "P95", "Outliers"})
	writePrettyStatsRow(tp, "Lead Time", rm.LeadTimeStats)
	writePrettyStatsRow(tp, "Cycle Time", rm.CycleTimeStats)
	writePrettyStatsRow(tp, "Release Lag", rm.ReleaseLagStats)
	if err := tp.Render(); err != nil {
		return err
	}

	// Warnings
	if len(warnings) > 0 {
		fmt.Fprintln(w, "\nWarnings:")
		for _, warn := range warnings {
			fmt.Fprintf(w, "  ! %s\n", warn)
		}
	}

	return nil
}

func writePrettyStatsRow(tp tableprinter.TablePrinter, name string, s model.Stats) {
	sd := "--"
	if s.StdDev != nil {
		sd = format.FormatDuration(*s.StdDev)
	}
	p90 := "--"
	if s.P90 != nil {
		p90 = format.FormatDuration(*s.P90)
	}
	p95 := "--"
	if s.P95 != nil {
		p95 = format.FormatDuration(*s.P95)
	}
	outliers := "--"
	if s.OutlierCutoff != nil {
		outliers = fmt.Sprintf("%d", s.OutlierCount)
	}
	tp.AddField(name)
	tp.AddField(format.FormatDurationPtr(s.Mean))
	tp.AddField(format.FormatDurationPtr(s.Median))
	tp.AddField(sd)
	tp.AddField(p90)
	tp.AddField(p95)
	tp.AddField(outliers)
	tp.EndRow()
}
