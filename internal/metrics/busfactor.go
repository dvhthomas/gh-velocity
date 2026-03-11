package metrics

import (
	"sort"
	"time"

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

// BusFactorResult holds the complete bus factor analysis.
type BusFactorResult struct {
	Paths []PathRisk
	Since time.Time
	Depth int
}

// ComputeBusFactor computes bus factor risk from contributor data.
// Paths are sorted: HIGH first, then MEDIUM, then LOW, then by path name.
func ComputeBusFactor(paths []git.PathContributors, since time.Time, depth int) BusFactorResult {
	var risks []PathRisk
	for _, p := range paths {
		// Sort contributors by commit count descending.
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

	// Sort: HIGH > MEDIUM > LOW, then alphabetically by path.
	sort.Slice(risks, func(i, j int) bool {
		ri, rj := riskOrder(risks[i].Risk), riskOrder(risks[j].Risk)
		if ri != rj {
			return ri < rj
		}
		return risks[i].Path < risks[j].Path
	})

	return BusFactorResult{
		Paths: risks,
		Since: since,
		Depth: depth,
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
