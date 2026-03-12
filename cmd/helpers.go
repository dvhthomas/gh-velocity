package cmd

import (
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
)

// buildCycleTimeStrategy creates the appropriate CycleTimeStrategy based on config.
func buildCycleTimeStrategy(deps *Deps, client *gh.Client) metrics.CycleTimeStrategy {
	cfg := deps.Config
	switch cfg.CycleTime.Strategy {
	case "pr":
		return &metrics.PRStrategy{}
	case "project-board":
		// TODO(PR C): resolve cfg.Project.URL → project node ID at runtime.
		// For now, ProjectBoardStrategy fields are empty; the config validator
		// ensures project.url is set when strategy is "project-board".
		return &metrics.ProjectBoardStrategy{
			Client: client,
		}
	default: // "issue"
		return &metrics.IssueStrategy{}
	}
}
