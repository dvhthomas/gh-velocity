package metrics

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// Insight thresholds — named constants for testability and discoverability.
const (
	SkewThreshold      = 3.0 // mean/median ratio to trigger skew warning
	DefectRateHigh     = 0.20
	DefectRateSuspicious = 0.60 // above this, suggest reviewing category matchers
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
		multiple := int(math.Round(float64(*stats.OutlierCutoff) / float64(*stats.Median)))
		if multiple < 2 {
			multiple = 2
		}
		if multiple > OutlierMultipleCap {
			insights = append(insights, model.Insight{
				Type: "outlier_detection",
				Message: fmt.Sprintf(
					"%d items took %dx+ longer than the median (%s).",
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
				Message: fmt.Sprintf("Mean %s vs median %s — heavy right skew from %d outliers.", fmtDur(*stats.Mean), fmtDur(*stats.Median), stats.OutlierCount),
			})
		}
	}

	// Fastest/slowest callout.
	if stats.Count >= MinItemsForInsight && len(items) >= MinItemsForInsight {
		fastest, slowest := findExtremes(items)
		if fastest != nil && slowest != nil && fastest.Number != slowest.Number {
			insights = append(insights, model.Insight{
				Type: "fastest_slowest",
				Message: fmt.Sprintf("Fastest: #%d %s (%s). Slowest: #%d %s (%s).",
					fastest.Number, truncate(fastest.Title, 40), fmtDur(fastest.Duration),
					slowest.Number, truncate(slowest.Title, 40), fmtDur(slowest.Duration)),
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
// then adds strategy-specific insights.
func GenerateCycleTimeInsights(stats *model.Stats, strategy string, items []ItemRef) []model.Insight {
	// No data — strategy-specific guidance.
	if stats == nil {
		switch strategy {
		case model.StrategyIssue:
			return []model.Insight{{
				Type:    "no_data",
				Message: "No cycle time data — configure lifecycle.in-progress with project_status or label matchers.",
			}}
		case model.StrategyPR:
			return []model.Insight{{
				Type:    "no_data",
				Message: "No cycle time data — no issues had a closing PR. Ensure PRs reference issues with 'closes #N'.",
			}}
		default:
			return []model.Insight{{
				Type:    "no_data",
				Message: "No cycle time data available.",
			}}
		}
	}

	// Shared stats insights (outlier, skew, fastest/slowest, category).
	insights := GenerateStatsInsights(*stats, "Cycle Time", items)

	// PR strategy callout.
	if strategy == model.StrategyPR && stats.Median != nil {
		speed := "moderate"
		medianHours := stats.Median.Hours()
		if medianHours < 4 {
			speed = "fast"
		} else if medianHours > 48 {
			speed = "slow"
		}
		insights = append(insights, model.Insight{
			Type:    "strategy_callout",
			Message: fmt.Sprintf("PR cycle time median %s — %s review turnaround.", fmtDur(*stats.Median), speed),
		})
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
			Message: "No issues closed or PRs merged in this window.",
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

	// Per-category distribution.
	if len(categoryDist) >= 2 {
		total := 0
		for _, c := range categoryDist {
			total += c
		}
		if total > 0 {
			parts := formatCategoryDist(categoryDist, total)
			insights = append(insights, model.Insight{
				Type:    "category_distribution",
				Message: fmt.Sprintf("%d items: %s.", total, strings.Join(parts, ", ")),
			})
		}
	}

	return insights
}

// GenerateQualityInsights produces quality-specific insights.
func GenerateQualityInsights(quality model.StatsQuality, items []ItemRef, hotfixWindowHours int) []model.Insight {
	var insights []model.Insight

	if quality.TotalIssues == 0 {
		return nil
	}

	// Defect rate threshold — >60% suggests category matcher issues, not real bugs.
	switch {
	case quality.DefectRate > DefectRateSuspicious:
		insights = append(insights, model.Insight{
			Type:    "defect_rate_review",
			Message: fmt.Sprintf("%.0f%% defect rate — may reflect issue template naming rather than actual bugs. Review category matchers.", quality.DefectRate*100),
		})
	case quality.DefectRate > DefectRateHigh:
		insights = append(insights, model.Insight{
			Type:    "defect_rate_high",
			Message: fmt.Sprintf("%.0f%% defect rate — above typical 20%% threshold.", quality.DefectRate*100),
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

	// Category distribution.
	catCounts := make(map[string]int)
	for _, item := range items {
		if item.Category != "" {
			catCounts[item.Category]++
		}
	}
	if len(catCounts) >= 2 {
		total := 0
		for _, c := range catCounts {
			total += c
		}
		parts := formatCategoryDist(catCounts, total)
		insights = append(insights, model.Insight{
			Type:    "category_distribution",
			Message: fmt.Sprintf("%d items: %s.", total, strings.Join(parts, ", ")),
		})
	}

	// Hotfix detection.
	if hotfixWindowHours > 0 {
		window := time.Duration(hotfixWindowHours) * time.Hour
		var hotfixCount int
		for _, item := range items {
			if item.Duration > 0 && item.Duration < window {
				hotfixCount++
			}
		}
		if hotfixCount > 0 {
			insights = append(insights, model.Insight{
				Type:    "hotfix_count",
				Message: fmt.Sprintf("%d hotfixes (resolved within %dh of creation).", hotfixCount, hotfixWindowHours),
			})
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

// fmtDur formats a duration for insight messages. Compact human-readable format.
func fmtDur(d time.Duration) string {
	if d < 0 {
		return "-" + fmtDur(-d)
	}
	if d == 0 {
		return "0s"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// truncate shortens a string to maxLen, adding "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// formatCategoryDist builds sorted "N% name" parts from a category count map.
func formatCategoryDist(dist map[string]int, total int) []string {
	type catPct struct {
		name string
		pct  int
	}
	var cats []catPct
	for name, count := range dist {
		cats = append(cats, catPct{name: name, pct: count * 100 / total})
	}
	sort.Slice(cats, func(i, j int) bool {
		if cats[i].pct != cats[j].pct {
			return cats[i].pct > cats[j].pct
		}
		return cats[i].name < cats[j].name
	})
	parts := make([]string, len(cats))
	for i, c := range cats {
		parts[i] = fmt.Sprintf("%d%% %s", c.pct, c.name)
	}
	return parts
}
