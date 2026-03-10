package cmd

import (
	"time"

	"github.com/bitsbyme/gh-velocity/internal/dateutil"
	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/posting"
	"github.com/spf13/cobra"
)

// NewReportCmd returns the report command (composite dashboard).
func NewReportCmd() *cobra.Command {
	var (
		sinceFlag, untilFlag string
	)

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Composite dashboard of velocity and quality metrics",
		Long: `Show a trailing-window report composing lead time, cycle time,
throughput, work in progress, and quality metrics.

Default window is the last 30 days. Use --since and --until to customize.

Each section computes independently; a failure in one section does not
block others. Sections that require specific config (WIP needs project.id
or active_labels; quality needs releases) are gracefully omitted when
unavailable.`,
		Example: `  # Default: last 30 days
  gh velocity report

  # Custom window
  gh velocity report --since 14d --until 2026-03-01

  # Remote repo, JSON for CI dashboards
  gh velocity report --since 30d -R cli/cli -f json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			deps := DepsFromContext(ctx)
			if deps == nil {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "internal error: missing dependencies",
				}
			}

			now := time.Now().UTC()

			// Default: 30 days
			if sinceFlag == "" {
				sinceFlag = "30d"
			}
			since, err := dateutil.Parse(sinceFlag, now)
			if err != nil {
				return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
			}

			until := now
			if untilFlag != "" {
				until, err = dateutil.Parse(untilFlag, now)
				if err != nil {
					return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
				}
			}

			if err := dateutil.ValidateWindow(since, until, now); err != nil {
				return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
			}

			client, err := gh.NewClient(deps.Owner, deps.Repo)
			if err != nil {
				return err
			}

			repo := deps.Owner + "/" + deps.Repo
			cfg := deps.Config
			result := metrics.ComputeDashboard(ctx, client, metrics.DashboardInput{
				Repo:              repo,
				Since:             since,
				Until:             until,
				Now:               now,
				CycleTimeStrategy: buildCycleTimeStrategy(deps, client),
				CycleTimeLabel:    cfg.CycleTime.Strategy,
				ProjectID:         cfg.Project.ID,
				StatusFieldID:     cfg.Project.StatusFieldID,
				BacklogStatus:     cfg.Statuses.Backlog,
				DoneStatus:        cfg.Statuses.Done,
				ActiveLabels:      cfg.Statuses.ActiveLabels,
				BacklogLabels:     cfg.Statuses.BacklogLabels,
				BugLabels:         cfg.Quality.BugLabels,
			})

			w, postFn := postIfEnabled(cmd, deps, client, posting.PostOptions{
				Command: "report",
				Context: dateutil.FormatContext(sinceFlag, untilFlag),
				Target:  posting.DiscussionTarget,
			})

			var fmtErr error
			switch deps.Format {
			case format.JSON:
				fmtErr = format.WriteReportJSON(w, result)
			case format.Markdown:
				fmtErr = format.WriteReportMarkdown(w, result)
			default:
				fmtErr = format.WriteReportPretty(w, deps.IsTTY, deps.TermWidth, result)
			}
			if fmtErr != nil {
				return fmtErr
			}
			return postFn()
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Start of date window (default: 30d)")
	cmd.Flags().StringVar(&untilFlag, "until", "", "End of date window (default: now)")

	return cmd
}
