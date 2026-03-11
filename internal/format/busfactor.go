package format

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/metrics"
)

// WriteBusFactorPretty writes bus factor results as formatted text.
func WriteBusFactorPretty(rc RenderContext, result metrics.BusFactorResult) error {
	w := rc.Writer
	days := int(math.Round(time.Since(result.Since).Hours() / 24))
	fmt.Fprintf(w, "Knowledge Risk Report (last %d days, depth %d)\n\n", days, result.Depth)

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

// WriteBusFactorMarkdown writes bus factor results as markdown.
func WriteBusFactorMarkdown(rc RenderContext, result metrics.BusFactorResult) error {
	w := rc.Writer
	days := int(math.Round(time.Since(result.Since).Hours() / 24))
	fmt.Fprintf(w, "## Knowledge Risk Report (last %d days, depth %d)\n\n", days, result.Depth)

	if len(result.Paths) == 0 {
		fmt.Fprintln(w, "_No paths with enough commit activity._")
		return nil
	}

	fmt.Fprintln(w, "| Risk | Path | Contributors | Primary | Commits |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- |")
	for _, p := range result.Paths {
		primary := formatPrimary(p)
		fmt.Fprintf(w, "| %s | %s | %d | %s | %d |\n",
			string(p.Risk), p.Path, p.ContributorCount, primary, p.TotalCommits)
	}

	fmt.Fprintf(w, "\n%s\n", formatRiskSummary(result))
	return nil
}

type jsonBusFactorOutput struct {
	Since string           `json:"since"`
	Depth int              `json:"depth"`
	Paths []jsonPathRisk   `json:"paths"`
	Summary jsonRiskSummary `json:"summary"`
}

type jsonPathRisk struct {
	Path             string              `json:"path"`
	Risk             string              `json:"risk"`
	ContributorCount int                 `json:"contributor_count"`
	Primary          jsonContributor     `json:"primary"`
	PrimaryPct       float64             `json:"primary_pct"`
	TotalCommits     int                 `json:"total_commits"`
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

// WriteBusFactorJSON writes bus factor results as JSON.
func WriteBusFactorJSON(w io.Writer, result metrics.BusFactorResult) error {
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

	out := jsonBusFactorOutput{
		Since: result.Since.UTC().Format(time.RFC3339),
		Depth: result.Depth,
		Paths: paths,
		Summary: jsonRiskSummary{
			High:   high,
			Medium: med,
			Low:    low,
		},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func formatPrimary(p metrics.PathRisk) string {
	if p.ContributorCount >= 3 && p.PrimaryPct <= 70 {
		return "distributed"
	}
	return fmt.Sprintf("%s (%.0f%%)", p.Primary.Name, p.PrimaryPct)
}

func countRisks(result metrics.BusFactorResult) (high, medium, low int) {
	for _, p := range result.Paths {
		switch p.Risk {
		case metrics.RiskHigh:
			high++
		case metrics.RiskMedium:
			medium++
		case metrics.RiskLow:
			low++
		}
	}
	return
}

func formatRiskSummary(result metrics.BusFactorResult) string {
	high, med, low := countRisks(result)
	return fmt.Sprintf("%d HIGH risk, %d MEDIUM risk, %d LOW risk areas", high, med, low)
}
