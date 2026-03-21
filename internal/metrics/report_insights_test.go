package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// helpers
func dur(days int) time.Duration            { return time.Duration(days) * 24 * time.Hour }
func durH(hours int) time.Duration          { return time.Duration(hours) * time.Hour }
func ptrDur(d time.Duration) *time.Duration { return &d }

// assertInsights checks insight count and optional substring match.
func assertInsights(t *testing.T, got []model.Insight, wantCount int, wantSubstr string) {
	t.Helper()
	if len(got) != wantCount {
		t.Errorf("got %d insights, want %d", len(got), wantCount)
		for i, ins := range got {
			t.Logf("  [%d] type=%s msg=%s", i, ins.Type, ins.Message)
		}
		return
	}
	if wantSubstr != "" {
		found := false
		for _, ins := range got {
			if strings.Contains(ins.Message, wantSubstr) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no insight contains %q", wantSubstr)
			for i, ins := range got {
				t.Logf("  [%d] type=%s msg=%s", i, ins.Type, ins.Message)
			}
		}
	}
}

// assertHasType checks that at least one insight has the given type.
func assertHasType(t *testing.T, got []model.Insight, wantType string) {
	t.Helper()
	for _, ins := range got {
		if ins.Type == wantType {
			return
		}
	}
	t.Errorf("no insight has type %q", wantType)
	for i, ins := range got {
		t.Logf("  [%d] type=%s msg=%s", i, ins.Type, ins.Message)
	}
}

// --- GenerateStatsInsights ---

func TestGenerateStatsInsights(t *testing.T) {
	tests := []struct {
		name       string
		stats      model.Stats
		items      []ItemRef
		wantCount  int
		wantSubstr string
		wantType   string
	}{
		{
			name:  "empty stats produces no insights",
			stats: model.Stats{},
		},
		{
			name:  "too few items skips fastest/slowest",
			stats: model.Stats{Count: 2, Mean: ptrDur(dur(10)), Median: ptrDur(dur(5))},
			items: []ItemRef{
				{Number: 1, Title: "A", Duration: dur(5)},
				{Number: 2, Title: "B", Duration: dur(10)},
			},
			wantCount: 0,
		},
		{
			name: "outliers at threshold fires with median multiple",
			stats: model.Stats{
				Count:         10,
				OutlierCount:  2,
				OutlierCutoff: ptrDur(dur(30)),
				Mean:          ptrDur(dur(10)),
				Median:        ptrDur(dur(5)),
			},
			wantCount:  1,
			wantSubstr: "2 items took",
			wantType:   "outlier_detection",
		},
		{
			name: "outliers below threshold silent",
			stats: model.Stats{
				Count:         10,
				OutlierCount:  1,
				OutlierCutoff: ptrDur(dur(30)),
				Mean:          ptrDur(dur(10)),
				Median:        ptrDur(dur(5)),
			},
			wantCount: 0,
		},
		{
			name: "skew warning fires when mean >> median",
			stats: model.Stats{
				Count:  10,
				Mean:   ptrDur(dur(90)),
				Median: ptrDur(dur(10)),
			},
			wantCount:  1,
			wantSubstr: "pulling the average up",
			wantType:   "skew_warning",
		},
		{
			name: "skew warning silent when ratio below threshold",
			stats: model.Stats{
				Count:  10,
				Mean:   ptrDur(dur(10)),
				Median: ptrDur(dur(5)),
			},
			wantCount: 0,
		},
		{
			name:  "fastest/slowest callout with enough items",
			stats: model.Stats{Count: 5, Mean: ptrDur(dur(10)), Median: ptrDur(dur(8))},
			items: []ItemRef{
				{Number: 1, Title: "Quick fix", Duration: durH(2)},
				{Number: 2, Title: "Medium", Duration: dur(5)},
				{Number: 3, Title: "Slow one", Duration: dur(30)},
				{Number: 4, Title: "Another", Duration: dur(7)},
				{Number: 5, Title: "Last", Duration: dur(3)},
			},
			wantCount:  1,
			wantSubstr: "ranges from",
			wantType:   "fastest_slowest",
		},
		{
			name:  "per-category comparison with two categories",
			stats: model.Stats{Count: 6, Mean: ptrDur(dur(10)), Median: ptrDur(dur(8))},
			items: []ItemRef{
				{Number: 1, Title: "Bug 1", Duration: dur(2), Category: "bug"},
				{Number: 2, Title: "Bug 2", Duration: dur(3), Category: "bug"},
				{Number: 3, Title: "Bug 3", Duration: dur(4), Category: "bug"},
				{Number: 4, Title: "Feat 1", Duration: dur(10), Category: "feature"},
				{Number: 5, Title: "Feat 2", Duration: dur(15), Category: "feature"},
				{Number: 6, Title: "Feat 3", Duration: dur(20), Category: "feature"},
			},
			wantCount:  2, // fastest/slowest + category_comparison
			wantSubstr: "bug",
			wantType:   "category_comparison",
		},
		{
			name:  "per-category skipped with single category",
			stats: model.Stats{Count: 3, Mean: ptrDur(dur(5)), Median: ptrDur(dur(5))},
			items: []ItemRef{
				{Number: 1, Title: "A", Duration: dur(3), Category: "bug"},
				{Number: 2, Title: "B", Duration: dur(5), Category: "bug"},
				{Number: 3, Title: "C", Duration: dur(7), Category: "bug"},
			},
			wantCount: 1, // fastest/slowest only (count >= MinItemsForInsight)
			wantType:  "fastest_slowest",
		},
		{
			name:  "per-category skipped when items have no category",
			stats: model.Stats{Count: 4, Mean: ptrDur(dur(5)), Median: ptrDur(dur(5))},
			items: []ItemRef{
				{Number: 1, Title: "A", Duration: dur(3)},
				{Number: 2, Title: "B", Duration: dur(5)},
				{Number: 3, Title: "C", Duration: dur(7)},
				{Number: 4, Title: "D", Duration: dur(9)},
			},
			wantCount: 1, // fastest/slowest only
			wantType:  "fastest_slowest",
		},
		{
			name: "multiple rules fire together",
			stats: model.Stats{
				Count:         10,
				Mean:          ptrDur(dur(200)),
				Median:        ptrDur(dur(10)),
				OutlierCount:  4,
				OutlierCutoff: ptrDur(dur(100)),
			},
			items: []ItemRef{
				{Number: 1, Title: "Fast", Duration: durH(1), Category: "bug"},
				{Number: 2, Title: "Med", Duration: dur(5), Category: "bug"},
				{Number: 3, Title: "Slow", Duration: dur(500), Category: "feature"},
				{Number: 4, Title: "X", Duration: dur(10), Category: "feature"},
				{Number: 5, Title: "Y", Duration: dur(20), Category: "feature"},
			},
			wantCount: 4, // outlier + skew + fastest/slowest + category
		},
		{
			name: "predictability low fires when CV > 1.0",
			stats: model.Stats{
				Count:  10,
				Mean:   ptrDur(dur(10)),
				Median: ptrDur(dur(5)),
				StdDev: ptrDur(dur(15)), // CV = 15/10 = 1.5
			},
			wantCount:  1,
			wantSubstr: "vary widely",
			wantType:   "predictability",
		},
		{
			name: "predictability moderate fires when CV 0.5-1.0",
			stats: model.Stats{
				Count:  10,
				Mean:   ptrDur(dur(10)),
				Median: ptrDur(dur(8)),
				StdDev: ptrDur(dur(7)), // CV = 7/10 = 0.7
			},
			wantCount:  1,
			wantSubstr: "Moderate delivery",
			wantType:   "predictability",
		},
		{
			name: "predictability silent when CV < 0.5",
			stats: model.Stats{
				Count:  10,
				Mean:   ptrDur(dur(10)),
				Median: ptrDur(dur(9)),
				StdDev: ptrDur(dur(3)), // CV = 3/10 = 0.3
			},
			wantCount: 0,
		},
		// --- Noise detection ---
		{
			name:  "noise detection fires when >= 3 sub-60s items",
			stats: model.Stats{Count: 5, Mean: ptrDur(dur(5)), Median: ptrDur(dur(5))},
			items: []ItemRef{
				{Number: 1, Title: "Spam 1", Duration: 10 * time.Second},
				{Number: 2, Title: "Spam 2", Duration: 20 * time.Second},
				{Number: 3, Title: "Spam 3", Duration: 30 * time.Second},
				{Number: 4, Title: "Real 1", Duration: dur(5)},
				{Number: 5, Title: "Real 2", Duration: dur(10)},
			},
			wantSubstr: "closed in under 60 seconds",
			wantType:   "noise_detection",
			wantCount:  2, // noise_detection + fastest_slowest
		},
		{
			name:  "noise detection silent when < 3 sub-60s items",
			stats: model.Stats{Count: 4, Mean: ptrDur(dur(5)), Median: ptrDur(dur(5))},
			items: []ItemRef{
				{Number: 1, Title: "Spam 1", Duration: 10 * time.Second},
				{Number: 2, Title: "Spam 2", Duration: 20 * time.Second},
				{Number: 3, Title: "Real 1", Duration: dur(5)},
				{Number: 4, Title: "Real 2", Duration: dur(10)},
			},
			wantCount: 1, // fastest_slowest only
			wantType:  "fastest_slowest",
		},
		// --- Outlier multiplier cap ---
		{
			name: "outlier multiplier capped at 100x",
			stats: model.Stats{
				Count:         50,
				Mean:          ptrDur(dur(100)),
				Median:        ptrDur(time.Minute), // 1 minute median
				OutlierCount:  10,
				OutlierCutoff: ptrDur(dur(200)), // 200 days / 1 min >> 100x
			},
			wantSubstr: "over 100x",
			wantType:   "outlier_detection",
			wantCount:  3, // outlier_detection + low_median + skew_warning
		},
		// --- Low-median diagnostic ---
		{
			name: "low-median fires when median < 1h and count > 10",
			stats: model.Stats{
				Count:  50,
				Mean:   ptrDur(durH(1)), // mean=1h, median=30s → ratio=120, skew fires
				Median: ptrDur(30 * time.Second),
			},
			wantSubstr: "likely distorted by noise",
			wantType:   "low_median",
			wantCount:  2, // low_median + skew_warning
		},
		{
			name: "low-median suppressed when noise detection fires",
			stats: model.Stats{
				Count:  50,
				Mean:   ptrDur(durH(1)),
				Median: ptrDur(30 * time.Second),
			},
			items: []ItemRef{
				{Number: 1, Title: "Spam 1", Duration: 10 * time.Second},
				{Number: 2, Title: "Spam 2", Duration: 20 * time.Second},
				{Number: 3, Title: "Spam 3", Duration: 30 * time.Second},
				{Number: 4, Title: "Real", Duration: dur(5)},
			},
			wantSubstr: "closed in under 60 seconds",
			wantType:   "noise_detection",
			wantCount:  2, // noise_detection + skew_warning (fastest/slowest skipped: only 1 eligible item)
		},
		// --- Fastest/slowest min-duration filter ---
		{
			name:  "fastest/slowest skips sub-minute items",
			stats: model.Stats{Count: 4, Mean: ptrDur(dur(5)), Median: ptrDur(dur(5))},
			items: []ItemRef{
				{Number: 1, Title: "Spam", Duration: 10 * time.Second},
				{Number: 2, Title: "Also spam", Duration: 30 * time.Second},
				{Number: 3, Title: "Real fast", Duration: dur(2)},
				{Number: 4, Title: "Real slow", Duration: dur(10)},
			},
			wantSubstr: "ranges from",
			wantType:   "fastest_slowest",
			wantCount:  1,
		},
		{
			name:  "fastest/slowest returns nil when all sub-minute",
			stats: model.Stats{Count: 3, Mean: ptrDur(30 * time.Second), Median: ptrDur(20 * time.Second)},
			items: []ItemRef{
				{Number: 1, Title: "A", Duration: 10 * time.Second},
				{Number: 2, Title: "B", Duration: 20 * time.Second},
				{Number: 3, Title: "C", Duration: 30 * time.Second},
			},
			wantCount: 1, // noise_detection fires (3 sub-60s)
			wantType:  "noise_detection",
		},
		// --- Extreme median ---
		{
			name: "extreme median fires when > 365 days",
			stats: model.Stats{
				Count:  10,
				Mean:   ptrDur(dur(500)),
				Median: ptrDur(dur(500)),
			},
			wantSubstr: "backlog cleanup",
			wantType:   "extreme_median",
			wantCount:  1,
		},
		{
			name: "extreme median silent when <= 365 days",
			stats: model.Stats{
				Count:  10,
				Mean:   ptrDur(dur(100)),
				Median: ptrDur(dur(100)),
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateStatsInsights(tt.stats, "Lead Time", tt.items)
			assertInsights(t, got, tt.wantCount, tt.wantSubstr)
			if tt.wantType != "" {
				assertHasType(t, got, tt.wantType)
			}
		})
	}
}

// --- GenerateCycleTimeInsights ---

func TestGenerateCycleTimeInsights(t *testing.T) {
	tests := []struct {
		name       string
		stats      *model.Stats
		strategy   string
		items      []ItemRef
		wantCount  int
		wantSubstr string
		wantType   string
	}{
		{
			name:       "nil stats with issue strategy suggests lifecycle config",
			stats:      nil,
			strategy:   model.StrategyIssue,
			wantCount:  1,
			wantSubstr: "lifecycle",
			wantType:   "no_data",
		},
		{
			name:       "nil stats with pr strategy mentions closing PRs",
			stats:      nil,
			strategy:   model.StrategyPR,
			wantCount:  1,
			wantSubstr: "closing PR",
			wantType:   "no_data",
		},
		{
			name:      "PR strategy without lead time has no comparison insight",
			stats:     &model.Stats{Count: 10, Mean: ptrDur(durH(4)), Median: ptrDur(durH(2))},
			strategy:  model.StrategyPR,
			wantCount: 0,
		},
		{
			name:      "issue strategy with data has no comparison insight",
			stats:     &model.Stats{Count: 10, Mean: ptrDur(dur(5)), Median: ptrDur(dur(3))},
			strategy:  model.StrategyIssue,
			wantCount: 0,
		},
		{
			name: "inherits outlier detection from stats insights",
			stats: &model.Stats{
				Count:         10,
				Mean:          ptrDur(dur(50)),
				Median:        ptrDur(dur(5)),
				OutlierCount:  3,
				OutlierCutoff: ptrDur(dur(30)),
			},
			strategy:   model.StrategyIssue,
			wantCount:  2, // outlier + skew
			wantSubstr: "3 items took",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateCycleTimeInsights(tt.stats, tt.strategy, tt.items)
			assertInsights(t, got, tt.wantCount, tt.wantSubstr)
			if tt.wantType != "" {
				assertHasType(t, got, tt.wantType)
			}
		})
	}
}

// --- GenerateThroughputInsights ---

func TestGenerateThroughputInsights(t *testing.T) {
	tests := []struct {
		name         string
		issuesClosed int
		prsMerged    int
		categoryDist map[string]int
		wantCount    int
		wantSubstr   string
		wantType     string
	}{
		{
			name:       "zero activity fires",
			wantCount:  1,
			wantSubstr: "No issues closed or PRs merged",
			wantType:   "zero_activity",
		},
		{
			name:         "PRs but no issues fires mismatch",
			prsMerged:    15,
			issuesClosed: 0,
			wantCount:    1,
			wantSubstr:   "not be linked",
			wantType:     "issue_pr_mismatch",
		},
		{
			name:         "high PR:issue ratio fires mismatch",
			prsMerged:    30,
			issuesClosed: 5,
			wantCount:    1,
			wantSubstr:   "30 PRs merged",
			wantType:     "issue_pr_mismatch",
		},
		{
			name:         "balanced ratio no mismatch",
			prsMerged:    10,
			issuesClosed: 8,
			wantCount:    0,
		},
		{
			name:         "per-category distribution removed (data not insight)",
			issuesClosed: 10,
			prsMerged:    5,
			categoryDist: map[string]int{"bug": 4, "feature": 5, "chore": 1},
			wantCount:    0,
		},
		{
			name:         "per-category skipped with single category",
			issuesClosed: 10,
			prsMerged:    5,
			categoryDist: map[string]int{"bug": 10},
			wantCount:    0,
		},
		{
			name:         "per-category skipped when nil",
			issuesClosed: 10,
			prsMerged:    5,
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateThroughputInsights(tt.issuesClosed, tt.prsMerged, tt.categoryDist)
			assertInsights(t, got, tt.wantCount, tt.wantSubstr)
			if tt.wantType != "" {
				assertHasType(t, got, tt.wantType)
			}
		})
	}
}

// --- GenerateQualityInsights ---

func TestGenerateQualityInsights(t *testing.T) {
	tests := []struct {
		name              string
		quality           model.StatsQuality
		items             []ItemRef
		hotfixWindowHours int
		bugRatioThreshold float64 // 0 means use default (0.20)
		wantCount         int
		wantSubstr        string
		wantType          string
	}{
		{
			name:      "empty quality no insights",
			quality:   model.StatsQuality{},
			wantCount: 0,
		},
		{
			name:       "high bug ratio fires",
			quality:    model.StatsQuality{BugCount: 8, TotalIssues: 20, BugRatio: 0.40},
			wantCount:  1,
			wantSubstr: "40%",
			wantType:   "bug_ratio_high",
		},
		{
			name:      "normal bug ratio silent",
			quality:   model.StatsQuality{BugCount: 2, TotalIssues: 20, BugRatio: 0.10},
			wantCount: 0,
		},
		{
			name:       "bug ratio >60% fires review insight instead of high",
			quality:    model.StatsQuality{BugCount: 35, TotalIssues: 45, BugRatio: 0.78},
			wantCount:  1,
			wantSubstr: "Review category matchers",
			wantType:   "bug_ratio_review",
		},
		{
			name:              "bug fix speed comparison",
			quality:           model.StatsQuality{BugCount: 3, TotalIssues: 6, BugRatio: 0.50},
			hotfixWindowHours: 72,
			items: []ItemRef{
				{Number: 1, Title: "Bug A", Duration: dur(1), Category: "bug"},
				{Number: 2, Title: "Bug B", Duration: dur(2), Category: "bug"},
				{Number: 3, Title: "Bug C", Duration: dur(3), Category: "bug"},
				{Number: 4, Title: "Feat A", Duration: dur(10), Category: "feature"},
				{Number: 5, Title: "Feat B", Duration: dur(15), Category: "feature"},
				{Number: 6, Title: "Feat C", Duration: dur(20), Category: "feature"},
			},
			wantCount:  3, // bug_ratio_high + bug_fix_speed + hotfix (3 bugs < 72h)
			wantSubstr: "Bug fixes",
			wantType:   "bug_fix_speed",
		},
		{
			name:              "category distribution shows percentages",
			quality:           model.StatsQuality{BugCount: 2, TotalIssues: 10, BugRatio: 0.20},
			hotfixWindowHours: 72,
			items: []ItemRef{
				{Number: 1, Duration: dur(5), Category: "bug"},
				{Number: 2, Duration: dur(5), Category: "bug"},
				{Number: 3, Duration: dur(5), Category: "feature"},
				{Number: 4, Duration: dur(5), Category: "feature"},
				{Number: 5, Duration: dur(5), Category: "feature"},
				{Number: 6, Duration: dur(5), Category: "feature"},
				{Number: 7, Duration: dur(5), Category: "feature"},
				{Number: 8, Duration: dur(5), Category: "chore"},
				{Number: 9, Duration: dur(5), Category: "chore"},
				{Number: 10, Duration: dur(5), Category: "chore"},
			},
			wantCount: 0, // category_distribution removed; bug ratio exactly 20% not above threshold; all dur(5d) > 72h
		},
		{
			name:              "hotfix detection finds fast items",
			quality:           model.StatsQuality{BugCount: 2, TotalIssues: 5, BugRatio: 0.40},
			hotfixWindowHours: 72,
			items: []ItemRef{
				{Number: 1, Title: "Hotfix 1", Duration: durH(1), Category: "bug"},
				{Number: 2, Title: "Hotfix 2", Duration: durH(24), Category: "bug"},
				{Number: 3, Title: "Normal", Duration: dur(10), Category: "feature"},
				{Number: 4, Title: "Normal 2", Duration: dur(5), Category: "feature"},
				{Number: 5, Title: "Normal 3", Duration: dur(7), Category: "chore"},
			},
			wantCount:  3, // bug_ratio_high + bug_fix_speed + hotfix
			wantSubstr: "2 of 5 items (40%)",
			wantType:   "hotfix_count",
		},
		{
			name:              "no hotfixes when all items exceed window",
			quality:           model.StatsQuality{BugCount: 0, TotalIssues: 3, BugRatio: 0},
			hotfixWindowHours: 72,
			items: []ItemRef{
				{Number: 1, Duration: dur(10)},
				{Number: 2, Duration: dur(20)},
				{Number: 3, Duration: dur(30)},
			},
			wantCount: 0,
		},
		{
			name:              "custom threshold 15% fires at 20% bug ratio",
			quality:           model.StatsQuality{BugCount: 4, TotalIssues: 20, BugRatio: 0.20},
			bugRatioThreshold: 0.15,
			wantCount:         1,
			wantSubstr:        "configured 15%",
			wantType:          "bug_ratio_high",
		},
		{
			name:              "custom threshold 30% silent at 20% bug ratio",
			quality:           model.StatsQuality{BugCount: 4, TotalIssues: 20, BugRatio: 0.20},
			bugRatioThreshold: 0.30,
			wantCount:         0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.hotfixWindowHours
			if h == 0 {
				h = HotfixMaxHours // use default
			}
			threshold := tt.bugRatioThreshold
			if threshold == 0 {
				threshold = BugRatioHigh // use default
			}
			got := GenerateQualityInsights(tt.quality, tt.items, h, threshold)
			assertInsights(t, got, tt.wantCount, tt.wantSubstr)
			if tt.wantType != "" {
				assertHasType(t, got, tt.wantType)
			}
		})
	}
}

// --- ComputeCategoryMedians ---

func TestComputeCategoryMedians(t *testing.T) {
	tests := []struct {
		name      string
		items     []ItemRef
		wantCount int
	}{
		{
			name:      "nil items returns empty",
			wantCount: 0,
		},
		{
			name: "items without category returns empty",
			items: []ItemRef{
				{Number: 1, Duration: dur(5)},
				{Number: 2, Duration: dur(10)},
			},
			wantCount: 0,
		},
		{
			name: "single category returns one entry",
			items: []ItemRef{
				{Number: 1, Duration: dur(5), Category: "bug"},
				{Number: 2, Duration: dur(10), Category: "bug"},
			},
			wantCount: 1,
		},
		{
			name: "multiple categories sorted by count desc",
			items: []ItemRef{
				{Number: 1, Duration: dur(1), Category: "bug"},
				{Number: 2, Duration: dur(2), Category: "feature"},
				{Number: 3, Duration: dur(3), Category: "feature"},
				{Number: 4, Duration: dur(4), Category: "feature"},
			},
			wantCount: 2,
		},
		{
			name: "median computed correctly for odd count",
			items: []ItemRef{
				{Number: 1, Duration: dur(1), Category: "bug"},
				{Number: 2, Duration: dur(3), Category: "bug"},
				{Number: 3, Duration: dur(5), Category: "bug"},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeCategoryMedians(tt.items)
			if len(got) != tt.wantCount {
				t.Errorf("got %d categories, want %d", len(got), tt.wantCount)
			}
			// Verify median for the "median computed correctly" case.
			if tt.name == "median computed correctly for odd count" && len(got) == 1 {
				if got[0].Median != dur(3) {
					t.Errorf("median = %v, want %v", got[0].Median, dur(3))
				}
			}
		})
	}
}
