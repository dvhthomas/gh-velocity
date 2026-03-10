package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/gitdata"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/spf13/cobra"
)

// NewCycleTimeCmd returns the cycle-time command.
func NewCycleTimeCmd() *cobra.Command {
	var prFlag int

	cmd := &cobra.Command{
		Use:   "cycle-time [<issue>]",
		Short: "Cycle time for an issue or PR (work started → closed)",
		Long: `Cycle time measures how long an issue or PR was actively worked on.

For issues, the start signal is detected automatically, in priority order:
  1. Status change — issue moved out of backlog in a Projects v2 board
  2. Label — an "active" label was added (e.g., "in-progress")
  3. PR created — any PR referencing the issue was opened (including drafts)
  4. First assigned — when the issue was first assigned to someone
  5. First commit — the earliest commit referencing the issue (requires local git)

For PRs (--pr flag), cycle time is PR created → PR merged.

The end signal is the issue's close date (or PR merge date with --pr).

Signals #1-2 require configuration in .gh-velocity.yml. Signal #1 needs
project.id/status_field_id. Signal #2 needs statuses.active_labels.
Signals #3-4 work automatically. Signal #5 requires a local clone.

If the issue is currently in backlog (Projects v2 status or backlog_labels),
cycle time is suppressed even if other signals exist.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
					Message: "provide an issue number or use --pr <number>",
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

	return cmd
}

// runCycleTimePR computes cycle time for a PR: created → merged.
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

	var warnings []string
	var ctDuration *time.Duration
	var signal string
	startedAt := &pr.CreatedAt // always known for a PR

	if pr.MergedAt != nil {
		ctDuration = metrics.CycleTime(pr.CreatedAt, *pr.MergedAt)
		signal = "pr-lifecycle (created → merged)"
	} else if pr.State == "closed" {
		signal = "pr-lifecycle (closed without merge)"
		warnings = append(warnings, "PR was closed without merging")
	} else {
		signal = "pr-lifecycle (in progress)"
		warnings = append(warnings, "PR is still open; cycle time is in progress")
	}

	w := cmd.OutOrStdout()
	switch deps.Format {
	case format.JSON:
		return format.WriteCycleTimePRJSON(w, deps.Owner+"/"+deps.Repo, prNumber, pr.Title, pr.State, startedAt, ctDuration, signal, warnings)
	case format.Markdown:
		fmt.Fprintf(w, "| PR | Title | Started | Cycle Time | Signal |\n")
		fmt.Fprintf(w, "| ---: | --- | --- | --- | --- |\n")
		fmt.Fprintf(w, "| #%d | %s | %s | %s | %s |\n",
			prNumber, pr.Title, startedAt.Format(time.DateOnly), format.FormatCycleStatus(ctDuration, true), signal)
		for _, warn := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
		}
	default:
		tp := format.NewTable(w, deps.IsTTY, deps.TermWidth)
		tp.AddField(fmt.Sprintf("PR #%d", prNumber))
		tp.AddField(pr.Title)
		tp.EndRow()
		tp.AddField("Started")
		tp.AddField(startedAt.Format(time.RFC3339))
		tp.EndRow()
		tp.AddField("Cycle Time")
		tp.AddField(format.FormatCycleStatus(ctDuration, true))
		tp.EndRow()
		if signal != "" {
			tp.AddField("Signal")
			tp.AddField(signal)
			tp.EndRow()
		}
		if err := tp.Render(); err != nil {
			return err
		}
		for _, warn := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
		}
	}

	return nil
}

// runCycleTimeIssue computes cycle time for an issue using the signal hierarchy.
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

	var warnings []string
	var ctDuration *time.Duration
	var startedAt *time.Time
	var signal string
	var commitCount int
	backlogOverride := false // true when issue is in backlog → cycle time is N/A

	// Signal #1: Projects v2 status change (requires project config)
	cfg := deps.Config
	if cfg.Project.ID != "" || cfg.Project.StatusFieldID != "" {
		backlog := cfg.Statuses.Backlog
		if backlog == "" {
			backlog = "Backlog"
		}
		ps, psErr := client.GetProjectStatus(ctx, issueNumber, cfg.Project.ID, cfg.Project.StatusFieldID, backlog)
		if psErr != nil {
			warnings = append(warnings, fmt.Sprintf("could not query project status: %v", psErr))
		} else if ps.InBacklog {
			backlogOverride = true
			warnings = append(warnings, "issue is in backlog; cycle time not applicable until work starts")
		} else if ps.CycleStart != nil {
			startedAt = &ps.CycleStart.Time
			signal = fmt.Sprintf("%s (%s)", ps.CycleStart.Signal, ps.CycleStart.Detail)
			if issue.ClosedAt != nil {
				ctDuration = metrics.CycleTime(ps.CycleStart.Time, *issue.ClosedAt)
			}
		}
	}

	// Signals #2–#4: label, PR created, first assigned
	// Skip if backlog override is active — issue was moved back to backlog.
	if startedAt == nil && !backlogOverride {
		csResult, csErr := client.GetCycleStart(ctx, issueNumber, cfg.Statuses.ActiveLabels, cfg.Statuses.BacklogLabels)
		if csErr != nil {
			warnings = append(warnings, fmt.Sprintf("could not query issue timeline: %v", csErr))
		} else {
			if csResult.InBacklog {
				backlogOverride = true
				warnings = append(warnings, "issue has a backlog label; cycle time not applicable until work starts")
			} else if csResult.CycleStart != nil {
				startedAt = &csResult.CycleStart.Time
				signal = fmt.Sprintf("%s (%s)", csResult.CycleStart.Signal, csResult.CycleStart.Detail)
				if issue.ClosedAt != nil {
					ctDuration = metrics.CycleTime(csResult.CycleStart.Time, *issue.ClosedAt)
				}
			}
		}
	}

	// Signal #5 + enrichment: commits (requires local clone)
	if deps.HasLocalRepo {
		wd, wdErr := os.Getwd()
		if wdErr != nil {
			return fmt.Errorf("get working directory: %w", wdErr)
		}
		if gitdata.IsShallowClone(wd) {
			warnings = append(warnings, "shallow clone detected; commit history may be incomplete (use fetch-depth: 0)")
		}
		source := gitdata.NewLocalSource(wd)
		commits, gitErr := source.CommitsForIssue(ctx, issueNumber, "HEAD")
		if gitErr != nil {
			warnings = append(warnings, fmt.Sprintf("could not read git log: %v", gitErr))
		} else {
			commitCount = len(commits)
			// If no higher-priority signal found and not in backlog, fall back to first commit
			if startedAt == nil && !backlogOverride && len(commits) > 0 {
				firstCommit := commits[len(commits)-1].AuthoredAt
				startedAt = &firstCommit
				signal = fmt.Sprintf("commit (%s)", commits[len(commits)-1].SHA[:7])
				if issue.ClosedAt != nil {
					ctDuration = metrics.CycleTime(firstCommit, *issue.ClosedAt)
				}
			}
		}
	}

	started := startedAt != nil
	w := cmd.OutOrStdout()
	switch deps.Format {
	case format.JSON:
		return format.WriteCycleTimeJSON(w, deps.Owner+"/"+deps.Repo, issueNumber, issue.Title, issue.State, commitCount, startedAt, ctDuration, signal, warnings)
	case format.Markdown:
		fmt.Fprintf(w, "| Issue | Title | Started | Cycle Time | Signal | Commits |\n")
		fmt.Fprintf(w, "| ---: | --- | --- | --- | --- | ---: |\n")
		startedStr := "N/A"
		if startedAt != nil {
			startedStr = startedAt.Format(time.DateOnly)
		}
		fmt.Fprintf(w, "| #%d | %s | %s | %s | %s | %d |\n",
			issueNumber, issue.Title, startedStr, format.FormatCycleStatus(ctDuration, started), signal, commitCount)
		for _, warn := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
		}
	default:
		tp := format.NewTable(w, deps.IsTTY, deps.TermWidth)
		tp.AddField(fmt.Sprintf("Issue #%d", issueNumber))
		tp.AddField(issue.Title)
		tp.EndRow()
		if startedAt != nil {
			tp.AddField("Started")
			tp.AddField(startedAt.Format(time.RFC3339))
			tp.EndRow()
		}
		tp.AddField("Cycle Time")
		tp.AddField(format.FormatCycleStatus(ctDuration, started))
		tp.EndRow()
		if signal != "" {
			tp.AddField("Signal")
			tp.AddField(signal)
			tp.EndRow()
		}
		if commitCount > 0 {
			tp.AddField("Commits")
			tp.AddField(fmt.Sprintf("%d", commitCount))
			tp.EndRow()
		}
		if err := tp.Render(); err != nil {
			return err
		}
		for _, warn := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
		}
	}

	return nil
}
