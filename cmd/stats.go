package cmd

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/cycletime"
	"github.com/bitsbyme/gh-velocity/internal/dateutil"
	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
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

// computeStats gathers all dashboard sections concurrently with graceful degradation.
// Independent API calls run in parallel; compute phases share fetched data.
func computeStats(ctx context.Context, deps *Deps, client *gh.Client, repo string, since, until, now time.Time) format.StatsResult {
	result := format.StatsResult{
		Repository: repo,
		Since:      since,
		Until:      until,
	}

	cfg := deps.Config

	// --- Phase 1: Parallel API fetches ---
	// Three independent data sources fetched concurrently:
	// 1. Closed issues (used by lead time, cycle time, throughput, quality)
	// 2. Merged PRs (used by throughput, and cycle time PR strategy)
	// 3. WIP items (optional, from project board or labels)

	var (
		closedIssues []model.Issue
		mergedPRs    []model.PR
		closingPRs   map[int]*model.PR // issue number → closing PR (PR strategy only)
		wipCount     *int
		warnings     []string
		mu           sync.Mutex // guards result writes from goroutines
	)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5) // rate limit protection

	// Fetch closed issues
	g.Go(func() error {
		issues, err := client.SearchClosedIssues(gctx, since, until)
		if err != nil {
			mu.Lock()
			warnings = append(warnings, fmt.Sprintf("could not fetch closed issues: %v", err))
			mu.Unlock()
			return nil // graceful: don't fail the group
		}
		mu.Lock()
		closedIssues = issues
		mu.Unlock()
		return nil
	})

	// Fetch merged PRs (for throughput, and PR strategy cycle time).
	// When PR strategy is active, also fetches linked issues in the same goroutine
	// to overlap with other parallel fetches.
	g.Go(func() error {
		prs, err := client.SearchMergedPRs(gctx, since, until)
		if err != nil {
			mu.Lock()
			warnings = append(warnings, fmt.Sprintf("could not fetch merged PRs: %v", err))
			mu.Unlock()
			return nil
		}
		prMap := buildClosingPRMap(gctx, client, prs)
		mu.Lock()
		mergedPRs = prs
		closingPRs = prMap
		mu.Unlock()
		return nil
	})

	// Fetch WIP (optional — project board or labels)
	if cfg.Project.ID != "" {
		g.Go(func() error {
			projectItems, err := client.ListProjectItems(gctx, cfg.Project.ID, cfg.Project.StatusFieldID)
			if err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("could not fetch WIP from project board: %v", err))
				mu.Unlock()
				return nil
			}
			count := 0
			for _, pi := range projectItems {
				if pi.Status != cfg.Statuses.Backlog && pi.Status != cfg.Statuses.Done {
					count++
				}
			}
			mu.Lock()
			wipCount = &count
			mu.Unlock()
			return nil
		})
	} else if len(cfg.Statuses.ActiveLabels) > 0 {
		g.Go(func() error {
			activeIssues, err := client.SearchOpenIssuesWithLabels(gctx, cfg.Statuses.ActiveLabels)
			if err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("could not fetch WIP from labels: %v", err))
				mu.Unlock()
				return nil
			}
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
			mu.Lock()
			wipCount = &count
			mu.Unlock()
			return nil
		})
	}

	// Wait for all fetches to complete.
	if waitErr := g.Wait(); waitErr != nil {
		warnings = append(warnings, fmt.Sprintf("stats fetch error: %v", waitErr))
	}
	result.Warnings = warnings

	// --- Phase 2: Compute metrics from fetched data ---
	// All computation is CPU-bound; no further API calls needed
	// (except PR strategy which may need FetchPRLinkedIssues).

	if closedIssues == nil {
		return result
	}

	// Lead Time
	var leadDurations []time.Duration
	for _, issue := range closedIssues {
		m := metrics.LeadTime(issue)
		if m.Duration != nil {
			leadDurations = append(leadDurations, *m.Duration)
		}
	}
	leadStats := metrics.ComputeStats(leadDurations)
	result.LeadTime = &leadStats

	// Cycle Time
	strat := buildCycleTimeStrategy(deps, client)

	if closingPRs == nil {
		closingPRs = make(map[int]*model.PR)
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
	result.CycleTimeStrategy = cfg.CycleTime.Strategy

	// Throughput
	result.Throughput = &format.StatsThroughput{
		IssuesClosed: len(closedIssues),
		PRsMerged:    len(mergedPRs),
	}

	// WIP
	result.WIPCount = wipCount

	// Quality: defect rate from bug labels in closed issues
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
