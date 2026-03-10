package strategy

import (
	"sort"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// strategyPriority returns a numeric priority for a strategy name.
// Lower number = higher priority. pr-link wins over commit-ref wins over changelog.
func strategyPriority(name string) int {
	switch name {
	case "pr-link":
		return 0
	case "commit-ref":
		return 1
	case "changelog":
		return 2
	default:
		return 99
	}
}

// Merge deduplicates items across strategies using priority-based merge.
// When the same issue is found by multiple strategies, the higher-priority
// strategy's data wins (PR timestamps, linked PR info).
// PRs without linked issues are also included in the merged result.
func Merge(strategyResults []model.StrategyResult) []model.DiscoveredItem {
	// Sort strategy results by priority so higher-priority items are processed first.
	sorted := make([]model.StrategyResult, len(strategyResults))
	copy(sorted, strategyResults)
	sort.Slice(sorted, func(i, j int) bool {
		return strategyPriority(sorted[i].Name) < strategyPriority(sorted[j].Name)
	})

	// Track seen issues and PRs by number.
	// issueIndex maps issue number → index in merged slice for commit union.
	seenIssues := make(map[int]int) // issue number → merged index
	seenPRs := make(map[int]bool)
	var merged []model.DiscoveredItem

	for _, sr := range sorted {
		for _, item := range sr.Items {
			// Check if this issue was already added by a higher-priority strategy.
			if item.Issue != nil {
				if idx, seen := seenIssues[item.Issue.Number]; seen {
					// Union commits from lower-priority strategy into the existing item.
					merged[idx].Commits = unionCommits(merged[idx].Commits, item.Commits)
					continue
				}
				seenIssues[item.Issue.Number] = len(merged)
			}

			// Check if this PR was already added by a higher-priority strategy.
			if item.PR != nil {
				if seenPRs[item.PR.Number] {
					// If the issue is new but PR is seen, still add the issue.
					if item.Issue != nil {
						if _, seen := seenIssues[item.Issue.Number]; !seen {
							seenIssues[item.Issue.Number] = len(merged)
							merged = append(merged, item)
						}
					}
					continue
				}
				seenPRs[item.PR.Number] = true
			}

			merged = append(merged, item)
		}
	}

	// Sort merged results by issue/PR number for stable output.
	sort.Slice(merged, func(i, j int) bool {
		ni := itemNumber(merged[i])
		nj := itemNumber(merged[j])
		return ni < nj
	})

	return merged
}

// unionCommits merges two commit slices, deduplicating by SHA.
func unionCommits(a, b []model.Commit) []model.Commit {
	seen := make(map[string]bool, len(a))
	for _, c := range a {
		seen[c.SHA] = true
	}
	result := make([]model.Commit, len(a))
	copy(result, a)
	for _, c := range b {
		if !seen[c.SHA] {
			result = append(result, c)
			seen[c.SHA] = true
		}
	}
	return result
}

// itemNumber returns the primary number for sorting (issue number if available, else PR number).
func itemNumber(item model.DiscoveredItem) int {
	if item.Issue != nil {
		return item.Issue.Number
	}
	if item.PR != nil {
		return item.PR.Number
	}
	return 0
}
