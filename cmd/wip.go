package cmd

import (
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/spf13/cobra"
)

// NewWIPCmd returns the wip command.
func NewWIPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wip",
		Short: "Show work in progress",
		Long: `Show items currently in progress.

Primary source: Projects v2 board status (requires project.url in config).

Use -R owner/repo to filter board items to a specific repo.`,
		Example: `  # Show WIP from configured project board
  gh velocity status wip

  # Filter to a specific repo on the board
  gh velocity status wip -R owner/repo

  # JSON output for CI/automation
  gh velocity status wip -f json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps := DepsFromContext(cmd.Context())
			if deps == nil {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "internal error: missing dependencies",
				}
			}

			cfg := deps.Config

			// TODO(PR C #22): resolve cfg.Project.URL → project node ID at runtime.
			// TODO(PR D #23): wire lifecycle stages (backlog/done project_status) for WIP filtering.
			if cfg.Project.URL != "" {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "wip with project boards is not yet supported (project URL resolution coming soon)\n\n  Track progress: gh issue view 22 -R dvhthomas/gh-velocity",
				}
			}

			return &model.AppError{
				Code:    model.ErrConfigInvalid,
				Message: "wip requires project.url in .gh-velocity.yml\n\n  To auto-detect your setup:  gh velocity config preflight -R owner/repo\n  To find project boards:     gh velocity config discover -R owner/repo",
			}
		},
	}

	return cmd
}
