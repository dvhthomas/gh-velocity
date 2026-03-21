package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dvhthomas/gh-velocity/internal/classify"
	"github.com/dvhthomas/gh-velocity/internal/dateutil"
	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/metrics"
	"github.com/dvhthomas/gh-velocity/internal/model"
	cycletimepipe "github.com/dvhthomas/gh-velocity/internal/pipeline/cycletime"
	"github.com/dvhthomas/gh-velocity/internal/pipeline/leadtime"
	qualitypipe "github.com/dvhthomas/gh-velocity/internal/pipeline/quality"
	"github.com/dvhthomas/gh-velocity/internal/pipeline/throughput"
	"github.com/dvhthomas/gh-velocity/internal/pipeline/velocity"
	wippipe "github.com/dvhthomas/gh-velocity/internal/pipeline/wip"
	"github.com/dvhthomas/gh-velocity/internal/posting"
	"github.com/dvhthomas/gh-velocity/internal/scope"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// NewReportCmd returns the report command (composite dashboard).
func NewReportCmd() *cobra.Command {
	var (
		sinceFlag, untilFlag string
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
  gh velocity report --since 30d -R cli/cli -r json

  # Write all formats to a directory (single data-gathering pass)
  gh velocity report --since 30d --results md,json --write-to ./out`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd, sinceFlag, untilFlag, summaryOnly)
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Start of date window (default: 30d)")
	cmd.Flags().StringVar(&untilFlag, "until", "", "End of date window (default: now)")
	cmd.Flags().BoolVar(&summaryOnly, "summary-only", false, "Show only the summary dashboard table, omit per-item detail sections")

	return cmd
}

func runReport(cmd *cobra.Command, sinceFlag, untilFlag string, summaryOnly bool) error {
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
			deps.Warn("could not search merged PRs for cycle-time: %v", prErr)
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

	// Build open-item queries for WIP (reuses throughput's fetch to avoid extra API calls).
	var allLifecycleLabels []string
	for _, m := range append(cfg.Lifecycle.InProgress.Match, cfg.Lifecycle.InReview.Match...) {
		if strings.HasPrefix(m, "label:") {
			label := m[len("label:"):]
			allLifecycleLabels = append(allLifecycleLabels, label)
			throughputPipeline.OpenIssueQueries = append(throughputPipeline.OpenIssueQueries,
				scope.OpenIssueByLabelQuery(deps.Scope, label).Build())
			throughputPipeline.OpenPRQueries = append(throughputPipeline.OpenPRQueries,
				scope.OpenPRByLabelQuery(deps.Scope, label).Build())
		}
	}
	if len(allLifecycleLabels) > 0 {
		throughputPipeline.OpenPRQueries = append(throughputPipeline.OpenPRQueries,
			scope.OpenUnlabeledPRQuery(deps.Scope, allLifecycleLabels).Build())
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
				warnings = append(warnings, err.Error())
				velocityOK = false
				mu.Unlock()
			}
			return nil
		})
	}

	_ = g.Wait()

	// --- Enrich REST-sourced issues with IssueType when config uses type: matchers ---
	if len(cfg.Quality.Categories) > 0 {
		if enrichClassifier, cErr := classify.NewClassifier(cfg.Quality.Categories); cErr == nil && enrichClassifier.HasTypeMatchers() {
			if leadOK {
				if err := client.EnrichIssueTypes(ctx, leadPipeline.Issues); err != nil {
					deps.Warn("issue type enrichment (lead time): %v", err)
				}
			}
			if throughputOK {
				if err := client.EnrichIssueTypes(ctx, throughputPipeline.OpenIssues); err != nil {
					deps.Warn("issue type enrichment (throughput open): %v", err)
				}
			}
		}
	}

	// --- ProcessData ---
	result := model.StatsResult{
		Repository: repo,
		Since:      since,
		Until:      until,
		Warnings:   warnings,
	}

	if leadOK {
		if err := leadPipeline.ProcessData(); err != nil {
			deps.Warn("lead time ProcessData: %v", err)
			result.Warnings = append(result.Warnings, fmt.Sprintf("lead time: %v", err))
		} else {
			result.LeadTime = &leadPipeline.Stats
			result.LeadTimeInsights = leadPipeline.Insights
		}
	}

	if cycleOK {
		if err := cyclePipeline.ProcessData(); err != nil {
			deps.Warn("cycle time ProcessData: %v", err)
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
			deps.Warn("throughput ProcessData: %v", err)
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
			deps.Warn("velocity ProcessData: %v", err)
			result.Warnings = append(result.Warnings, fmt.Sprintf("velocity: %v", err))
		} else {
			result.Velocity = &velocityPipeline.Result
		}
	}

	// Quality: bug ratio + insights from categories (reuses lead time's closed issues)
	var qualDetail qualityResult
	if leadOK && len(cfg.Quality.Categories) > 0 {
		qualDetail = computeQualityWithInsights(leadPipeline.Items, cfg.Quality.Categories, cfg.Quality.HotfixWindowHours, cfg.Quality.BugRatioThreshold)
		result.Quality = qualDetail.Quality
		result.QualityInsights = qualDetail.Insights
	}

	// WIP: reuse throughput's open items when lifecycle labels are configured.
	var wipPipeline *wippipe.Pipeline
	wipOK := true
	hasLifecycleConfig := len(cfg.Lifecycle.InProgress.Match) > 0 || len(cfg.Lifecycle.InReview.Match) > 0
	if throughputOK && hasLifecycleConfig {
		wipPipeline = &wippipe.Pipeline{
			Owner:           deps.Owner,
			Repo:            deps.Repo,
			LifecycleConfig: cfg.Lifecycle,
			EffortConfig:    cfg.Effort,
			WIPConfig:       cfg.WIP,
			ExcludeUsers:    cfg.ExcludeUsers,
			Now:             now,
			Truncated:       client.SearchTruncated(),
			InjectedIssues:  throughputPipeline.OpenIssues,
			InjectedPRs:     throughputPipeline.OpenPRs,
		}
		if err := wipPipeline.ProcessData(); err != nil {
			deps.Warn("wip ProcessData: %v", err)
			result.Warnings = append(result.Warnings, fmt.Sprintf("wip: %v", err))
			wipOK = false
		} else {
			result.WIP = &wipPipeline.Result
		}
	}

	// Surface search truncation as a general warning.
	if client.SearchTruncated() {
		result.Warnings = append(result.Warnings,
			"Some search queries returned 1,000 results (GitHub API maximum). Data may be incomplete — narrow the date range or scope for complete results.")
	}

	// --- Render ---

	// renderReportToWriter writes the report in the given format to w,
	// including detail sections when applicable.
	var renderReportToWriter func(w io.Writer, f format.Format) error
	renderReportToWriter = func(w io.Writer, f format.Format) error {
		rc := format.RenderContext{
			Writer: w,
			Format: f,
			IsTTY:  deps.IsTTY,
			Width:  deps.TermWidth,
			Owner:  deps.Owner,
			Repo:   deps.Repo,
		}

		switch f {
		case format.JSON:
			return format.WriteReportJSON(w, result)
		case format.HTML:
			// Render the full report as markdown, then convert to HTML.
			var mdBuf bytes.Buffer
			if err := renderReportToWriter(&mdBuf, format.Markdown); err != nil {
				return err
			}
			title := fmt.Sprintf("Velocity Report: %s", result.Repository)
			return format.WriteReportHTML(w, mdBuf.String(), title)
		case format.Markdown:
			if err := format.WriteReportMarkdown(rc, result); err != nil {
				return err
			}
		default:
			if err := format.WriteReportPretty(rc, result); err != nil {
				return err
			}
		}

		// Detail sections: append per-item tables after the summary unless --summary-only.
		if !summaryOnly && f != format.JSON {
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
				var tpCats []throughput.CategoryRow
				for _, c := range qualDetail.Categories {
					tpCats = append(tpCats, throughput.CategoryRow{Name: c.Name, Count: c.Count, Pct: c.Pct})
				}
				if err := writeDetail(rc, summary, func() error {
					return throughput.WriteMarkdownWithCategories(rc.Writer, throughputPipeline.Result, throughputPipeline.SearchURL, throughputPipeline.Insights, tpCats)
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
					return velocity.WritePretty(rc, velocityPipeline.Result, false)
				}); err != nil {
					return err
				}
			}

			if qualDetail.Quality != nil && len(qualDetail.Items) > 0 {
				summary := fmt.Sprintf("Quality (%d issues)", qualDetail.Quality.TotalIssues)
				detail := qualitypipe.Detail{
					Repository: repo,
					Since:      since,
					Until:      until,
					Quality:    *qualDetail.Quality,
					Insights:   qualDetail.Insights,
					Items:      qualDetail.Items,
					Categories: qualDetail.Categories,
				}
				if err := writeDetail(rc, summary, func() error {
					return qualitypipe.WriteMarkdown(rc, detail)
				}, func() error {
					return qualitypipe.WritePretty(rc, detail)
				}); err != nil {
					return err
				}
			}

			if wipOK && wipPipeline != nil && len(wipPipeline.Result.Items) > 0 {
				summary := fmt.Sprintf("Work in Progress (%d items)", len(wipPipeline.Result.Items))
				if err := writeDetail(rc, summary, func() error {
					return format.WriteWIPDetailMarkdown(rc, wipPipeline.Result)
				}, func() error {
					return format.WriteWIPDetailPretty(rc, wipPipeline.Result)
				}); err != nil {
					return err
				}
			}
		}
		return nil
	}

	// Post setup: independent buffer for markdown content.
	pc, postFn := setupPost(cmd, deps, client, posting.PostOptions{
		Command: "report",
		Context: dateutil.FormatContext(sinceFlag, untilFlag),
		Target:  posting.DiscussionTarget,
	})

	prov := buildProvenance(cmd, map[string]string{"repository": deps.Owner + "/" + deps.Repo})

	if deps.Output.WriteTo != "" {
		// --write-to mode: render all formats to files, no stdout.

		// Capture markdown for post if needed.
		if pc != nil {
			if err := renderReportToWriter(&pc.buf, format.Markdown); err != nil {
				return err
			}
			writeProvenance(&pc.buf, format.Markdown, prov)
		}

		// Write requested formats.
		for _, f := range deps.Output.Results {
			name := "report." + formatExt(f)
			path := filepath.Join(deps.Output.WriteTo, name)
			if err := writeFileAtomic(path, func(w *os.File) error {
				if err := renderReportToWriter(w, f); err != nil {
					return err
				}
				writeProvenance(w, f, prov)
				return nil
			}); err != nil {
				return fmt.Errorf("writing %s: %w", path, err)
			}
		}

		// Per-section artifacts (report-specific).
		var sections []artifactSection
		if leadOK && leadPipeline.Stats.Count > 0 {
			sections = append(sections, leadTimeArtifact(leadPipeline))
		}
		if cycleOK && cyclePipeline.Stats.Count > 0 {
			sections = append(sections, cycleTimeArtifact(cyclePipeline, cfg.CycleTime.Strategy))
		}
		if throughputOK {
			sections = append(sections, throughputArtifact(throughputPipeline))
		}
		if wipOK && wipPipeline != nil && len(wipPipeline.Result.Items) > 0 {
			wipResult := wipPipeline.Result
			sections = append(sections, artifactSection{
				Name: "status-wip",
				WriteJSON: func(w *os.File) error {
					return format.WriteWIPDetailJSON(w, wipResult)
				},
				WriteMD: func(w *os.File, rc format.RenderContext) error {
					return format.WriteWIPDetailMarkdown(rc, wipResult)
				},
			})
		}
		if err := writeReportArtifacts(deps, deps.Output.WriteTo, sections); err != nil {
			return err
		}
	} else {
		// Stdout mode: single format.
		stdout := cmd.OutOrStdout()
		var w io.Writer = stdout
		if pc != nil {
			w = pc.postWriter(stdout)
		}
		f := deps.ResultFormat()
		if err := renderReportToWriter(w, f); err != nil {
			return err
		}
		writeProvenance(w, f, prov)
	}

	return postFn()
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

// artifactSection describes a per-section artifact that can be written as JSON and Markdown.
type artifactSection struct {
	Name      string                                          // filename stem, e.g. "flow-lead-time"
	WriteJSON func(w *os.File) error                          // writes JSON to the file
	WriteMD   func(w *os.File, rc format.RenderContext) error // writes Markdown to the file
}

// leadTimeArtifact creates an artifactSection for the lead-time pipeline.
func leadTimeArtifact(p *leadtime.BulkPipeline) artifactSection {
	repo := p.Owner + "/" + p.Repo
	return artifactSection{
		Name: "flow-lead-time",
		WriteJSON: func(w *os.File) error {
			return leadtime.WriteBulkJSON(w, repo, p.Since, p.Until, p.Items, p.Stats, p.SearchURL, p.Warnings, p.Insights)
		},
		WriteMD: func(w *os.File, rc format.RenderContext) error {
			return leadtime.WriteBulkMarkdown(rc, repo, p.Since, p.Until, p.Items, p.Stats, p.SearchURL, p.Insights)
		},
	}
}

// cycleTimeArtifact creates an artifactSection for the cycle-time pipeline.
func cycleTimeArtifact(p *cycletimepipe.BulkPipeline, strategy string) artifactSection {
	repo := p.Owner + "/" + p.Repo
	return artifactSection{
		Name: "flow-cycle-time",
		WriteJSON: func(w *os.File) error {
			return cycletimepipe.WriteBulkJSON(w, repo, p.Since, p.Until, strategy, p.Items, p.Stats, p.SearchURL, p.Warnings, p.Insights)
		},
		WriteMD: func(w *os.File, rc format.RenderContext) error {
			return cycletimepipe.WriteBulkMarkdown(rc, repo, p.Since, p.Until, strategy, p.Items, p.Stats, p.SearchURL, p.Insights)
		},
	}
}

// throughputArtifact creates an artifactSection for the throughput pipeline.
func throughputArtifact(p *throughput.Pipeline) artifactSection {
	return artifactSection{
		Name: "flow-throughput",
		WriteJSON: func(w *os.File) error {
			return throughput.WriteJSON(w, p.Result, p.SearchURL, p.Warnings, p.Insights)
		},
		WriteMD: func(w *os.File, rc format.RenderContext) error {
			return throughput.WriteMarkdown(w, p.Result, p.SearchURL, p.Insights)
		},
	}
}

// writeReportArtifacts writes per-section artifact files (e.g., flow-lead-time.json,
// flow-lead-time.md) to the given directory. Top-level report files (report.json,
// report.md) are handled by the caller. This is a pure rendering step — no API calls.
func writeReportArtifacts(deps *Deps, dir string, sections []artifactSection) error {
	// Per-section artifacts.
	for _, s := range sections {
		if err := writeFileAtomic(filepath.Join(dir, s.Name+".json"), func(w *os.File) error {
			return s.WriteJSON(w)
		}); err != nil {
			return fmt.Errorf("writing %s.json: %w", s.Name, err)
		}

		if err := writeFileAtomic(filepath.Join(dir, s.Name+".md"), func(w *os.File) error {
			mrc := deps.RenderCtx(w)
			return s.WriteMD(w, mrc)
		}); err != nil {
			return fmt.Errorf("writing %s.md: %w", s.Name, err)
		}
	}

	names := []string{"report.json", "report.md"}
	for _, s := range sections {
		names = append(names, s.Name+".json", s.Name+".md")
	}
	log.Debug("artifacts written to %s (%s)", dir, strings.Join(names, ", "))
	return nil
}

// computeQualityWithInsights computes bug ratio and quality insights from closed issues.
// Issues classified as "bug" are counted as defects. Returns both the quality stats
// and insight observations about bug ratio, bug fix speed, category distribution, and hotfixes.
// qualityResult holds quality computation output including per-item classification.
type qualityResult struct {
	Quality    *model.StatsQuality
	Insights   []model.Insight
	Items      []qualitypipe.QualityItem
	Categories []qualitypipe.CategoryRow
}

func computeQualityWithInsights(items []leadtime.BulkItem, categories []model.CategoryConfig, hotfixWindowHours, bugRatioThreshold float64) qualityResult {
	if len(items) == 0 {
		return qualityResult{}
	}
	classifier, err := classify.NewClassifier(categories)
	if err != nil {
		return qualityResult{}
	}

	// Classify all items and build ItemRef slices for insight generation.
	bugCount := 0
	var insightItems []metrics.ItemRef
	var qualityItems []qualitypipe.QualityItem
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
				URL:      item.Issue.URL,
			})
			qualityItems = append(qualityItems, qualitypipe.QualityItem{
				Number:      item.Issue.Number,
				Title:       item.Issue.Title,
				URL:         item.Issue.URL,
				Category:    cat,
				LeadTime:    format.FormatDuration(*dur),
				LeadTimeDur: dur,
			})
		}
	}

	bugRatio := float64(bugCount) / float64(len(items))
	quality := &model.StatsQuality{
		BugCount:    bugCount,
		TotalIssues: len(items),
		BugRatio:    bugRatio,
	}

	hwh := int(hotfixWindowHours)
	if hwh <= 0 {
		hwh = metrics.HotfixMaxHours
	}
	insights := metrics.GenerateQualityInsights(*quality, insightItems, hwh, bugRatioThreshold)
	return qualityResult{
		Quality:    quality,
		Insights:   insights,
		Items:      qualityItems,
		Categories: qualitypipe.BuildCategories(qualityItems),
	}
}
