package metrics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// ComputeWIPStageCounts groups items by stage and counts per matcher within each stage.
// Returns sorted: In Progress first, then In Review.
func ComputeWIPStageCounts(items []model.WIPItem, inProgressMatchers, inReviewMatchers []string) []model.WIPStageCount {
	// Build ordered matcher lists per stage.
	stageMatchers := map[string][]string{
		"In Progress": appendNativeSignals(inProgressMatchers, "draft"),
		"In Review":   appendNativeSignals(inReviewMatchers, "open-pr"),
	}

	// Count items per stage+matcher.
	type key struct {
		stage, matcher string
	}
	counts := make(map[key]int)
	stageTotals := make(map[string]int)

	for _, item := range items {
		k := key{stage: item.Status, matcher: item.MatchedMatcher}
		counts[k]++
		stageTotals[item.Status]++
	}

	// Build result in stage order.
	stageOrder := []string{"In Progress", "In Review"}
	var result []model.WIPStageCount

	for _, stage := range stageOrder {
		total := stageTotals[stage]
		if total == 0 {
			continue
		}

		matchers := stageMatchers[stage]
		var matcherCounts []model.WIPMatcherCount
		for _, m := range matchers {
			c := counts[key{stage: stage, matcher: m}]
			if c == 0 {
				continue
			}
			matcherCounts = append(matcherCounts, model.WIPMatcherCount{
				Matcher: m,
				Label:   matcherDisplayLabel(m),
				Count:   c,
			})
		}

		result = append(result, model.WIPStageCount{
			Stage:         stage,
			Count:         total,
			MatcherCounts: matcherCounts,
		})
	}

	return result
}

// ComputeWIPAssignees aggregates WIP load per assignee.
// Items with multiple assignees count for each. Items with no assignees
// count under "unassigned". Returns top `limit` entries sorted by ItemCount
// descending, then Login ascending.
func ComputeWIPAssignees(items []model.WIPItem, limit int) []model.WIPAssignee {
	type accumulator struct {
		count       int
		totalEffort float64
		byStage     map[string]int
	}
	agg := make(map[string]*accumulator)

	for _, item := range items {
		logins := item.Assignees
		if len(logins) == 0 {
			logins = []string{"unassigned"}
		}
		for _, login := range logins {
			a, ok := agg[login]
			if !ok {
				a = &accumulator{byStage: make(map[string]int)}
				agg[login] = a
			}
			a.count++
			a.totalEffort += item.EffortValue
			a.byStage[item.Status]++
		}
	}

	result := make([]model.WIPAssignee, 0, len(agg))
	for login, a := range agg {
		result = append(result, model.WIPAssignee{
			Login:       login,
			ItemCount:   a.count,
			TotalEffort: a.totalEffort,
			ByStage:     a.byStage,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].ItemCount != result[j].ItemCount {
			return result[i].ItemCount > result[j].ItemCount
		}
		return result[i].Login < result[j].Login
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result
}

// ComputeWIPStaleness counts items by staleness level.
func ComputeWIPStaleness(items []model.WIPItem) model.WIPStaleness {
	var s model.WIPStaleness
	for _, item := range items {
		switch item.Staleness {
		case model.StalenessActive:
			s.Active++
		case model.StalenessAging:
			s.Aging++
		case model.StalenessStale:
			s.Stale++
		}
	}
	return s
}

// GenerateWIPInsights produces human-readable insights from a WIPResult.
func GenerateWIPInsights(result model.WIPResult) []model.Insight {
	var insights []model.Insight

	if len(result.Items) == 0 {
		return nil
	}

	// Count unique people.
	people := make(map[string]bool)
	for _, a := range result.Assignees {
		if a.Login != "unassigned" {
			people[a.Login] = true
		}
	}
	numPeople := len(people)

	// wip_capacity: "N items in progress across M people"
	if numPeople > 0 {
		insights = append(insights, model.Insight{
			Type:    "wip_capacity",
			Message: fmt.Sprintf("%d items in progress across %d people.", len(result.Items), numPeople),
		})
	} else {
		insights = append(insights, model.Insight{
			Type:    "wip_capacity",
			Message: fmt.Sprintf("%d items in progress (no assignees).", len(result.Items)),
		})
	}

	// wip_assignee_load: highest loaded person (only if >1 person).
	if numPeople > 1 && len(result.Assignees) > 0 {
		top := result.Assignees[0] // already sorted descending
		if top.Login != "unassigned" {
			insights = append(insights, model.Insight{
				Type:    "wip_assignee_load",
				Message: fmt.Sprintf("%s has %d items assigned, highest on team.", top.Login, top.ItemCount),
			})
		}
	}

	// wip_staleness: "X% of WIP is stale"
	total := len(result.Items)
	staleCount := result.Staleness.Stale
	if staleCount > 0 {
		pct := float64(staleCount) / float64(total) * 100
		insights = append(insights, model.Insight{
			Type:    "wip_staleness",
			Message: fmt.Sprintf("%.0f%% of WIP is stale (%d items with no activity for 7+ days).", pct, staleCount),
		})
	}

	// wip_stage_health: observations about stage distribution.
	for _, sc := range result.StageCounts {
		if sc.Stage == "In Review" && sc.Count > 0 {
			reviewPct := float64(sc.Count) / float64(total) * 100
			if reviewPct > 50 {
				insights = append(insights, model.Insight{
					Type:    "wip_stage_health",
					Message: fmt.Sprintf("%.0f%% of WIP is in review — potential review bottleneck.", reviewPct),
				})
			}
		}
	}

	// wip_team_limit_exceeded
	if result.TeamLimit != nil && result.TotalEffort > *result.TeamLimit {
		insights = append(insights, model.Insight{
			Type:    "wip_team_limit_exceeded",
			Message: fmt.Sprintf("Team WIP (%.0f) exceeds limit (%.0f). Consider finishing items before starting new work.", result.TotalEffort, *result.TeamLimit),
		})
	}

	// wip_person_limit_exceeded
	if result.PersonLimit != nil {
		for _, a := range result.Assignees {
			if a.OverLimit {
				insights = append(insights, model.Insight{
					Type:    "wip_person_limit_exceeded",
					Message: fmt.Sprintf("%s has %.0f effort in progress (limit %.0f). Consider redistributing or finishing current work.", a.Login, a.TotalEffort, *result.PersonLimit),
				})
			}
		}
	}

	return insights
}

// --- helpers ---

// appendNativeSignals adds native signal matchers that won't be in config matchers.
func appendNativeSignals(matchers []string, signals ...string) []string {
	result := make([]string, len(matchers))
	copy(result, matchers)
	return append(result, signals...)
}

// matcherDisplayLabel converts a matcher string to a human-readable label.
func matcherDisplayLabel(m string) string {
	switch {
	case m == "draft":
		return "Draft PR"
	case m == "open-pr":
		return "Open PR"
	case strings.HasPrefix(m, "label:"):
		name := m[len("label:"):]
		// Title-case the label name.
		return strings.Title(strings.ReplaceAll(name, "-", " ")) //nolint:staticcheck // strings.Title is fine for display labels
	case strings.HasPrefix(m, "type:"):
		return m[len("type:"):]
	default:
		return m
	}
}
