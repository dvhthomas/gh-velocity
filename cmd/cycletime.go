package cmd

import (
	"fmt"
	"strconv"

	"github.com/dvhthomas/gh-velocity/internal/dateutil"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/metrics"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/dvhthomas/gh-velocity/internal/pipeline/cycletime"
	"github.com/dvhthomas/gh-velocity/internal/posting"
	"github.com/dvhthomas/gh-velocity/internal/scope"
	"github.com/spf13/cobra"
)

// NewCycleTimeCmd returns the cycle-time command.
func NewCycleTimeCmd() *cobra.Command {
	var (
		prFlag               int
		sinceFlag, untilFlag string
	)

	cmd := &cobra.Command{
		Use:   "cycle-time [<issue>]",
		Short: "Cycle time for an issue, PR, or bulk query",
		Long: `Cycle time measures how long an issue or PR was actively worked on.

The measurement strategy is set in .gh-velocity.yml:

  cycle_time:
    strategy: issue  # work started → issue closed (default)
    strategy: pr     # PR created → PR merged

The issue strategy detects "work started" from lifecycle config
(project board status change). Configure lifecycle.in-progress for
cycle time metrics.

Single mode:  gh velocity cycle-time 42
              gh velocity cycle-time --pr 99
Bulk mode:    gh velocity cycle-time --since 30d [--until 2026-03-01]

The --pr flag overrides the configured strategy for a single run.
When a signal is not available for an item, cycle time is N/A.`,
		Example: `  # Single issue (uses configured strategy)
  gh velocity flow cycle-time 42

  # Single PR (always uses PR created → merged)
  gh velocity flow cycle-time --pr 99

  # All issues closed in the last 30 days
  gh velocity flow cycle-time --since 30d

  # Remote repo, markdown output
  gh velocity flow cycle-time --since 14d -R cli/cli -r markdown`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Conflict: --since with positional or --pr
			if sinceFlag != "" {
				if len(args) > 0 {
					return &model.AppError{
						Code:    model.ErrConfigInvalid,
						Message: "provide either an issue number or --since, not both",
					}
				}
				if prFlag > 0 {
					return &model.AppError{
						Code:    model.ErrConfigInvalid,
						Message: "provide either --pr or --since, not both",
					}
				}
				return runCycleTimeBulk(cmd, sinceFlag, untilFlag)
			}

			if prFlag > 0 {
				if len(args) > 0 {
					return &model.AppError{
						Code:    model.ErrConfigInvalid,
						Message: "provide either a positional issue number or --pr, not both",
					}
				}
				return runCycleTimePR(cmd, prFlag)
			}

			if len(args) == 0 {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "provide an issue number, --pr <number>, or --since for bulk mode",
				}
			}

			issueNumber, err := parseIssueArg(args[0])
			if err != nil {
				return err
			}
			return runCycleTimeIssue(cmd, issueNumber)
		},
	}

	cmd.Flags().IntVar(&prFlag, "pr", 0, "Measure cycle time for a pull request instead of an issue")
	cmd.Flags().StringVar(&sinceFlag, "since", "", "Start of date window (YYYY-MM-DD, RFC3339, or Nd relative)")
	cmd.Flags().StringVar(&untilFlag, "until", "", "End of date window (default: now)")

	return cmd
}

// runCycleTimePR computes cycle time for a specific PR: created → merged.
func runCycleTimePR(cmd *cobra.Command, prNumber int) error {
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

	p := &cycletime.PRPipeline{
		Client:   client,
		Owner:    deps.Owner,
		Repo:     deps.Repo,
		PRNumber: prNumber,
	}

	if err := p.GatherData(ctx); err != nil {
		return err
	}
	if err := p.ProcessData(); err != nil {
		return err
	}

	for _, warn := range p.Warnings {
		deps.Warn("%s", warn)
	}

	return renderPipeline(cmd, deps, p, client, posting.PostOptions{
		Command: "cycle-time",
		Context: "pr-" + strconv.Itoa(prNumber),
		Target:  posting.PRComment,
		Number:  prNumber,
	})
}

// runCycleTimeIssue computes cycle time for an issue using the configured strategy.
func runCycleTimeIssue(cmd *cobra.Command, issueNumber int) error {
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

	strat := buildCycleTimeStrategy(ctx, deps, client)

	p := &cycletime.IssuePipeline{
		Client:      client,
		Owner:       deps.Owner,
		Repo:        deps.Repo,
		IssueNumber: issueNumber,
		Strategy:    strat,
		StrategyStr: deps.Config.CycleTime.Strategy,
	}

	if err := p.GatherData(ctx); err != nil {
		return err
	}
	if err := p.ProcessData(); err != nil {
		return err
	}

	for _, warn := range p.Warnings {
		deps.Warn("%s", warn)
	}

	return renderPipeline(cmd, deps, p, client, posting.PostOptions{
		Command: "cycle-time",
		Context: strconv.Itoa(issueNumber),
		Target:  posting.IssueComment,
		Number:  issueNumber,
	})
}

// runCycleTimeBulk computes cycle time for all issues closed in a date window.
func runCycleTimeBulk(cmd *cobra.Command, sinceStr, untilStr string) error {
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

	issueQuery := scope.ClosedIssueQuery(deps.Scope, since, until)
	issueQuery.ExcludeUsers = deps.ExcludeUsers
	if deps.Debug {
		log.Debug("cycle-time issue query:\n%s", issueQuery.Verbose())
	}

	strat := buildCycleTimeStrategy(ctx, deps, client)

	// For PR strategy, bulk-fetch closing PRs to avoid N+1 API calls.
	closingPRs := make(map[int]*model.PR)
	var preWarnings []string
	if deps.Config.CycleTime.Strategy == model.StrategyPR {
		prQuery := scope.MergedPRQuery(deps.Scope, since, until)
		prQuery.ExcludeUsers = deps.ExcludeUsers
		if deps.Debug {
			log.Debug("cycle-time PR query:\n%s", prQuery.Verbose())
		}
		mergedPRs, prErr := client.SearchPRs(ctx, prQuery.Build())
		if prErr != nil {
			w := fmt.Sprintf("could not search merged PRs: %v", prErr)
			deps.Warn("%s", w)
			preWarnings = append(preWarnings, w)
		} else {
			closingPRs = metrics.BuildClosingPRMap(ctx, client, mergedPRs)
		}
	}

	p := &cycletime.BulkPipeline{
		Client:      client,
		Owner:       deps.Owner,
		Repo:        deps.Repo,
		Since:       since,
		Until:       until,
		Strategy:    strat,
		StrategyStr: deps.Config.CycleTime.Strategy,
		SearchQuery: issueQuery.Build(),
		SearchURL:   issueQuery.URL(),
		ClosingPRs:  closingPRs,
	}

	if err := p.GatherData(ctx); err != nil {
		return err
	}
	if err := p.ProcessData(); err != nil {
		return err
	}

	// Merge pre-gather warnings (e.g., PR search failures) with pipeline warnings.
	p.Warnings = append(preWarnings, p.Warnings...)

	for _, warn := range p.Warnings {
		deps.Warn("%s", warn)
	}

	return renderPipeline(cmd, deps, p, client, posting.PostOptions{
		Command: "cycle-time",
		Context: dateutil.FormatContext(sinceStr, untilStr),
		Target:  posting.DiscussionTarget,
	})
}
