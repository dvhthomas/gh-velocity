package cmd

import (
	"fmt"
	"strconv"

	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/spf13/cobra"
)

// NewLeadTimeCmd returns the lead-time command.
func NewLeadTimeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lead-time <issue>",
		Short: "Lead time for an issue (created → closed)",
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

			lt := metrics.LeadTime(*issue)

			w := cmd.OutOrStdout()
			switch deps.Format {
			case format.JSON:
				return format.WriteLeadTimeJSON(w, deps.Owner+"/"+deps.Repo, issueNumber, issue.Title, issue.State, lt, nil)
			case format.Markdown:
				fmt.Fprintf(w, "| Issue | Title | Lead Time |\n")
				fmt.Fprintf(w, "| ---: | --- | --- |\n")
				fmt.Fprintf(w, "| #%d | %s | %s |\n", issueNumber, issue.Title, format.FormatDurationPtr(lt))
			default:
				fmt.Fprintf(w, "Issue #%d: %s\n", issueNumber, issue.Title)
				fmt.Fprintf(w, "Lead Time: %s\n", format.FormatDurationPtr(lt))
			}

			return nil
		},
	}
}
