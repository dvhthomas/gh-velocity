package cmd

import (
	"context"

	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// buildCycleTimeStrategy creates the appropriate CycleTimeStrategy based on config.
// For the issue strategy, it populates label-based and/or project board signal
// sources. Labels are preferred (immutable timestamps); project board is a fallback.
func buildCycleTimeStrategy(ctx context.Context, deps *Deps, client *gh.Client) metrics.CycleTimeStrategy {
	cfg := deps.Config
	switch cfg.CycleTime.Strategy {
	case model.StrategyPR:
		return &metrics.PRStrategy{}
	default: // model.StrategyIssue (also handles deprecated "project-board")
		strat := &metrics.IssueStrategy{Client: client}

		// Label-based lifecycle: preferred signal (immutable createdAt).
		if len(cfg.Lifecycle.InProgress.Match) > 0 {
			strat.InProgressMatch = cfg.Lifecycle.InProgress.Match
		}

		// Project board: fallback signal (updatedAt may be stale).
		if len(cfg.Lifecycle.InProgress.ProjectStatus) > 0 && cfg.Project.URL != "" {
			info, err := client.ResolveProject(ctx, cfg.Project.URL, cfg.Project.StatusField)
			if err != nil {
				deps.WarnUnlessJSON("Could not resolve project for cycle time: %v", err)
			} else {
				strat.ProjectID = info.ProjectID
				strat.StatusFieldID = info.StatusFieldID
				strat.BacklogStatus = cfg.Lifecycle.Backlog.ProjectStatus
			}
		}

		// Warn when project board is the only cycle-start signal.
		if strat.ProjectID != "" && len(strat.InProgressMatch) == 0 {
			deps.WarnUnlessJSON("cycle time: project board timestamps can be unreliable (reflects last update, not transition). Recommend adding lifecycle.in-progress.match with label matchers. Run: gh velocity config preflight --write")
		}

		// No signal source at all.
		if strat.ProjectID == "" && len(strat.InProgressMatch) == 0 {
			strat.Client = nil // no API calls needed
		}

		return strat
	}
}
