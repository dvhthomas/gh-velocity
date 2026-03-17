package cmd

import (
	"fmt"
	"os"
	"path/filepath"
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
	"github.com/bitsbyme/gh-velocity/internal/pipeline/velocity"
	"github.com/bitsbyme/gh-velocity/internal/posting"
	"github.com/bitsbyme/gh-velocity/internal/scope"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// NewReportCmd returns the report command (composite dashboard).
func NewReportCmd() *cobra.Command {
	var (
		sinceFlag, untilFlag string
		artifactDir          string
		summaryOnly          bool
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
  gh velocity report --since 30d -R cli/cli -f json

  # Write all formats to a directory (single data-gathering pass)
  gh velocity report --since 30d --artifact-dir ./out`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd, sinceFlag, untilFlag, artifactDir, summaryOnly)
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Start of date window (default: 30d)")
	cmd.Flags().StringVar(&untilFlag, "until", "", "End of date window (default: now)")
	cmd.Flags().StringVar(&artifactDir, "artifact-dir", "", "Write report in all formats (json, markdown) to this directory")
	cmd.Flags().BoolVar(&summaryOnly, "summary-only", false, "Show only the summary dashboard table, omit per-item detail sections")

	return cmd
}

func runReport(cmd *cobra.Command, sinceFlag, untilFlag, artifactDir string, summaryOnly bool) error {
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
	setCycleTimeBatchParams(cyclePipeline, strat)

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
			deps.WarnUnlessJSON("could not search merged PRs for cycle-time: %v", prErr)
		} else {
			cyclePipeline.ClosingPRs = metrics.BuildClosingPRMap(ctx, client, mergedPRs)
		}
	}

	// Velocity pipeline — only if iteration strategy is configured.
	var velocityPipeline *velocity.Pipeline
	if cfg.Velocity.Iteration.Strategy != "" {
		velocityPipeline = &velocity.Pipeline{
			Client:         client,
			Owner:          deps.Owner,
			Repo:           deps.Repo,
			Config:         cfg.Velocity,
			ProjectConfig:  cfg.Project,
			Scope:          deps.Scope,
			ExcludeUsers:   deps.ExcludeUsers,
			Now:            now,
			IterationCount: cfg.Velocity.Iteration.Count,
			Since:          &since,
			Until:          &until,
		}
	}

	// --- GatherData concurrently ---
	var (
		warnings []string
		mu       sync.Mutex
	)
	leadOK, cycleOK, throughputOK, velocityOK := true, true, true, true

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

	if velocityPipeline != nil {
		g.Go(func() error {
			if err := velocityPipeline.GatherData(gctx); err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("velocity: %v", err))
				velocityOK = false
				mu.Unlock()
			}
			return nil
		})
	}

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
			deps.WarnUnlessJSON("lead time ProcessData: %v", err)
			result.Warnings = append(result.Warnings, fmt.Sprintf("lead time: %v", err))
		} else {
			result.LeadTime = &leadPipeline.Stats
			result.LeadTimeInsights = leadPipeline.Insights
		}
	}

	if cycleOK {
		if err := cyclePipeline.ProcessData(); err != nil {
			deps.WarnUnlessJSON("cycle time ProcessData: %v", err)
			result.Warnings = append(result.Warnings, fmt.Sprintf("cycle time: %v", err))
		} else {
			result.CycleTime = &cyclePipeline.Stats
			result.CycleTimeStrategy = cfg.CycleTime.Strategy
			result.CycleTimeInsights = cyclePipeline.Insights
		}
	}
	// Always surface strategy so format layer can show N/A context.
	if result.CycleTimeStrategy == "" {
		result.CycleTimeStrategy = cfg.CycleTime.Strategy
	}

	if throughputOK {
		if err := throughputPipeline.ProcessData(); err != nil {
			deps.WarnUnlessJSON("throughput ProcessData: %v", err)
			result.Warnings = append(result.Warnings, fmt.Sprintf("throughput: %v", err))
		} else {
			result.Throughput = &model.StatsThroughput{
				IssuesClosed: throughputPipeline.Result.IssuesClosed,
				PRsMerged:    throughputPipeline.Result.PRsMerged,
			}
			result.ThroughputInsights = throughputPipeline.Insights
		}
		// Surface partial-failure warnings (e.g., PR search rate-limited).
		result.Warnings = append(result.Warnings, throughputPipeline.Warnings...)
	}

	if velocityOK && velocityPipeline != nil {
		if err := velocityPipeline.ProcessData(); err != nil {
			deps.WarnUnlessJSON("velocity ProcessData: %v", err)
			result.Warnings = append(result.Warnings, fmt.Sprintf("velocity: %v", err))
		} else {
			result.Velocity = &velocityPipeline.Result
		}
	}

	// Quality: defect rate + insights from categories (reuses lead time's closed issues)
	if leadOK && len(cfg.Quality.Categories) > 0 {
		q, qInsights := computeQualityWithInsights(leadPipeline.Items, cfg.Quality.Categories, cfg.Quality.HotfixWindowHours)
		result.Quality = q
		result.QualityInsights = qInsights
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

	// Detail sections: append per-item tables after the summary unless --summary-only.
	if !summaryOnly && deps.Format != format.JSON {
		rc := deps.RenderCtx(w)
		fmt.Fprintln(rc.Writer)

		if leadOK && leadPipeline.Stats.Count > 0 {
			summary := fmt.Sprintf("Lead Time (%d issues)", leadPipeline.Stats.Count)
			if err := writeDetail(rc, summary, func() error {
				return leadtime.WriteBulkMarkdown(rc, repo, since, until, leadPipeline.Items, leadPipeline.Stats, leadPipeline.SearchURL, leadPipeline.Insights)
			}, func() error {
				return leadtime.WriteBulkPretty(rc, repo, since, until, leadPipeline.Items, leadPipeline.Stats, leadPipeline.SearchURL, leadPipeline.Insights)
			}); err != nil {
				return err
			}
		}

		if cycleOK && cyclePipeline.Stats.Count > 0 {
			summary := fmt.Sprintf("Cycle Time (%d items)", cyclePipeline.Stats.Count)
			if err := writeDetail(rc, summary, func() error {
				return cycletimepipe.WriteBulkMarkdown(rc, repo, since, until, cfg.CycleTime.Strategy, cyclePipeline.Items, cyclePipeline.Stats, cyclePipeline.SearchURL, cyclePipeline.Insights)
			}, func() error {
				return cycletimepipe.WriteBulkPretty(rc, repo, since, until, cfg.CycleTime.Strategy, cyclePipeline.Items, cyclePipeline.Stats, cyclePipeline.SearchURL, cyclePipeline.Insights)
			}); err != nil {
				return err
			}
		}

		if throughputOK && throughputPipeline.Result.IssuesClosed+throughputPipeline.Result.PRsMerged > 0 {
			total := throughputPipeline.Result.IssuesClosed + throughputPipeline.Result.PRsMerged
			summary := fmt.Sprintf("Throughput (%d items)", total)
			if err := writeDetail(rc, summary, func() error {
				return throughput.WriteMarkdown(rc.Writer, throughputPipeline.Result, throughputPipeline.SearchURL, throughputPipeline.Insights)
			}, func() error {
				return throughput.WritePretty(rc.Writer, throughputPipeline.Result, throughputPipeline.SearchURL, throughputPipeline.Insights)
			}); err != nil {
				return err
			}
		}

		if velocityOK && velocityPipeline != nil {
			if err := writeDetail(rc, "Velocity", func() error {
				return velocity.WriteMarkdown(rc.Writer, velocityPipeline.Result)
			}, func() error {
				return velocity.WritePretty(rc.Writer, velocityPipeline.Result, false)
			}); err != nil {
				return err
			}
		}
	}

	if err := postFn(); err != nil {
		return err
	}

	// Write artifacts — pure rendering from the already-computed result.
	if artifactDir != "" {
		if err := writeReportArtifacts(deps, artifactDir, result); err != nil {
			return err
		}
	}

	return nil
}

// writeDetail dispatches to the markdown or pretty renderer based on format.
// For markdown, wraps content in a collapsible <details> section.
func writeDetail(rc format.RenderContext, summary string, md, pretty func() error) error {
	if rc.Format != format.Markdown {
		return pretty()
	}
	fmt.Fprintf(rc.Writer, "<details>\n<summary>%s</summary>\n\n", summary)
	if err := md(); err != nil {
		return err
	}
	fmt.Fprintln(rc.Writer, "</details>")
	fmt.Fprintln(rc.Writer)
	return nil
}

// writeReportArtifacts writes report output in all formats to the given
// directory. This is a pure rendering step — no API calls.
func writeReportArtifacts(deps *Deps, dir string, result model.StatsResult) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating artifact dir: %w", err)
	}

	// JSON
	jsonFile, err := os.Create(filepath.Join(dir, "report.json"))
	if err != nil {
		return fmt.Errorf("creating report.json: %w", err)
	}
	defer jsonFile.Close()
	if err := format.WriteReportJSON(jsonFile, result); err != nil {
		return fmt.Errorf("writing report.json: %w", err)
	}

	// Markdown
	mdFile, err := os.Create(filepath.Join(dir, "report.md"))
	if err != nil {
		return fmt.Errorf("creating report.md: %w", err)
	}
	defer mdFile.Close()
	rctx := deps.RenderCtx(mdFile)
	if err := format.WriteReportMarkdown(rctx, result); err != nil {
		return fmt.Errorf("writing report.md: %w", err)
	}

	log.Debug("artifacts written to %s (report.json, report.md)", dir)
	return nil
}

// computeQualityWithInsights computes defect rate and quality insights from closed issues.
// Issues classified as "bug" are counted as defects. Returns both the quality stats
// and insight observations about defect rate, bug fix speed, category distribution, and hotfixes.
func computeQualityWithInsights(items []leadtime.BulkItem, categories []model.CategoryConfig, hotfixWindowHours float64) (*model.StatsQuality, []model.Insight) {
	if len(items) == 0 {
		return nil, nil
	}
	classifier, err := classify.NewClassifier(categories)
	if err != nil {
		return nil, nil
	}

	// Classify all items and build ItemRef slices for insight generation.
	bugCount := 0
	var insightItems []metrics.ItemRef
	for _, item := range items {
		result := classifier.Classify(classify.Input{
			Labels:    item.Issue.Labels,
			IssueType: item.Issue.IssueType,
			Title:     item.Issue.Title,
		})
		cat := result.Category()
		if cat == "bug" {
			bugCount++
		}
		dur := item.Metric.Duration
		if dur != nil {
			insightItems = append(insightItems, metrics.ItemRef{
				Number:   item.Issue.Number,
				Title:    item.Issue.Title,
				Duration: *dur,
				Category: cat,
			})
		}
	}

	defectRate := float64(bugCount) / float64(len(items))
	quality := &model.StatsQuality{
		BugCount:    bugCount,
		TotalIssues: len(items),
		DefectRate:  defectRate,
	}

	hwh := int(hotfixWindowHours)
	if hwh <= 0 {
		hwh = metrics.HotfixMaxHours
	}
	insights := metrics.GenerateQualityInsights(*quality, insightItems, hwh)
	return quality, insights
}
