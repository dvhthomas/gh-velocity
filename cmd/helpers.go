package cmd

import (
	"context"

	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
)

// buildCycleTimeStrategy creates the appropriate CycleTimeStrategy based on config.
// For the issue strategy, it resolves project board IDs from config and populates
// the strategy with lifecycle backlog status values.
func buildCycleTimeStrategy(ctx context.Context, deps *Deps, client *gh.Client) metrics.CycleTimeStrategy {
	cfg := deps.Config
	switch cfg.CycleTime.Strategy {
	case "pr":
		return &metrics.PRStrategy{}
	default: // "issue" (also handles deprecated "project-board")
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
		}
		return strat
	}
}
