package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/cycletime"
	"github.com/bitsbyme/gh-velocity/internal/dateutil"
	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
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

	return outputCycleTime(cmd, deps, ct, warnings, "PR", prNumber, pr.Title, pr.State, 0)
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

	strat, warnings := buildStrategy(ctx, deps, client, issueNumber)
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

	return outputCycleTime(cmd, deps, ct, warnings, "Issue", issueNumber, issue.Title, issue.State, 0)
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

	strat, _ := buildStrategy(ctx, deps, client, 0)

	// For PR strategy, bulk-fetch closing PRs to avoid N+1 API calls.
	// Uses SearchMergedPRs + FetchPRLinkedIssues (batched) instead of per-issue GetClosingPR.
	closingPRs := make(map[int]*model.PR) // issue number → closing PR
	if deps.Config.CycleTime.Strategy == "pr" {
		mergedPRs, prErr := client.SearchMergedPRs(ctx, since, until)
		if prErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not search merged PRs: %v\n", prErr)
		} else if len(mergedPRs) > 0 {
			prNumbers := make([]int, len(mergedPRs))
			prMap := make(map[int]*model.PR)
			for i, pr := range mergedPRs {
				prNumbers[i] = pr.Number
				prCopy := pr
				prMap[pr.Number] = &prCopy
			}
			linkedIssues, linkErr := client.FetchPRLinkedIssues(ctx, prNumbers)
			if linkErr != nil {
				fmt.Fprintf(os.Stderr, "warning: could not fetch PR linked issues: %v\n", linkErr)
			} else {
				for prNum, issues := range linkedIssues {
					for _, issue := range issues {
						closingPRs[issue.Number] = prMap[prNum]
					}
				}
			}
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

	w := cmd.OutOrStdout()
	switch deps.Format {
	case format.JSON:
		return format.WriteCycleTimeBulkJSON(w, repo, since, until, deps.Config.CycleTime.Strategy, items, stats)
	case format.Markdown:
		return format.WriteCycleTimeBulkMarkdown(w, repo, since, until, deps.Config.CycleTime.Strategy, items, stats)
	default:
		return format.WriteCycleTimeBulkPretty(w, deps.IsTTY, deps.TermWidth, repo, since, until, deps.Config.CycleTime.Strategy, items, stats)
	}
}

// buildStrategy creates the appropriate CycleTimeStrategy based on config.
func buildStrategy(ctx context.Context, deps *Deps, client *gh.Client, issueNumber int) (cycletime.Strategy, []string) {
	cfg := deps.Config
	var warnings []string

	switch cfg.CycleTime.Strategy {
	case "pr":
		return &cycletime.PRStrategy{}, warnings
	case "project-board":
		backlog := cfg.Statuses.Backlog
		if backlog == "" {
			backlog = "Backlog"
		}
		return &cycletime.ProjectBoardStrategy{
			Client:        client,
			ProjectID:     cfg.Project.ID,
			StatusFieldID: cfg.Project.StatusFieldID,
			BacklogStatus: backlog,
		}, warnings
	default: // "issue"
		return &cycletime.IssueStrategy{}, warnings
	}
}

// outputCycleTime renders cycle-time results in the requested format.
func outputCycleTime(cmd *cobra.Command, deps *Deps, ct model.Metric, warnings []string, kind string, number int, title, state string, commitCount int) error {
	w := cmd.OutOrStdout()
	repo := deps.Owner + "/" + deps.Repo

	switch deps.Format {
	case format.JSON:
		if kind == "PR" {
			return format.WriteCycleTimePRJSON(w, repo, number, title, state, ct, warnings)
		}
		return format.WriteCycleTimeJSON(w, repo, number, title, state, commitCount, ct, warnings)
	case format.Markdown:
		fmt.Fprintf(w, "| %s | Title | Started (UTC) | Cycle Time |\n", kind)
		fmt.Fprintf(w, "| ---: | --- | --- | --- |\n")
		startedStr := "N/A"
		if ct.Start != nil {
			startedStr = ct.Start.Time.UTC().Format(time.DateOnly)
		}
		fmt.Fprintf(w, "| #%d | %s | %s | %s |\n", number, title, startedStr, format.FormatMetric(ct))
		for _, warn := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
		}
	default:
		fmt.Fprintf(w, "%s #%d  %s\n", kind, number, title)
		fmt.Fprintf(w, "  Strategy:   %s\n", deps.Config.CycleTime.Strategy)
		if ct.Start != nil {
			fmt.Fprintf(w, "  Started:    %s UTC\n", ct.Start.Time.UTC().Format(time.RFC3339))
		}
		fmt.Fprintf(w, "  Cycle Time: %s\n", format.FormatMetric(ct))
		for _, warn := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
		}
	}

	return nil
}
