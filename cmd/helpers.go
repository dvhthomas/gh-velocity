package cmd

import (
	"context"

	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// buildCycleTimeStrategy creates the appropriate CycleTimeStrategy based on config.
// For the issue strategy, it resolves project board IDs from config and populates
// the strategy with lifecycle backlog status values.
func buildCycleTimeStrategy(ctx context.Context, deps *Deps, client *gh.Client) metrics.CycleTimeStrategy {
	cfg := deps.Config
	switch cfg.CycleTime.Strategy {
	case model.StrategyPR:
		return &metrics.PRStrategy{}
	default: // model.StrategyIssue (also handles deprecated "project-board")
		strat := &metrics.IssueStrategy{}
		// Resolve project board IDs if lifecycle in-progress uses project_status.
		if len(cfg.Lifecycle.InProgress.ProjectStatus) > 0 && cfg.Project.URL != "" {
			info, err := client.ResolveProject(ctx, cfg.Project.URL, cfg.Project.StatusField)
			if err != nil {
				log.Warn("Could not resolve project for cycle time: %v", err)
				return strat
			}
			strat.Client = client
			strat.ProjectID = info.ProjectID
			strat.StatusFieldID = info.StatusFieldID
			strat.BacklogStatus = cfg.Lifecycle.Backlog.ProjectStatus
		} else if len(cfg.Lifecycle.InProgress.Match) > 0 {
			// Label-based cycle time: use timeline API to detect "work started".
			strat.Client = client
			strat.InProgressMatch = cfg.Lifecycle.InProgress.Match
		}
		return strat
	}
}
