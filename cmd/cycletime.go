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
	startEvent := &model.Event{
		Time:   pr.CreatedAt,
		Signal: model.SignalPRCreated,
		Detail: fmt.Sprintf("PR #%d", prNumber),
	}

	var endEvent *model.Event
	if pr.MergedAt != nil {
		endEvent = &model.Event{
			Time:   *pr.MergedAt,
			Signal: model.SignalPRMerged,
		}
	} else if pr.State == "closed" {
		warnings = append(warnings, "PR was closed without merging")
	} else {
		warnings = append(warnings, "PR is still open; cycle time is in progress")
	}

	ct := metrics.CycleTime(startEvent, endEvent)

	w := cmd.OutOrStdout()
	switch deps.Format {
	case format.JSON:
		return format.WriteCycleTimePRJSON(w, deps.Owner+"/"+deps.Repo, prNumber, pr.Title, pr.State, ct, warnings)
	case format.Markdown:
		fmt.Fprintf(w, "| PR | Title | Started | Cycle Time |\n")
		fmt.Fprintf(w, "| ---: | --- | --- | --- |\n")
		fmt.Fprintf(w, "| #%d | %s | %s | %s |\n",
			prNumber, pr.Title, pr.CreatedAt.Format(time.DateOnly), format.FormatMetric(ct))
		for _, warn := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
		}
	default:
		fmt.Fprintf(w, "PR #%d  %s\n", prNumber, pr.Title)
		fmt.Fprintf(w, "  Started:    %s\n", pr.CreatedAt.Format(time.RFC3339))
		fmt.Fprintf(w, "  Cycle Time: %s\n", format.FormatMetric(ct))
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
	var startEvent *model.Event
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
			startEvent = &model.Event{
				Time:   ps.CycleStart.Time,
				Signal: ps.CycleStart.Signal,
				Detail: ps.CycleStart.Detail,
			}
		}
	}

	// Signals #2–#4: label, PR created, first assigned
	// Skip if backlog override is active — issue was moved back to backlog.
	if startEvent == nil && !backlogOverride {
		csResult, csErr := client.GetCycleStart(ctx, issueNumber, cfg.Statuses.ActiveLabels, cfg.Statuses.BacklogLabels)
		if csErr != nil {
			warnings = append(warnings, fmt.Sprintf("could not query issue timeline: %v", csErr))
		} else {
			if csResult.InBacklog {
				backlogOverride = true
				warnings = append(warnings, "issue has a backlog label; cycle time not applicable until work starts")
			} else if csResult.CycleStart != nil {
				startEvent = &model.Event{
					Time:   csResult.CycleStart.Time,
					Signal: csResult.CycleStart.Signal,
					Detail: csResult.CycleStart.Detail,
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
			if startEvent == nil && !backlogOverride && len(commits) > 0 {
				firstCommit := commits[len(commits)-1]
				startEvent = &model.Event{
					Time:   firstCommit.AuthoredAt,
					Signal: model.SignalCommit,
					Detail: firstCommit.SHA[:7],
				}
			}
		}
	}

	var endEvent *model.Event
	if issue.ClosedAt != nil {
		endEvent = &model.Event{
			Time:   *issue.ClosedAt,
			Signal: model.SignalIssueClosed,
		}
	}

	ct := metrics.CycleTime(startEvent, endEvent)

	w := cmd.OutOrStdout()
	switch deps.Format {
	case format.JSON:
		return format.WriteCycleTimeJSON(w, deps.Owner+"/"+deps.Repo, issueNumber, issue.Title, issue.State, commitCount, ct, warnings)
	case format.Markdown:
		fmt.Fprintf(w, "| Issue | Title | Started | Cycle Time | Commits |\n")
		fmt.Fprintf(w, "| ---: | --- | --- | --- | ---: |\n")
		startedStr := "N/A"
		if startEvent != nil {
			startedStr = startEvent.Time.Format(time.DateOnly)
		}
		fmt.Fprintf(w, "| #%d | %s | %s | %s | %d |\n",
			issueNumber, issue.Title, startedStr, format.FormatMetric(ct), commitCount)
		for _, warn := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
		}
	default:
		fmt.Fprintf(w, "Issue #%d  %s\n", issueNumber, issue.Title)
		if startEvent != nil {
			fmt.Fprintf(w, "  Started:    %s\n", startEvent.Time.Format(time.RFC3339))
		}
		fmt.Fprintf(w, "  Cycle Time: %s\n", format.FormatMetric(ct))
		if commitCount > 0 {
			fmt.Fprintf(w, "  Commits:    %d\n", commitCount)
		}
		for _, warn := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
		}
	}

	return nil
}
