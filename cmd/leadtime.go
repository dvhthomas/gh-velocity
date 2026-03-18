package cmd

import (
	"strconv"

	"github.com/dvhthomas/gh-velocity/internal/dateutil"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/dvhthomas/gh-velocity/internal/pipeline/leadtime"
	"github.com/dvhthomas/gh-velocity/internal/posting"
	"github.com/dvhthomas/gh-velocity/internal/scope"
	"github.com/spf13/cobra"
)

// NewLeadTimeCmd returns the lead-time command.
func NewLeadTimeCmd() *cobra.Command {
	var sinceFlag, untilFlag string

	cmd := &cobra.Command{
		Use:   "lead-time [<issue>]",
		Short: "Lead time for an issue or bulk query (created → closed)",
		Long: `Lead time measures the elapsed time from issue creation to close.

Single mode:  gh velocity lead-time 42
Bulk mode:    gh velocity lead-time --since 30d [--until 2026-03-01]

In bulk mode, returns per-item rows plus aggregate statistics
for all issues closed in the given date window.`,
		Example: `  # Single issue
  gh velocity flow lead-time 42
  gh velocity flow lead-time 42 -R cli/cli

  # All issues closed in the last 30 days
  gh velocity flow lead-time --since 30d

  # Custom window, JSON output
  gh velocity flow lead-time --since 2026-01-01 --until 2026-02-01 -f json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && sinceFlag != "" {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "provide either an issue number or --since, not both",
				}
			}
			if sinceFlag != "" {
				return runLeadTimeBulk(cmd, sinceFlag, untilFlag)
			}
			if len(args) == 0 {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "provide an issue number or use --since for bulk mode",
				}
			}
			return runLeadTimeSingle(cmd, args[0])
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Start of date window (YYYY-MM-DD, RFC3339, or Nd relative)")
	cmd.Flags().StringVar(&untilFlag, "until", "", "End of date window (default: now)")

	return cmd
}

// runLeadTimeSingle computes lead time for a single issue.
func runLeadTimeSingle(cmd *cobra.Command, arg string) error {
	issueNumber, err := parseIssueArg(arg)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	deps := DepsFromContext(ctx)
	if deps == nil {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "internal error: missing dependencies",
		}
	}

	client, err := deps.NewClient()
	if err != nil {
		return err
	}

	p := &leadtime.SinglePipeline{
		Client:      client,
		Owner:       deps.Owner,
		Repo:        deps.Repo,
		IssueNumber: issueNumber,
	}

	if err := p.GatherData(ctx); err != nil {
		return err
	}
	if err := p.ProcessData(); err != nil {
		return err
	}

	w, postFn := postIfEnabled(cmd, deps, client, posting.PostOptions{
		Command: "lead-time",
		Context: strconv.Itoa(issueNumber),
		Target:  posting.IssueComment,
		Number:  issueNumber,
	})
	rc := deps.RenderCtx(w)
	if err := p.Render(rc); err != nil {
		return err
	}
	return postFn()
}

// runLeadTimeBulk computes lead time for all issues closed in a date window.
func runLeadTimeBulk(cmd *cobra.Command, sinceStr, untilStr string) error {
	ctx := cmd.Context()
	deps := DepsFromContext(ctx)
	if deps == nil {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "internal error: missing dependencies",
		}
	}

	now := deps.Now()
	since, err := dateutil.Parse(sinceStr, now)
	if err != nil {
		return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
	}

	until := now
	if untilStr != "" {
		until, err = dateutil.Parse(untilStr, now)
		if err != nil {
			return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
		}
	}

	if err := dateutil.ValidateWindow(since, until, now); err != nil {
		return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
	}

	client, err := deps.NewClient()
	if err != nil {
		return err
	}

	q := scope.ClosedIssueQuery(deps.Scope, since, until)
	q.ExcludeUsers = deps.ExcludeUsers
	if deps.Debug {
		log.Debug("lead-time query:\n%s", q.Verbose())
	}

	p := &leadtime.BulkPipeline{
		Client:      client,
		Owner:       deps.Owner,
		Repo:        deps.Repo,
		Since:       since,
		Until:       until,
		SearchQuery: q.Build(),
		SearchURL:   q.URL(),
	}

	if err := p.GatherData(ctx); err != nil {
		return err
	}
	if err := p.ProcessData(); err != nil {
		return err
	}

	w, postFn := postIfEnabled(cmd, deps, client, posting.PostOptions{
		Command: "lead-time",
		Context: dateutil.FormatContext(sinceStr, untilStr),
		Target:  posting.DiscussionTarget,
	})
	rc := deps.RenderCtx(w)
	if err := p.Render(rc); err != nil {
		return err
	}
	return postFn()
}
