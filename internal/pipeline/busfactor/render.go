package busfactor

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"text/template"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
)

//go:embed templates/*.md.tmpl
var templateFS embed.FS

var markdownTmpl = template.Must(
	template.New("busfactor.md.tmpl").Funcs(busfactorFuncMap()).ParseFS(templateFS, "templates/busfactor.md.tmpl"),
)

func busfactorFuncMap() template.FuncMap {
	fm := format.TemplateFuncMap()
	fm["primary"] = formatPrimary
	return fm
}

// --- Pretty ---

// WritePretty writes bus factor results as formatted text.
func WritePretty(rc format.RenderContext, result Result) error {
	w := rc.Writer
	days := int(math.Round(time.Since(result.Since).Hours() / 24))
	fmt.Fprintf(w, "Knowledge Risk Report: %s (last %d days, depth %d, min-commits %d)\n\n",
		result.Repository, days, result.Depth, result.MinCommits)

	if len(result.Paths) == 0 {
		fmt.Fprintln(w, "  No paths with enough commit activity.")
		return nil
	}

	fmt.Fprintf(w, "%-8s %-30s %-15s %-20s %s\n", "Risk", "Path", "Contributors", "Primary", "Commits")
	for _, p := range result.Paths {
		primary := formatPrimary(p)
		fmt.Fprintf(w, "%-8s %-30s %-15d %-20s %d\n",
			string(p.Risk), p.Path, p.ContributorCount, primary, p.TotalCommits)
	}

	fmt.Fprintf(w, "\nSummary: %s\n", formatRiskSummary(result))
	return nil
}

// --- Markdown ---

type templateData struct {
	Repository  string
	Since       time.Time
	Depth       int
	MinCommits  int
	Days        int
	Paths       []PathRisk
	SummaryLine string
}

// WriteMarkdown writes bus factor results as markdown.
func WriteMarkdown(rc format.RenderContext, result Result) error {
	days := int(math.Round(time.Since(result.Since).Hours() / 24))
	high, med, low := countRisks(result)

	data := templateData{
		Repository:  result.Repository,
		Since:       result.Since,
		Depth:       result.Depth,
		MinCommits:  result.MinCommits,
		Days:        days,
		Paths:       result.Paths,
		SummaryLine: fmt.Sprintf("%d HIGH risk, %d MEDIUM risk, %d LOW risk areas", high, med, low),
	}

	return markdownTmpl.Execute(rc.Writer, data)
}

// --- JSON ---

type jsonOutput struct {
	Repository string          `json:"repository"`
	Since      string          `json:"since"`
	Depth      int             `json:"depth"`
	MinCommits int             `json:"min_commits"`
	Paths      []jsonPathRisk  `json:"paths"`
	Summary    jsonRiskSummary `json:"summary"`
	Warnings   []string        `json:"warnings,omitempty"`
}

type jsonPathRisk struct {
	Path             string          `json:"path"`
	Risk             string          `json:"risk"`
	ContributorCount int             `json:"contributor_count"`
	Primary          jsonContributor `json:"primary"`
	PrimaryPct       float64         `json:"primary_pct"`
	TotalCommits     int             `json:"total_commits"`
}

type jsonContributor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type jsonRiskSummary struct {
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
}

// WriteJSON writes bus factor results as JSON.
func WriteJSON(w io.Writer, result Result, warnings []string) error {
	high, med, low := countRisks(result)

	paths := make([]jsonPathRisk, len(result.Paths))
	for i, p := range result.Paths {
		paths[i] = jsonPathRisk{
			Path:             p.Path,
			Risk:             string(p.Risk),
			ContributorCount: p.ContributorCount,
			Primary: jsonContributor{
				Name:  p.Primary.Name,
				Email: p.Primary.Email,
			},
			PrimaryPct:   math.Round(p.PrimaryPct*10) / 10,
			TotalCommits: p.TotalCommits,
		}
	}

	out := jsonOutput{
		Repository: result.Repository,
		Since:      result.Since.UTC().Format(time.RFC3339),
		Depth:      result.Depth,
		MinCommits: result.MinCommits,
		Paths:      paths,
		Summary: jsonRiskSummary{
			High:   high,
			Medium: med,
			Low:    low,
		},
		Warnings: warnings,
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- Helpers ---

func formatPrimary(p PathRisk) string {
	if p.ContributorCount >= 3 && p.PrimaryPct <= 70 {
		return "distributed"
	}
	return fmt.Sprintf("%s (%.0f%%)", p.Primary.Name, p.PrimaryPct)
}

func countRisks(result Result) (high, medium, low int) {
	for _, p := range result.Paths {
		switch p.Risk {
		case RiskHigh:
			high++
		case RiskMedium:
			medium++
		case RiskLow:
			low++
		}
	}
	return
}

func formatRiskSummary(result Result) string {
	high, med, low := countRisks(result)
	return fmt.Sprintf("%d HIGH risk, %d MEDIUM risk, %d LOW risk areas", high, med, low)
}
