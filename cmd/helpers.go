package cmd

import (
	"context"

	"github.com/bitsbyme/gh-velocity/internal/cycletime"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// buildCycleTimeStrategy creates the appropriate CycleTimeStrategy based on config.
func buildCycleTimeStrategy(deps *Deps, client *gh.Client) cycletime.Strategy {
	cfg := deps.Config
	switch cfg.CycleTime.Strategy {
	case "pr":
		return &cycletime.PRStrategy{}
	case "project-board":
		return &cycletime.ProjectBoardStrategy{
			Client:        client,
			ProjectID:     cfg.Project.ID,
			StatusFieldID: cfg.Project.StatusFieldID,
			BacklogStatus: cfg.Statuses.Backlog,
		}
	default: // "issue"
		return &cycletime.IssueStrategy{}
	}
}

// buildClosingPRMap maps issue numbers to their closing PRs using bulk-fetched
// merged PRs and their linked issues. Avoids N+1 API calls.
func buildClosingPRMap(ctx context.Context, client *gh.Client, mergedPRs []model.PR) map[int]*model.PR {
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
		log.Warn("could not fetch PR linked issues: %v", err)
		return closingPRs
	}

	for prNum, issues := range linkedIssues {
		for _, issue := range issues {
			closingPRs[issue.Number] = prMap[prNum]
		}
	}

	return closingPRs
}
