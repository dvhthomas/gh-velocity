package cmd

import (
	"github.com/bitsbyme/gh-velocity/internal/dateutil"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/pipeline/throughput"
	"github.com/bitsbyme/gh-velocity/internal/posting"
	"github.com/bitsbyme/gh-velocity/internal/scope"
	"github.com/spf13/cobra"
)

// NewThroughputCmd returns the throughput command.
func NewThroughputCmd() *cobra.Command {
	var (
		sinceFlag, untilFlag string
	)

	cmd := &cobra.Command{
		Use:   "throughput",
		Short: "Count issues closed and PRs merged in a window",
		Long: `Throughput counts the number of issues closed and pull requests merged
in a date window. This is the simplest measure of team output.

Default window is the last 30 days.`,
		Example: `  # Last 30 days
  gh velocity flow throughput

  # Last 7 days, JSON output
  gh velocity flow throughput --since 7d -f json

  # Remote repo
  gh velocity flow throughput --since 30d -R cli/cli`,
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

			now := deps.Now()

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

			issueQuery := scope.ClosedIssueQuery(deps.Scope, since, until)
			issueQuery.ExcludeUsers = deps.ExcludeUsers
			prQuery := scope.MergedPRQuery(deps.Scope, since, until)
			prQuery.ExcludeUsers = deps.ExcludeUsers
			if deps.Debug {
				log.Debug("throughput issue query:\n%s", issueQuery.Verbose())
				log.Debug("throughput PR query:\n%s", prQuery.Verbose())
			}

			p := &throughput.Pipeline{
				Client:     client,
				Owner:      deps.Owner,
				Repo:       deps.Repo,
				Since:      since,
				Until:      until,
				IssueQuery: issueQuery.Build(),
				PRQuery:    prQuery.Build(),
				SearchURL:  issueQuery.URL(),
			}

			if err := p.GatherData(ctx); err != nil {
				return err
			}
			if err := p.ProcessData(); err != nil {
				return err
			}

			w, postFn := postIfEnabled(cmd, deps, client, posting.PostOptions{
				Command: "throughput",
				Context: dateutil.FormatContext(sinceFlag, untilFlag),
				Target:  posting.DiscussionTarget,
			})
			rc := deps.RenderCtx(w)
			if err := p.Render(rc); err != nil {
				return err
			}
			return postFn()
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Start of date window (default: 30d)")
	cmd.Flags().StringVar(&untilFlag, "until", "", "End of date window (default: now)")

	return cmd
}
