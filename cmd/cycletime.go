package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/cycletime"
	"github.com/bitsbyme/gh-velocity/internal/dateutil"
	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/posting"
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
    strategy: issue          # issue created → issue closed (default)
    strategy: pr             # PR created → PR merged
    strategy: project-board  # status change → issue closed

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
  gh velocity flow cycle-time --since 14d -R cli/cli -f markdown`,
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

	client, err := gh.NewClient(deps.Owner, deps.Repo)
	if err != nil {
		return err
	}

	pr, err := client.GetPR(ctx, prNumber)
	if err != nil {
		return err
	}

	strat := &cycletime.PRStrategy{}
	ct := strat.Compute(ctx, cycletime.Input{PR: pr})

	var warnings []string
	if pr.MergedAt == nil {
		if pr.State == "closed" {
			warnings = append(warnings, "PR was closed without merging")
		} else {
			warnings = append(warnings, "PR is still open; cycle time is in progress")
		}
	}

	return outputCycleTime(cmd, deps, client, posting.PostOptions{
		Command: "cycle-time",
		Context: "pr-" + strconv.Itoa(prNumber),
		Target:  posting.PRComment,
		Number:  prNumber,
	}, ct, warnings, "PR", prNumber, pr.Title, pr.State)
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

	client, err := gh.NewClient(deps.Owner, deps.Repo)
	if err != nil {
		return err
	}

	issue, err := client.GetIssue(ctx, issueNumber)
	if err != nil {
		return err
	}

	strat := buildCycleTimeStrategy(deps, client)
	var warnings []string
	input := cycletime.Input{Issue: issue}

	// For PR strategy, find the closing PR.
	if deps.Config.CycleTime.Strategy == "pr" {
		pr, prErr := client.GetClosingPR(ctx, issueNumber)
		if prErr != nil {
			warnings = append(warnings, fmt.Sprintf("could not find closing PR: %v", prErr))
		} else if pr == nil {
			warnings = append(warnings, "no closing PR found for this issue")
		} else {
			input.PR = pr
		}
	}

	ct := strat.Compute(ctx, input)

	return outputCycleTime(cmd, deps, client, posting.PostOptions{
		Command: "cycle-time",
		Context: strconv.Itoa(issueNumber),
		Target:  posting.IssueComment,
		Number:  issueNumber,
	}, ct, warnings, "Issue", issueNumber, issue.Title, issue.State)
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

	now := time.Now().UTC()
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

	client, err := gh.NewClient(deps.Owner, deps.Repo)
	if err != nil {
		return err
	}

	issues, err := client.SearchClosedIssues(ctx, since, until)
	if err != nil {
		return err
	}

	strat := buildCycleTimeStrategy(deps, client)

	// For PR strategy, bulk-fetch closing PRs to avoid N+1 API calls.
	closingPRs := make(map[int]*model.PR)
	if deps.Config.CycleTime.Strategy == "pr" {
		mergedPRs, prErr := client.SearchMergedPRs(ctx, since, until)
		if prErr != nil {
			log.Warn("could not search merged PRs: %v", prErr)
		} else {
			closingPRs = buildClosingPRMap(ctx, client, mergedPRs)
		}
	}

	var items []format.BulkCycleTimeItem
	var durations []time.Duration

	for _, issue := range issues {
		input := cycletime.Input{Issue: &issue}

		if pr, ok := closingPRs[issue.Number]; ok {
			input.PR = pr
		}

		ct := strat.Compute(ctx, input)
		items = append(items, format.BulkCycleTimeItem{Issue: issue, Metric: ct})
		if ct.Duration != nil {
			durations = append(durations, *ct.Duration)
		}
	}

	stats := metrics.ComputeStats(durations)
	repo := deps.Owner + "/" + deps.Repo

	w, postFn := postIfEnabled(cmd, deps, client, posting.PostOptions{
		Command: "cycle-time",
		Context: dateutil.FormatContext(sinceStr, untilStr),
		Target:  posting.DiscussionTarget,
	})

	var fmtErr error
	switch deps.Format {
	case format.JSON:
		fmtErr = format.WriteCycleTimeBulkJSON(w, repo, since, until, deps.Config.CycleTime.Strategy, items, stats)
	case format.Markdown:
		fmtErr = format.WriteCycleTimeBulkMarkdown(w, repo, since, until, deps.Config.CycleTime.Strategy, items, stats)
	default:
		fmtErr = format.WriteCycleTimeBulkPretty(w, deps.IsTTY, deps.TermWidth, repo, since, until, deps.Config.CycleTime.Strategy, items, stats)
	}
	if fmtErr != nil {
		return fmtErr
	}
	return postFn()
}

// outputCycleTime renders cycle-time results in the requested format and optionally posts.
func outputCycleTime(cmd *cobra.Command, deps *Deps, client *gh.Client, postOpts posting.PostOptions, ct model.Metric, warnings []string, kind string, number int, title, state string) error {
	w, postFn := postIfEnabled(cmd, deps, client, postOpts)
	repo := deps.Owner + "/" + deps.Repo

	for _, warn := range warnings {
		log.Warn("%s", warn)
	}

	var fmtErr error
	switch deps.Format {
	case format.JSON:
		if kind == "PR" {
			fmtErr = format.WriteCycleTimePRJSON(w, repo, number, title, state, ct, warnings)
		} else {
			fmtErr = format.WriteCycleTimeJSON(w, repo, number, title, state, ct, warnings)
		}
	case format.Markdown:
		fmtErr = format.WriteCycleTimeMarkdown(w, kind, number, title, ct)
	default:
		fmtErr = format.WriteCycleTimePretty(w, kind, number, title, deps.Config.CycleTime.Strategy, ct)
	}
	if fmtErr != nil {
		return fmtErr
	}
	return postFn()
}
