package cmd

import (
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/spf13/cobra"
)

// NewLeadTimeCmd returns the lead-time command.
func NewLeadTimeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lead-time <issue>",
		Short: "Lead time for an issue (created → closed)",
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

			lt := metrics.LeadTime(*issue)

			w := cmd.OutOrStdout()
			switch deps.Format {
			case format.JSON:
				return format.WriteLeadTimeJSON(w, deps.Owner+"/"+deps.Repo, issueNumber, issue.Title, issue.State, lt, nil)
			case format.Markdown:
				fmt.Fprintf(w, "| Issue | Title | Created (UTC) | Lead Time |\n")
				fmt.Fprintf(w, "| ---: | --- | --- | --- |\n")
				fmt.Fprintf(w, "| #%d | %s | %s | %s |\n", issueNumber, issue.Title, issue.CreatedAt.UTC().Format(time.DateOnly), format.FormatMetric(lt))
			default:
				fmt.Fprintf(w, "Issue #%d  %s\n", issueNumber, issue.Title)
				fmt.Fprintf(w, "  Created:   %s UTC\n", issue.CreatedAt.UTC().Format(time.RFC3339))
				fmt.Fprintf(w, "  Lead Time: %s\n", format.FormatMetric(lt))
			}

			return nil
		},
	}
}
