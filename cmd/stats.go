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

// NewStatsCmd returns the stats command.
func NewStatsCmd() *cobra.Command {
	var (
		sinceFlag, untilFlag string
	)

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Dashboard of velocity and quality metrics",
		Long: `Show a trailing-window dashboard composing lead time, cycle time,
throughput, work in progress, and quality metrics.

Default window is the last 30 days. Use --since and --until to customize.

Each section computes independently; a failure in one section does not
block others. Sections that require specific config (WIP needs project.id
or active_labels; quality needs releases) are gracefully omitted when
unavailable.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			deps := DepsFromContext(ctx)
			if deps == nil {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "internal error: missing dependencies",
				}
			}

			now := time.Now().UTC()

			// Default: 30 days
			if sinceFlag == "" {
				sinceFlag = "30d"
			}
			since, err := dateutil.Parse(sinceFlag, now)
			if err != nil {
				return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
			}

			until := now
			if untilFlag != "" {
				until, err = dateutil.Parse(untilFlag, now)
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

			repo := deps.Owner + "/" + deps.Repo
			result := computeStats(ctx, deps, client, repo, since, until, now)

			w := cmd.OutOrStdout()
			switch deps.Format {
			case format.JSON:
				return format.WriteStatsJSON(w, result)
			case format.Markdown:
				return format.WriteStatsMarkdown(w, result)
			default:
				return format.WriteStatsPretty(w, deps.IsTTY, deps.TermWidth, result)
			}
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Start of date window (default: 30d)")
	cmd.Flags().StringVar(&untilFlag, "until", "", "End of date window (default: now)")

	return cmd
}

// computeStats gathers all dashboard sections with graceful degradation.
func computeStats(ctx context.Context, deps *Deps, client *gh.Client, repo string, since, until, now time.Time) format.StatsResult {
	result := format.StatsResult{
		Repository: repo,
		Since:      since,
		Until:      until,
	}

	// 1. Closed issues (shared by lead time, cycle time, throughput, quality)
	closedIssues, err := client.SearchClosedIssues(ctx, since, until)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch closed issues: %v\n", err)
		return result
	}

	// 2. Lead Time
	var leadDurations []time.Duration
	for _, issue := range closedIssues {
		m := metrics.LeadTime(issue)
		if m.Duration != nil {
			leadDurations = append(leadDurations, *m.Duration)
		}
	}
	leadStats := metrics.ComputeStats(leadDurations)
	result.LeadTime = &leadStats

	// 3. Cycle Time
	strat, _ := buildStrategy(ctx, deps, client, 0)

	// For PR strategy, bulk-fetch closing PRs.
	closingPRs := make(map[int]*model.PR)
	if deps.Config.CycleTime.Strategy == "pr" {
		mergedPRs, prErr := client.SearchMergedPRs(ctx, since, until)
		if prErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not search merged PRs for cycle time: %v\n", prErr)
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

	var cycleDurations []time.Duration
	for _, issue := range closedIssues {
		input := cycletime.Input{Issue: &issue}
		if pr, ok := closingPRs[issue.Number]; ok {
			input.PR = pr
		}
		ct := strat.Compute(ctx, input)
		if ct.Duration != nil {
			cycleDurations = append(cycleDurations, *ct.Duration)
		}
	}
	cycleStats := metrics.ComputeStats(cycleDurations)
	result.CycleTime = &cycleStats

	// 4. Throughput
	throughput := format.StatsThroughput{
		IssuesClosed: len(closedIssues),
	}
	// Count merged PRs (reuse if already fetched for PR strategy, otherwise fetch)
	if deps.Config.CycleTime.Strategy == "pr" {
		// Already counted from the bulk fetch above
		mergedPRs, _ := client.SearchMergedPRs(ctx, since, until)
		throughput.PRsMerged = len(mergedPRs)
	} else {
		mergedPRs, prErr := client.SearchMergedPRs(ctx, since, until)
		if prErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not count merged PRs: %v\n", prErr)
		} else {
			throughput.PRsMerged = len(mergedPRs)
		}
	}
	result.Throughput = &throughput

	// 5. WIP (optional)
	cfg := deps.Config
	if cfg.Project.ID != "" {
		projectItems, listErr := client.ListProjectItems(ctx, cfg.Project.ID, cfg.Project.StatusFieldID)
		if listErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not fetch WIP from project board: %v\n", listErr)
		} else {
			backlog := cfg.Statuses.Backlog
			if backlog == "" {
				backlog = "Backlog"
			}
			done := cfg.Statuses.Done
			if done == "" {
				done = "Done"
			}
			count := 0
			for _, pi := range projectItems {
				if pi.Status != backlog && pi.Status != done {
					count++
				}
			}
			result.WIPCount = &count
		}
	} else if len(cfg.Statuses.ActiveLabels) > 0 {
		activeIssues, searchErr := client.SearchOpenIssuesWithLabels(ctx, cfg.Statuses.ActiveLabels)
		if searchErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not fetch WIP from labels: %v\n", searchErr)
		} else {
			backlogSet := make(map[string]bool)
			for _, l := range cfg.Statuses.BacklogLabels {
				backlogSet[l] = true
			}
			count := 0
			for _, issue := range activeIssues {
				hasBacklog := false
				for _, l := range issue.Labels {
					if backlogSet[l] {
						hasBacklog = true
						break
					}
				}
				if !hasBacklog {
					count++
				}
			}
			result.WIPCount = &count
		}
	}

	// 6. Quality: defect rate from bug labels in closed issues
	bugLabels := make(map[string]bool)
	for _, l := range cfg.Quality.BugLabels {
		bugLabels[l] = true
	}
	bugCount := 0
	for _, issue := range closedIssues {
		for _, l := range issue.Labels {
			if bugLabels[l] {
				bugCount++
				break
			}
		}
	}
	if len(closedIssues) > 0 {
		defectRate := float64(bugCount) / float64(len(closedIssues))
		result.Quality = &format.StatsQuality{
			BugCount:    bugCount,
			TotalIssues: len(closedIssues),
			DefectRate:  defectRate,
		}
	}

	return result
}
