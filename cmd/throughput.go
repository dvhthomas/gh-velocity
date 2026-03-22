package cmd

import (
	"github.com/dvhthomas/gh-velocity/internal/dateutil"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/dvhthomas/gh-velocity/internal/pipeline/throughput"
	"github.com/dvhthomas/gh-velocity/internal/posting"
	"github.com/dvhthomas/gh-velocity/internal/scope"
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
  gh velocity flow throughput --since 7d -r json

  # Remote repo
  gh velocity flow throughput --since 30d -R cli/cli`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps := DepsFromContext(cmd.Context())
			if deps == nil {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "internal error: missing dependencies",
				}
			}

			if sinceFlag == "" {
				sinceFlag = "30d"
			}
			since, until, err := parseDateWindow(sinceFlag, untilFlag, deps.Now())
			if err != nil {
				return err
			}

			client, err := deps.NewClient()
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

			return renderPipeline(cmd, deps, p, client, posting.PostOptions{
				Command: "throughput",
				Context: dateutil.FormatContext(sinceFlag, untilFlag),
				Target:  posting.DiscussionTarget,
			})
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Start of date window (default: 30d)")
	cmd.Flags().StringVar(&untilFlag, "until", "", "End of date window (default: now)")

	return cmd
}
