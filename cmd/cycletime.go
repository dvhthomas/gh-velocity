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
		Short: "Cycle time for an issue (first commit → closed/merged)",
		Args:  cobra.ExactArgs(1),
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

			// Find commits referencing this issue using targeted git log --grep
			var warnings []string
			var commits []model.Commit

			if deps.HasLocalRepo {
				wd, wdErr := os.Getwd()
				if wdErr != nil {
					return fmt.Errorf("get working directory: %w", wdErr)
				}
				if gitdata.IsShallowClone(wd) {
					fmt.Fprintf(os.Stderr, "warning: shallow clone detected; commit history is incomplete. Use 'actions/checkout' with fetch-depth: 0 for accurate metrics.\n")
				}
				source := gitdata.NewLocalSource(wd)
				commits, err = source.CommitsForIssue(ctx, issueNumber, "HEAD")
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("could not read git log: %v", err))
					commits = nil
				}
			} else {
				// Without a local checkout, we cannot enumerate all commits
				// to find issue references. Warn the user.
				warnings = append(warnings, "no local git checkout; commit linking unavailable via API for cycle-time (use from within a git repo for full results)")
			}

			// Compute cycle time
			var ctDuration *time.Duration
			if len(commits) > 0 && issue.ClosedAt != nil {
				firstCommit := commits[len(commits)-1].AuthoredAt // oldest commit
				ctDuration = metrics.CycleTime(firstCommit, *issue.ClosedAt)
			}

			w := cmd.OutOrStdout()
			switch deps.Format {
			case format.JSON:
				return format.WriteCycleTimeJSON(w, deps.Owner+"/"+deps.Repo, issueNumber, issue.Title, issue.State, len(commits), ctDuration, warnings)
			case format.Markdown:
				fmt.Fprintf(w, "| Issue | Title | Cycle Time | Commits |\n")
				fmt.Fprintf(w, "| ---: | --- | --- | ---: |\n")
				fmt.Fprintf(w, "| #%d | %s | %s | %d |\n",
					issueNumber, issue.Title, format.FormatDurationPtr(ctDuration), len(commits))
				for _, warn := range warnings {
					fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
				}
			default:
				fmt.Fprintf(w, "Issue #%d: %s\n", issueNumber, issue.Title)
				fmt.Fprintf(w, "Cycle Time: %s (%d commits)\n", format.FormatDurationPtr(ctDuration), len(commits))
				for _, warn := range warnings {
					fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
				}
			}

			return nil
		},
	}
}
