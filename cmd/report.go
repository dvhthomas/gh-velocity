package cmd

import (
	"fmt"
	"sync"

	"github.com/bitsbyme/gh-velocity/internal/classify"
	"github.com/bitsbyme/gh-velocity/internal/dateutil"
	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
	cycletimepipe "github.com/bitsbyme/gh-velocity/internal/pipeline/cycletime"
	"github.com/bitsbyme/gh-velocity/internal/pipeline/leadtime"
	"github.com/bitsbyme/gh-velocity/internal/pipeline/throughput"
	"github.com/bitsbyme/gh-velocity/internal/posting"
	"github.com/bitsbyme/gh-velocity/internal/scope"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// NewReportCmd returns the report command (composite dashboard).
func NewReportCmd() *cobra.Command {
	var (
		sinceFlag, untilFlag string
	)

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Composite dashboard of velocity and quality metrics",
		Long: `Show a trailing-window report composing lead time, cycle time,
throughput, work in progress, and quality metrics.

Default window is the last 30 days. Use --since and --until to customize.

Each section computes independently; a failure in one section does not
block others. Sections that require specific config (WIP needs project.id
or active_labels; quality needs releases) are gracefully omitted when
unavailable.`,
		Example: `  # Default: last 30 days
  gh velocity report

  # Custom window
  gh velocity report --since 14d --until 2026-03-01

  # Remote repo, JSON for CI dashboards
  gh velocity report --since 30d -R cli/cli -f json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd, sinceFlag, untilFlag)
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Start of date window (default: 30d)")
	cmd.Flags().StringVar(&untilFlag, "until", "", "End of date window (default: now)")

	return cmd
}

func runReport(cmd *cobra.Command, sinceFlag, untilFlag string) error {
	ctx := cmd.Context()
	deps := DepsFromContext(ctx)
	if deps == nil {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "internal error: missing dependencies",
		}
	}

	now := deps.Now()
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

	client, err := deps.NewClient()
	if err != nil {
		return err
	}

	repo := deps.Owner + "/" + deps.Repo
	cfg := deps.Config

	// Build scope-aware queries
	issueQuery := scope.ClosedIssueQuery(deps.Scope, since, until)
	issueQuery.ExcludeUsers = deps.ExcludeUsers
	prQuery := scope.MergedPRQuery(deps.Scope, since, until)
	prQuery.ExcludeUsers = deps.ExcludeUsers

	if deps.Debug {
		log.Debug("report issue query:\n%s", issueQuery.Verbose())
		log.Debug("report PR query:\n%s", prQuery.Verbose())
	}

	// Build pipelines
	leadPipeline := &leadtime.BulkPipeline{
		Client:      client,
		Owner:       deps.Owner,
		Repo:        deps.Repo,
		Since:       since,
		Until:       until,
		SearchQuery: issueQuery.Build(),
		SearchURL:   issueQuery.URL(),
	}

	strat := buildCycleTimeStrategy(ctx, deps, client)
	cyclePipeline := &cycletimepipe.BulkPipeline{
		Client:      client,
		Owner:       deps.Owner,
		Repo:        deps.Repo,
		Since:       since,
		Until:       until,
		Strategy:    strat,
		StrategyStr: cfg.CycleTime.Strategy,
		SearchQuery: issueQuery.Build(),
		SearchURL:   issueQuery.URL(),
	}

	throughputPipeline := &throughput.Pipeline{
		Client:     client,
		Owner:      deps.Owner,
		Repo:       deps.Repo,
		Since:      since,
		Until:      until,
		IssueQuery: issueQuery.Build(),
		PRQuery:    prQuery.Build(),
		SearchURL:  issueQuery.URL(),
	}

	// For PR strategy, pre-fetch closing PRs for cycle time pipeline.
	if cfg.CycleTime.Strategy == model.StrategyPR {
		mergedPRs, prErr := client.SearchPRs(ctx, prQuery.Build())
		if prErr != nil {
			log.Warn("could not search merged PRs for cycle-time: %v", prErr)
		} else {
			cyclePipeline.ClosingPRs = metrics.BuildClosingPRMap(ctx, client, mergedPRs)
		}
	}

	// --- GatherData concurrently ---
	var (
		warnings []string
		mu       sync.Mutex
	)
	leadOK, cycleOK, throughputOK := true, true, true

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	g.Go(func() error {
		if err := leadPipeline.GatherData(gctx); err != nil {
			mu.Lock()
			warnings = append(warnings, fmt.Sprintf("lead time: %v", err))
			leadOK = false
			mu.Unlock()
		}
		return nil
	})

	g.Go(func() error {
		if err := cyclePipeline.GatherData(gctx); err != nil {
			mu.Lock()
			warnings = append(warnings, fmt.Sprintf("cycle time: %v", err))
			cycleOK = false
			mu.Unlock()
		}
		return nil
	})

	g.Go(func() error {
		if err := throughputPipeline.GatherData(gctx); err != nil {
			mu.Lock()
			warnings = append(warnings, fmt.Sprintf("throughput: %v", err))
			throughputOK = false
			mu.Unlock()
		}
		return nil
	})

	_ = g.Wait()

	// --- ProcessData ---
	result := model.StatsResult{
		Repository: repo,
		Since:      since,
		Until:      until,
		Warnings:   warnings,
	}

	if leadOK {
		if err := leadPipeline.ProcessData(); err != nil {
			log.Warn("lead time ProcessData: %v", err)
			result.Warnings = append(result.Warnings, fmt.Sprintf("lead time: %v", err))
		} else {
			result.LeadTime = &leadPipeline.Stats
		}
	}

	if cycleOK {
		if err := cyclePipeline.ProcessData(); err != nil {
			log.Warn("cycle time ProcessData: %v", err)
			result.Warnings = append(result.Warnings, fmt.Sprintf("cycle time: %v", err))
		} else {
			result.CycleTime = &cyclePipeline.Stats
			result.CycleTimeStrategy = cfg.CycleTime.Strategy
		}
	}
	// Always surface strategy so format layer can show N/A context.
	if result.CycleTimeStrategy == "" {
		result.CycleTimeStrategy = cfg.CycleTime.Strategy
	}

	if throughputOK {
		if err := throughputPipeline.ProcessData(); err != nil {
			log.Warn("throughput ProcessData: %v", err)
			result.Warnings = append(result.Warnings, fmt.Sprintf("throughput: %v", err))
		} else {
			result.Throughput = &model.StatsThroughput{
				IssuesClosed: throughputPipeline.Result.IssuesClosed,
				PRsMerged:    throughputPipeline.Result.PRsMerged,
			}
		}
	}

	// Quality: defect rate from categories (reuses lead time's closed issues)
	if leadOK && len(cfg.Quality.Categories) > 0 {
		result.Quality = computeQuality(leadPipeline.Items, cfg.Quality.Categories)
	}

	// TODO(PR C): WIP from project board or active_labels config

	// --- Render ---
	w, postFn := postIfEnabled(cmd, deps, client, posting.PostOptions{
		Command: "report",
		Context: dateutil.FormatContext(sinceFlag, untilFlag),
		Target:  posting.DiscussionTarget,
	})

	var fmtErr error
	switch deps.Format {
	case format.JSON:
		fmtErr = format.WriteReportJSON(w, result)
	case format.Markdown:
		fmtErr = format.WriteReportMarkdown(deps.RenderCtx(w), result)
	default:
		fmtErr = format.WriteReportPretty(deps.RenderCtx(w), result)
	}
	if fmtErr != nil {
		return fmtErr
	}
	return postFn()
}

// computeQuality computes defect rate from closed issues using the classifier.
// Issues classified as "bug" are counted as defects.
func computeQuality(items []leadtime.BulkItem, categories []model.CategoryConfig) *model.StatsQuality {
	if len(items) == 0 {
		return nil
	}
	classifier, err := classify.NewClassifier(categories)
	if err != nil {
		return nil
	}
	bugCount := 0
	for _, item := range items {
		result := classifier.Classify(classify.Input{
			Labels:    item.Issue.Labels,
			IssueType: item.Issue.IssueType,
			Title:     item.Issue.Title,
		})
		if result.Category == "bug" {
			bugCount++
		}
	}
	defectRate := float64(bugCount) / float64(len(items))
	return &model.StatsQuality{
		BugCount:    bugCount,
		TotalIssues: len(items),
		DefectRate:  defectRate,
	}
}
