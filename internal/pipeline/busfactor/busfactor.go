// Package busfactor implements the bus-factor metric pipeline.
// It analyzes local git history to identify directories where knowledge
// is concentrated in one or two contributors.
package busfactor

import (
	"context"
	"sort"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/git"
)

// RiskLevel indicates knowledge concentration risk.
type RiskLevel string

const (
	RiskHigh   RiskLevel = "HIGH"   // 1 contributor
	RiskMedium RiskLevel = "MEDIUM" // 2 contributors, primary >70%
	RiskLow    RiskLevel = "LOW"    // 3+ contributors, distributed
)

// PathRisk holds bus factor analysis for a single directory.
type PathRisk struct {
	Path             string
	Risk             RiskLevel
	ContributorCount int
	Primary          git.Contributor
	PrimaryPct       float64
	TotalCommits     int
}

// Result holds the complete bus factor analysis.
type Result struct {
	Repository string // "owner/repo"
	Paths      []PathRisk
	Since      time.Time
	Depth      int
	MinCommits int
}

// Compute computes bus factor risk from contributor data.
// Paths are sorted: HIGH first, then MEDIUM, then LOW, then by path name.
func Compute(paths []git.PathContributors, since time.Time, depth, minCommits int) Result {
	var risks []PathRisk
	for _, p := range paths {
		sorted := make([]git.Contributor, len(p.Contributors))
		copy(sorted, p.Contributors)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Commits > sorted[j].Commits
		})

		primary := sorted[0]
		pct := float64(primary.Commits) / float64(p.TotalCommits) * 100

		risk := classifyRisk(len(sorted), pct)

		risks = append(risks, PathRisk{
			Path:             p.Path,
			Risk:             risk,
			ContributorCount: len(sorted),
			Primary:          primary,
			PrimaryPct:       pct,
			TotalCommits:     p.TotalCommits,
		})
	}

	sort.Slice(risks, func(i, j int) bool {
		ri, rj := riskOrder(risks[i].Risk), riskOrder(risks[j].Risk)
		if ri != rj {
			return ri < rj
		}
		return risks[i].Path < risks[j].Path
	})

	return Result{
		Paths:      risks,
		Since:      since,
		Depth:      depth,
		MinCommits: minCommits,
	}
}

func classifyRisk(contributorCount int, primaryPct float64) RiskLevel {
	switch {
	case contributorCount == 1:
		return RiskHigh
	case contributorCount == 2 && primaryPct > 70:
		return RiskMedium
	default:
		return RiskLow
	}
}

func riskOrder(r RiskLevel) int {
	switch r {
	case RiskHigh:
		return 0
	case RiskMedium:
		return 1
	default:
		return 2
	}
}

// Pipeline implements pipeline.Pipeline for the bus-factor command.
type Pipeline struct {
	// Constructor params
	Repository string
	WorkDir    string
	Since      time.Time
	Depth      int
	MinCommits int

	// GatherData output
	paths []git.PathContributors

	// ProcessData output
	Result Result
}

// GatherData fetches contributor data from local git history.
func (p *Pipeline) GatherData(ctx context.Context) error {
	runner := git.NewRunner(p.WorkDir)
	paths, err := runner.ContributorsByPath(ctx, p.Since, p.Depth, p.MinCommits)
	if err != nil {
		return err
	}
	p.paths = paths
	return nil
}

// ProcessData computes bus factor risk from gathered contributor data.
func (p *Pipeline) ProcessData() error {
	p.Result = Compute(p.paths, p.Since, p.Depth, p.MinCommits)
	p.Result.Repository = p.Repository
	return nil
}

// Render writes the bus factor result in the requested format.
func (p *Pipeline) Render(rc format.RenderContext) error {
	switch rc.Format {
	case format.JSON:
		return WriteJSON(rc.Writer, p.Result)
	case format.Markdown:
		return WriteMarkdown(rc, p.Result)
	default:
		return WritePretty(rc, p.Result)
	}
}
