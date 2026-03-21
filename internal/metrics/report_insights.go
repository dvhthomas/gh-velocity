package metrics

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// Insight thresholds — named constants for testability and discoverability.
const (
	SkewThreshold      = 3.0 // mean/median ratio to trigger skew warning
	BugRatioHigh       = 0.20
	BugRatioSuspicious = 0.60 // above this, suggest reviewing category matchers
	OutlierMinCount    = 2
	OutlierMultipleCap = 100 // cap the "Nx longer" message to avoid absurd numbers
	MismatchRatio      = 3.0 // PR:issue ratio to flag mismatch
	HotfixMaxHours     = 72
	MinItemsForInsight = 3 // need ≥3 items for statistical insights

	// Noise detection thresholds.
	NoiseMinCount    = 3
	NoiseMaxDuration = 60 * time.Second

	// Low-median diagnostic thresholds.
	LowMedianThreshold = time.Hour
	LowMedianMinCount  = 10

	// Extreme median threshold.
	ExtremeMedianThreshold = 365 * 24 * time.Hour

	// Fastest/slowest minimum duration to avoid picking spam.
	FastestSlotMinDuration = time.Minute
)

// ItemRef is a minimal representation of an issue/PR for insight generation.
// Conversion from pipeline-specific types happens in the cmd layer.
type ItemRef struct {
	Number   int
	Title    string
	Duration time.Duration
	Category string // populated by classifier when available
	URL      string // GitHub issue/PR URL for linked references
}

// CategoryMedian holds per-category duration summary for insight generation.
type CategoryMedian struct {
	Name   string
	Count  int
	Median time.Duration
}

// GenerateStatsInsights produces insights from aggregate duration statistics.
// Used by lead time and cycle time (shared rules: outlier, skew, fastest/slowest, category comparison).
func GenerateStatsInsights(stats model.Stats, section string, items []ItemRef) []model.Insight {
	var insights []model.Insight

	if stats.Count == 0 {
		return nil
	}

	// Noise detection — many sub-minute items suggest spam/automation.
	var noiseDetected bool
	if len(items) > 0 {
		var noiseCount int
		for _, item := range items {
			if item.Duration > 0 && item.Duration < NoiseMaxDuration {
				noiseCount++
			}
		}
		if noiseCount >= NoiseMinCount {
			noiseDetected = true
			insights = append(insights, model.Insight{
				Type:    "noise_detection",
				Message: fmt.Sprintf("%d issues closed in under 60 seconds — consider narrowing scope to exclude noise (e.g., `scope: '-label:invalid'`).", noiseCount),
			})
		}
	}

	// Outlier detection — express in terms of median multiples, capped at 100x.
	if stats.OutlierCount >= OutlierMinCount && stats.OutlierCutoff != nil && stats.Median != nil && *stats.Median > 0 {
		multiple := max(int(math.Round(float64(*stats.OutlierCutoff)/float64(*stats.Median))), 2)
		if multiple > OutlierMultipleCap {
			insights = append(insights, model.Insight{
				Type: "outlier_detection",
				Message: fmt.Sprintf(
					"%d items took over %dx longer than the median (%s).",
					stats.OutlierCount, OutlierMultipleCap, fmtDur(*stats.Median)),
			})
		} else {
			insights = append(insights, model.Insight{
				Type: "outlier_detection",
				Message: fmt.Sprintf(
					"%d items took %dx longer than the median (%s).",
					stats.OutlierCount, multiple, fmtDur(*stats.Median)),
			})
		}
	}

	// Low-median diagnostic — median under 1h with enough data suggests noise distortion.
	// Suppressed when noise_detection already fired (same root cause).
	if !noiseDetected && stats.Median != nil && *stats.Median < LowMedianThreshold && stats.Count > LowMedianMinCount {
		insights = append(insights, model.Insight{
			Type:    "low_median",
			Message: fmt.Sprintf("Median is %s — likely distorted by noise issues closed in seconds.", fmtDur(*stats.Median)),
		})
	}

	// Predictability — surface CV when delivery times vary notably.
	if stats.StdDev != nil && stats.Mean != nil && *stats.Mean > 0 {
		cv := float64(*stats.StdDev) / float64(*stats.Mean)
		cv = math.Round(cv*10) / 10
		switch {
		case cv > 1.0:
			insights = append(insights, model.Insight{
				Type:    "predictability",
				Message: fmt.Sprintf("Delivery times vary widely (CV %.1f) — the median is a more reliable estimate than the mean.", cv),
			})
		case cv >= 0.5:
			insights = append(insights, model.Insight{
				Type:    "predictability",
				Message: fmt.Sprintf("Moderate delivery time variability (CV %.1f) — some items take significantly longer than others.", cv),
			})
		}
	}

	// Skew warning.
	if stats.Mean != nil && stats.Median != nil && *stats.Median > 0 {
		ratio := float64(*stats.Mean) / float64(*stats.Median)
		if ratio > SkewThreshold {
			insights = append(insights, model.Insight{
				Type:    "skew_warning",
				Message: fmt.Sprintf("Mean (%s) is much higher than median (%s) — a few slow items are pulling the average up.", fmtDur(*stats.Mean), fmtDur(*stats.Median)),
			})
		}
	}

	// Fastest/slowest callout — show spread and suggest investigation.
	if stats.Count >= MinItemsForInsight && len(items) >= MinItemsForInsight {
		fastest, slowest := findExtremes(items)
		if fastest != nil && slowest != nil && fastest.Number != slowest.Number && fastest.Duration > 0 {
			spread := int(slowest.Duration / fastest.Duration)
			msg := fmt.Sprintf("%s ranges from %s to %s",
				section, fmtDur(fastest.Duration), fmtDur(slowest.Duration))
			if spread >= 10 {
				msg += fmt.Sprintf(" (%dx spread) — investigate %s for process bottlenecks.", spread, fmtItemRef(slowest))
			} else {
				msg += "."
			}
			insights = append(insights, model.Insight{
				Type:    "fastest_slowest",
				Message: msg,
			})
		}
	}

	// Per-category comparison.
	cats := ComputeCategoryMedians(items)
	if len(cats) >= 2 {
		// Find the fastest and slowest categories.
		fastest := cats[0]
		slowest := cats[0]
		for _, c := range cats[1:] {
			if c.Median < fastest.Median {
				fastest = c
			}
			if c.Median > slowest.Median {
				slowest = c
			}
		}
		if fastest.Name != slowest.Name {
			insights = append(insights, model.Insight{
				Type: "category_comparison",
				Message: fmt.Sprintf("%s (median %s) faster than %s (median %s).",
					fastest.Name, fmtDur(fastest.Median), slowest.Name, fmtDur(slowest.Median)),
			})
		}
	}

	// Extreme median — flag when median exceeds 1 year (likely backlog cleanup).
	if stats.Median != nil && *stats.Median > ExtremeMedianThreshold {
		insights = append(insights, model.Insight{
			Type:    "extreme_median",
			Message: fmt.Sprintf("Median is %s — likely includes backlog cleanup alongside recent work.", fmtDur(*stats.Median)),
		})
	}

	return insights
}

// GenerateCycleTimeInsights produces cycle-time-specific insights.
// When stats is non-nil, delegates to GenerateStatsInsights for shared rules,
// then adds strategy-specific insights. Pass leadTimeMedian when both metrics
// are available (e.g., in report) for a cycle-vs-lead comparison.
func GenerateCycleTimeInsights(stats *model.Stats, strategy string, items []ItemRef, leadTimeMedian ...time.Duration) []model.Insight {
	// No data — strategy-specific guidance.
	if stats == nil {
		switch strategy {
		case model.StrategyIssue:
			return []model.Insight{{
				Type:    "no_data",
				Message: "No cycle time data — configure lifecycle.in-progress.match for cycle time.",
			}}
		case model.StrategyPR:
			return []model.Insight{{
				Type:    "no_data",
				Message: "No cycle time data — no issues had a closing PR. Ensure PRs reference issues with 'closes #N'.",
			}}
		default:
			return []model.Insight{{
				Type:    "no_data",
				Message: "No cycle time data — set cycle_time.strategy in .gh-velocity.yml to enable. See /guides/cycle-time-setup/",
			}}
		}
	}

	// Shared stats insights (outlier, skew, fastest/slowest, category).
	insights := GenerateStatsInsights(*stats, "Cycle Time", items)

	// Cycle-vs-lead comparison when both are available.
	if stats.Median != nil && len(leadTimeMedian) > 0 && leadTimeMedian[0] > 0 {
		ltMedian := leadTimeMedian[0]
		ctMedian := *stats.Median
		if ltMedian > 0 {
			pct := int(float64(ctMedian) / float64(ltMedian) * 100)
			insights = append(insights, model.Insight{
				Type:    "cycle_vs_lead",
				Message: fmt.Sprintf("Cycle time (median %s) is %d%% of lead time (median %s) — %d%% of time is spent before active work begins.", fmtDur(ctMedian), pct, fmtDur(ltMedian), 100-pct),
			})
		}
	}

	return insights
}

// GenerateThroughputInsights produces throughput-specific insights.
func GenerateThroughputInsights(issuesClosed, prsMerged int, categoryDist map[string]int) []model.Insight {
	var insights []model.Insight

	// Zero activity.
	if issuesClosed == 0 && prsMerged == 0 {
		return []model.Insight{{
			Type:    "zero_activity",
			Message: "No issues closed or PRs merged — verify the time window (--since) and scope filter match your workflow.",
		}}
	}

	// Issue/PR mismatch.
	if prsMerged > 0 && issuesClosed == 0 {
		insights = append(insights, model.Insight{
			Type:    "issue_pr_mismatch",
			Message: fmt.Sprintf("%d PRs merged but 0 issues closed — PRs may not be linked to issues.", prsMerged),
		})
	} else if issuesClosed > 0 && float64(prsMerged)/float64(issuesClosed) > MismatchRatio {
		insights = append(insights, model.Insight{
			Type:    "issue_pr_mismatch",
			Message: fmt.Sprintf("%d PRs merged vs %d issues closed — PRs may not be linked to issues.", prsMerged, issuesClosed),
		})
	}

	return insights
}

// GenerateQualityInsights produces quality-specific insights.
// bugRatioThreshold is the configured threshold for the "high bug ratio" insight (e.g. 0.20 = 20%).
func GenerateQualityInsights(quality model.StatsQuality, items []ItemRef, hotfixWindowHours int, bugRatioThreshold float64) []model.Insight {
	var insights []model.Insight

	if quality.TotalIssues == 0 {
		return nil
	}

	// Bug ratio threshold — >60% suggests category matcher issues, not real bugs.
	switch {
	case quality.BugRatio > BugRatioSuspicious:
		insights = append(insights, model.Insight{
			Type:    "bug_ratio_review",
			Message: fmt.Sprintf("%.0f%% bug ratio — may reflect issue template naming rather than actual bugs. Review category matchers.", quality.BugRatio*100),
		})
	case quality.BugRatio > bugRatioThreshold:
		insights = append(insights, model.Insight{
			Type:    "bug_ratio_high",
			Message: fmt.Sprintf("%.0f%% of closed issues are bugs (above configured %.0f%% threshold).", quality.BugRatio*100, bugRatioThreshold*100),
		})
	}

	// Bug fix speed comparison.
	var bugDurs, nonBugDurs []time.Duration
	for _, item := range items {
		if item.Category == "bug" {
			bugDurs = append(bugDurs, item.Duration)
		} else if item.Category != "" {
			nonBugDurs = append(nonBugDurs, item.Duration)
		}
	}
	if len(bugDurs) >= 2 && len(nonBugDurs) >= 2 {
		bugMedian := medianDuration(bugDurs)
		nonBugMedian := medianDuration(nonBugDurs)
		if bugMedian < nonBugMedian {
			insights = append(insights, model.Insight{
				Type:    "bug_fix_speed",
				Message: fmt.Sprintf("Bug fixes (median %s) faster than other work (median %s).", fmtDur(bugMedian), fmtDur(nonBugMedian)),
			})
		} else if bugMedian > nonBugMedian {
			insights = append(insights, model.Insight{
				Type:    "bug_fix_speed",
				Message: fmt.Sprintf("Bug fixes (median %s) slower than other work (median %s).", fmtDur(bugMedian), fmtDur(nonBugMedian)),
			})
		}
	}

	// Hotfix detection — bug fixes resolved within the configured window.
	// Two separate insights:
	//   1. Responsiveness: what % of bugs were hotfixed? (high = good)
	//   2. Burden: what % of all work was hotfixes? (high = concerning)
	if hotfixWindowHours > 0 && quality.BugCount > 0 {
		window := time.Duration(hotfixWindowHours) * time.Hour
		var hotfixCount int
		for _, item := range items {
			if item.Category == "bug" && item.Duration > 0 && item.Duration < window {
				hotfixCount++
			}
		}
		if hotfixCount > 0 {
			// Responsiveness: how quickly are bugs being fixed?
			pctOfBugs := hotfixCount * 100 / quality.BugCount
			insights = append(insights, model.Insight{
				Type:    "hotfix_responsiveness",
				Message: fmt.Sprintf("%d of %d bugs (%d%%) were hotfixed within %dh.", hotfixCount, quality.BugCount, pctOfBugs, hotfixWindowHours),
			})

			// Burden: how much total capacity is spent on urgent bug fixes?
			pctOfAll := hotfixCount * 100 / quality.TotalIssues
			if pctOfAll > 10 {
				insights = append(insights, model.Insight{
					Type:    "hotfix_burden",
					Message: fmt.Sprintf("Hotfixes account for %d%% of all work (%d of %d items) — consider investing in prevention.", pctOfAll, hotfixCount, quality.TotalIssues),
				})
			}
		}
	}

	return insights
}

// ComputeCategoryMedians groups items by category and computes median duration per group.
// Items without a category are excluded. Results are sorted by count descending.
func ComputeCategoryMedians(items []ItemRef) []CategoryMedian {
	groups := make(map[string][]time.Duration)
	for _, item := range items {
		if item.Category == "" {
			continue
		}
		groups[item.Category] = append(groups[item.Category], item.Duration)
	}

	if len(groups) == 0 {
		return nil
	}

	result := make([]CategoryMedian, 0, len(groups))
	for name, durs := range groups {
		result = append(result, CategoryMedian{
			Name:   name,
			Count:  len(durs),
			Median: medianDuration(durs),
		})
	}

	// Sort by count descending for consistent output.
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Name < result[j].Name
	})

	return result
}

// --- helpers ---

// findExtremes returns the items with the minimum and maximum duration.
// Items with duration < FastestSlotMinDuration are excluded to avoid
// picking spam/noise as the "fastest" item.
func findExtremes(items []ItemRef) (*ItemRef, *ItemRef) {
	// Filter out sub-minute items.
	var eligible []ItemRef
	for _, item := range items {
		if item.Duration >= FastestSlotMinDuration {
			eligible = append(eligible, item)
		}
	}
	if len(eligible) == 0 {
		return nil, nil
	}
	fastest := &eligible[0]
	slowest := &eligible[0]
	for i := range eligible {
		if eligible[i].Duration < fastest.Duration {
			fastest = &eligible[i]
		}
		if eligible[i].Duration > slowest.Duration {
			slowest = &eligible[i]
		}
	}
	return fastest, slowest
}

// medianDuration computes the median of a duration slice. Sorts in place.
func medianDuration(durs []time.Duration) time.Duration {
	if len(durs) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(durs))
	copy(sorted, durs)
	slices.Sort(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// fmtDur formats a duration for insight messages. Uses the largest two units.
// Mirrors format.FormatDuration (metrics cannot import format to avoid cycles).
func fmtDur(d time.Duration) string {
	if d < 0 {
		return "-" + fmtDur(-d)
	}
	if d == 0 {
		return "0s"
	}

	totalDays := int(d.Hours()) / 24
	years := totalDays / 365
	days := totalDays % 365
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	switch {
	case years > 0:
		return fmt.Sprintf("%dy %dd", years, days)
	case totalDays > 0:
		return fmt.Sprintf("%dd %dh", totalDays, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// fmtItemRef formats an issue/PR reference with truncated title.
// Uses markdown link when URL is available: [#N title…](url).
func fmtItemRef(r *ItemRef) string {
	title := truncate(r.Title, 40)
	if r.URL != "" {
		return fmt.Sprintf("[#%d %s](%s)", r.Number, title, r.URL)
	}
	return fmt.Sprintf("#%d %s", r.Number, title)
}

// truncate shortens a string to maxLen, adding "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
