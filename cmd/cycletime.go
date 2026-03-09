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
	return &cobra.Command{
		Use:   "cycle-time <issue>",
		Short: "Cycle time for an issue (work started → closed)",
		Long: `Cycle time measures how long an issue was actively worked on.

The start signal is detected automatically, in priority order:
  1. Status change — issue moved out of backlog in a Projects v2 board
  2. Label — an "active" label was added (e.g., "in-progress")
  3. PR created — any PR referencing the issue was opened (including drafts)
  4. First assigned — when the issue was first assigned to someone
  5. First commit — the earliest commit referencing the issue (requires local git)

The end signal is the issue's close date.

Signals #1-2 require configuration in .gh-velocity.yml. Signal #1 needs
project.id/status_field_id. Signal #2 needs statuses.active_labels.
Signals #3-4 work automatically. Signal #5 requires a local clone.

If the issue is currently in backlog (Projects v2 status or backlog_labels),
cycle time is suppressed even if other signals exist.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			issueNumber, err := parseIssueArg(args[0])
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
					// Issue is in backlog — suppress cycle time even if other
					// signals exist. Someone may have been assigned then the
					// issue was moved back to backlog.
					backlogOverride = true
					warnings = append(warnings, "issue is in backlog; cycle time not applicable until work starts")
				} else if ps.CycleStart != nil && issue.ClosedAt != nil {
					ctDuration = metrics.CycleTime(ps.CycleStart.Time, *issue.ClosedAt)
					signal = fmt.Sprintf("%s (%s)", ps.CycleStart.Signal, ps.CycleStart.Detail)
				}
			}

			// Signals #2–#4: label, PR created, first assigned
			// Skip if backlog override is active — issue was moved back to backlog.
			if ctDuration == nil && !backlogOverride {
				csResult, csErr := client.GetCycleStart(ctx, issueNumber, cfg.Statuses.ActiveLabels, cfg.Statuses.BacklogLabels)
				if csErr != nil {
					warnings = append(warnings, fmt.Sprintf("could not query issue timeline: %v", csErr))
				} else {
					// Label-based backlog check (alternative to Projects v2).
					if csResult.InBacklog {
						backlogOverride = true
						warnings = append(warnings, "issue has a backlog label; cycle time not applicable until work starts")
					} else if csResult.CycleStart != nil && issue.ClosedAt != nil {
						ctDuration = metrics.CycleTime(csResult.CycleStart.Time, *issue.ClosedAt)
						signal = fmt.Sprintf("%s (%s)", csResult.CycleStart.Signal, csResult.CycleStart.Detail)
					}
				}
			}

			// Signal #4 + enrichment: commits (requires local clone)
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
					if ctDuration == nil && !backlogOverride && len(commits) > 0 && issue.ClosedAt != nil {
						firstCommit := commits[len(commits)-1].AuthoredAt
						ctDuration = metrics.CycleTime(firstCommit, *issue.ClosedAt)
						signal = fmt.Sprintf("commit (%s)", commits[len(commits)-1].SHA[:7])
					}
				}
			}

			w := cmd.OutOrStdout()
			switch deps.Format {
			case format.JSON:
				return format.WriteCycleTimeJSON(w, deps.Owner+"/"+deps.Repo, issueNumber, issue.Title, issue.State, commitCount, ctDuration, signal, warnings)
			case format.Markdown:
				fmt.Fprintf(w, "| Issue | Title | Cycle Time | Signal | Commits |\n")
				fmt.Fprintf(w, "| ---: | --- | --- | --- | ---: |\n")
				fmt.Fprintf(w, "| #%d | %s | %s | %s | %d |\n",
					issueNumber, issue.Title, format.FormatDurationPtr(ctDuration), signal, commitCount)
				for _, warn := range warnings {
					fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
				}
			default:
				fmt.Fprintf(w, "Issue #%d: %s\n", issueNumber, issue.Title)
				fmt.Fprintf(w, "Cycle Time: %s\n", format.FormatDurationPtr(ctDuration))
				if signal != "" {
					fmt.Fprintf(w, "Signal: %s\n", signal)
				}
				if commitCount > 0 {
					fmt.Fprintf(w, "Commits: %d\n", commitCount)
				}
				for _, warn := range warnings {
					fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
				}
			}

			return nil
		},
	}
}
