package cmd

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/gitdata"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/linking"
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
			issueNumber, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid issue number %q: must be a positive integer", args[0])
			}
			if issueNumber <= 0 {
				return fmt.Errorf("invalid issue number %d: must be a positive integer", issueNumber)
			}

			ctx := cmd.Context()
			deps := DepsFromContext(ctx)
			if deps == nil {
				return fmt.Errorf("internal error: missing dependencies")
			}

			client, err := gh.NewClient(deps.Owner, deps.Repo)
			if err != nil {
				return err
			}

			issue, err := client.GetIssue(ctx, issueNumber)
			if err != nil {
				return err
			}

			// Find commits referencing this issue
			var warnings []string
			var allCommits []model.Commit

			if deps.HasLocalRepo {
				wd, wdErr := os.Getwd()
				if wdErr != nil {
					return fmt.Errorf("get working directory: %w", wdErr)
				}
				source := gitdata.NewLocalSource(wd)
				allCommits, err = source.AllCommits(ctx, "HEAD")
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("could not read git log: %v", err))
					allCommits = nil
				}
			} else {
				// Without a local checkout, we cannot enumerate all commits
				// to find issue references. Warn the user.
				warnings = append(warnings, "no local git checkout; commit linking unavailable via API for cycle-time (use from within a git repo for full results)")
			}

			issueCommitsMap := linking.LinkCommitsToIssues(allCommits)
			commits := issueCommitsMap[issueNumber]

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
