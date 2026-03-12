package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/dateutil"
	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/posting"
	"github.com/bitsbyme/gh-velocity/internal/scope"
	"github.com/spf13/cobra"
)

// NewLeadTimeCmd returns the lead-time command.
func NewLeadTimeCmd() *cobra.Command {
	var sinceFlag, untilFlag string

	cmd := &cobra.Command{
		Use:   "lead-time [<issue>]",
		Short: "Lead time for an issue or bulk query (created → closed)",
		Long: `Lead time measures the elapsed time from issue creation to close.

Single mode:  gh velocity lead-time 42
Bulk mode:    gh velocity lead-time --since 30d [--until 2026-03-01]

In bulk mode, returns per-item rows plus aggregate statistics
for all issues closed in the given date window.`,
		Example: `  # Single issue
  gh velocity flow lead-time 42
  gh velocity flow lead-time 42 -R cli/cli

  # All issues closed in the last 30 days
  gh velocity flow lead-time --since 30d

  # Custom window, JSON output
  gh velocity flow lead-time --since 2026-01-01 --until 2026-02-01 -f json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && sinceFlag != "" {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "provide either an issue number or --since, not both",
				}
			}
			if sinceFlag != "" {
				return runLeadTimeBulk(cmd, sinceFlag, untilFlag)
			}
			if len(args) == 0 {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "provide an issue number or use --since for bulk mode",
				}
			}
			return runLeadTimeSingle(cmd, args[0])
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Start of date window (YYYY-MM-DD, RFC3339, or Nd relative)")
	cmd.Flags().StringVar(&untilFlag, "until", "", "End of date window (default: now)")

	return cmd
}

// runLeadTimeSingle computes lead time for a single issue.
func runLeadTimeSingle(cmd *cobra.Command, arg string) error {
	issueNumber, err := parseIssueArg(arg)
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

	w, postFn := postIfEnabled(cmd, deps, client, posting.PostOptions{
		Command: "lead-time",
		Context: strconv.Itoa(issueNumber),
		Target:  posting.IssueComment,
		Number:  issueNumber,
	})
	switch deps.Format {
	case format.JSON:
		if err = format.WriteLeadTimeJSON(w, deps.Owner+"/"+deps.Repo, issueNumber, issue.Title, issue.State, issue.URL, issue.Labels, lt, nil); err != nil {
			return err
		}
	case format.Markdown:
		rc := deps.RenderCtx(w)
		fmt.Fprintf(w, "| Issue | Title | Created (UTC) | Lead Time |\n")
		fmt.Fprintf(w, "| ---: | --- | --- | --- |\n")
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n", format.FormatItemLink(issueNumber, issue.URL, rc), issue.Title, issue.CreatedAt.UTC().Format(time.DateOnly), format.FormatMetric(lt))
	default:
		rc := deps.RenderCtx(w)
		fmt.Fprintf(w, "Issue %s  %s\n", format.FormatItemLink(issueNumber, issue.URL, rc), issue.Title)
		fmt.Fprintf(w, "  Created:   %s UTC\n", issue.CreatedAt.UTC().Format(time.RFC3339))
		fmt.Fprintf(w, "  Lead Time: %s\n", format.FormatMetric(lt))
	}

	return postFn()
}

// runLeadTimeBulk computes lead time for all issues closed in a date window.
func runLeadTimeBulk(cmd *cobra.Command, sinceStr, untilStr string) error {
	ctx := cmd.Context()
	deps := DepsFromContext(ctx)
	if deps == nil {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "internal error: missing dependencies",
		}
	}

	now := deps.Now()
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

	q := scope.ClosedIssueQuery(deps.Scope, since, until)
	q.ExcludeUsers = deps.ExcludeUsers
	if deps.Debug {
		log.Debug("lead-time query:\n%s", q.Verbose())
	}
	issues, err := client.SearchIssues(ctx, q.Build())
	if err != nil {
		return err
	}

	// Compute per-issue lead time and collect durations for stats.
	var items []format.BulkLeadTimeItem
	var durations []time.Duration

	for _, issue := range issues {
		lt := metrics.LeadTime(issue)
		items = append(items, format.BulkLeadTimeItem{Issue: issue, Metric: lt})
		if lt.Duration != nil {
			durations = append(durations, *lt.Duration)
		}
	}

	stats := metrics.ComputeStats(durations)
	repo := deps.Owner + "/" + deps.Repo

	w, postFn := postIfEnabled(cmd, deps, client, posting.PostOptions{
		Command: "lead-time",
		Context: dateutil.FormatContext(sinceStr, untilStr),
		Target:  posting.DiscussionTarget,
	})

	searchURL := q.URL()

	var fmtErr error
	switch deps.Format {
	case format.JSON:
		fmtErr = format.WriteLeadTimeBulkJSON(w, repo, since, until, items, stats, searchURL)
	case format.Markdown:
		fmtErr = format.WriteLeadTimeBulkMarkdown(deps.RenderCtx(w), repo, since, until, items, stats, searchURL)
	default:
		fmtErr = format.WriteLeadTimeBulkPretty(deps.RenderCtx(w), repo, since, until, items, stats, searchURL)
	}
	if fmtErr != nil {
		return fmtErr
	}
	return postFn()
}
