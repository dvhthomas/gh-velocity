package metrics

import (
	"slices"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// ReleaseComposition computes the bug/feature/other ratio for a set of issues.
func ReleaseComposition(issues []model.Issue, bugLabels, featureLabels []string) (bugRatio, featureRatio, otherRatio float64) {
	if len(issues) == 0 {
		return 0, 0, 0
	}

	var bugs, features int
	for _, issue := range issues {
		if hasAnyLabel(issue.Labels, bugLabels) {
			bugs++
		} else if hasAnyLabel(issue.Labels, featureLabels) {
			features++
		}
	}

	total := float64(len(issues))
	others := len(issues) - bugs - features
	return float64(bugs) / total, float64(features) / total, float64(others) / total
}

// IsHotfix returns true if the release is a patch release within the hotfix window.
func IsHotfix(current, previous model.Release, hotfixWindowHours float64) bool {
	if previous.TagName == "" {
		return false
	}
	window := time.Duration(hotfixWindowHours * float64(time.Hour))
	return current.CreatedAt.Sub(previous.CreatedAt) <= window
}

func hasAnyLabel(issueLabels, targetLabels []string) bool {
	for _, label := range issueLabels {
		if slices.Contains(targetLabels, label) {
			return true
		}
	}
	return false
}
