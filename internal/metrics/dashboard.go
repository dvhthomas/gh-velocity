package metrics

import (
	"context"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// PRLinker can fetch the issues linked to a set of PRs.
type PRLinker interface {
	FetchPRLinkedIssues(ctx context.Context, prNumbers []int) (map[int][]model.Issue, error)
}

// BuildClosingPRMap builds a map from issue number to closing PR by fetching linked issues.
func BuildClosingPRMap(ctx context.Context, client PRLinker, mergedPRs []model.PR) map[int]*model.PR {
	closingPRs := make(map[int]*model.PR)
	if len(mergedPRs) == 0 {
		return closingPRs
	}

	prNumbers := make([]int, len(mergedPRs))
	prMap := make(map[int]*model.PR)
	for i, pr := range mergedPRs {
		prNumbers[i] = pr.Number
		prCopy := pr
		prMap[pr.Number] = &prCopy
	}

	linkedIssues, err := client.FetchPRLinkedIssues(ctx, prNumbers)
	if err != nil {
		// Graceful: return empty map, caller will proceed without PR data.
		return closingPRs
	}

	for prNum, issues := range linkedIssues {
		for _, issue := range issues {
			closingPRs[issue.Number] = prMap[prNum]
		}
	}
	return closingPRs
}
