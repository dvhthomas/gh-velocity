package cmd

import (
	"github.com/bitsbyme/gh-velocity/internal/cycletime"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
)

// buildCycleTimeStrategy creates the appropriate CycleTimeStrategy based on config.
func buildCycleTimeStrategy(deps *Deps, client *gh.Client) cycletime.Strategy {
	cfg := deps.Config
	switch cfg.CycleTime.Strategy {
	case "pr":
		return &cycletime.PRStrategy{}
	case "project-board":
		// TODO(PR C): resolve cfg.Project.URL → project node ID at runtime.
		// For now, ProjectBoardStrategy fields are empty; the config validator
		// ensures project.url is set when strategy is "project-board".
		return &cycletime.ProjectBoardStrategy{
			Client: client,
		}
	default: // "issue"
		return &cycletime.IssueStrategy{}
	}
}

