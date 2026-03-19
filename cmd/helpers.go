package cmd

import (
	"context"

	gh "github.com/dvhthomas/gh-velocity/internal/github"
	"github.com/dvhthomas/gh-velocity/internal/metrics"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// buildCycleTimeStrategy creates the appropriate CycleTimeStrategy based on config.
// For the issue strategy, it populates label-based signal sources.
func buildCycleTimeStrategy(_ context.Context, deps *Deps, client *gh.Client) metrics.CycleTimeStrategy {
	cfg := deps.Config
	switch cfg.CycleTime.Strategy {
	case model.StrategyPR:
		return &metrics.PRStrategy{}
	default: // model.StrategyIssue
		strat := &metrics.IssueStrategy{Client: client}

		// Label-based lifecycle: immutable createdAt timestamps.
		if len(cfg.Lifecycle.InProgress.Match) > 0 {
			strat.InProgressMatch = cfg.Lifecycle.InProgress.Match
		}

		// No signal source configured.
		if len(strat.InProgressMatch) == 0 {
			strat.Client = nil // no API calls needed
		}

		return strat
	}
}
