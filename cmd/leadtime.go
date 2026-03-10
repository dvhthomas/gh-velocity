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
			started := issue.ClosedAt != nil || issue.State == "open" // always true for issues

			w := cmd.OutOrStdout()
			switch deps.Format {
			case format.JSON:
				return format.WriteLeadTimeJSON(w, deps.Owner+"/"+deps.Repo, issueNumber, issue.Title, issue.State, issue.CreatedAt, lt, nil)
			case format.Markdown:
				fmt.Fprintf(w, "| Issue | Title | Started | Lead Time |\n")
				fmt.Fprintf(w, "| ---: | --- | --- | --- |\n")
				fmt.Fprintf(w, "| #%d | %s | %s | %s |\n", issueNumber, issue.Title, issue.CreatedAt.Format(time.DateOnly), format.FormatCycleStatus(lt, started))
			default:
				tp := format.NewTable(w, deps.IsTTY, deps.TermWidth)
				tp.AddField(fmt.Sprintf("Issue #%d", issueNumber))
				tp.AddField(issue.Title)
				tp.EndRow()
				tp.AddField("Started")
				tp.AddField(issue.CreatedAt.Format(time.RFC3339))
				tp.EndRow()
				tp.AddField("Lead Time")
				tp.AddField(format.FormatCycleStatus(lt, started))
				tp.EndRow()
				if err := tp.Render(); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
