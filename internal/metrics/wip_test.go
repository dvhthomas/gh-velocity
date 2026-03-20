package metrics

import (
	"strings"
	"testing"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestComputeWIPStageCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		items              []model.WIPItem
		inProgressMatchers []string
		inReviewMatchers   []string
		wantStages         []string
		wantCounts         []int
	}{
		{
			name:  "empty input",
			items: nil,
		},
		{
			name: "single stage with one matcher",
			items: []model.WIPItem{
				{Status: "In Progress", MatchedMatcher: "label:in-progress"},
				{Status: "In Progress", MatchedMatcher: "label:in-progress"},
			},
			inProgressMatchers: []string{"label:in-progress"},
			wantStages:         []string{"In Progress"},
			wantCounts:         []int{2},
		},
		{
			name: "multiple matchers per stage",
			items: []model.WIPItem{
				{Status: "In Progress", MatchedMatcher: "label:in-progress"},
				{Status: "In Progress", MatchedMatcher: "label:wip"},
				{Status: "In Review", MatchedMatcher: "label:in-review"},
			},
			inProgressMatchers: []string{"label:in-progress", "label:wip"},
			inReviewMatchers:   []string{"label:in-review"},
			wantStages:         []string{"In Progress", "In Review"},
			wantCounts:         []int{2, 1},
		},
		{
			name: "native signals included",
			items: []model.WIPItem{
				{Status: "In Progress", MatchedMatcher: "draft"},
				{Status: "In Review", MatchedMatcher: "open-pr"},
			},
			inProgressMatchers: []string{"label:in-progress"},
			inReviewMatchers:   []string{"label:in-review"},
			wantStages:         []string{"In Progress", "In Review"},
			wantCounts:         []int{1, 1},
		},
		{
			name: "in progress ordered before in review",
			items: []model.WIPItem{
				{Status: "In Review", MatchedMatcher: "label:in-review"},
				{Status: "In Progress", MatchedMatcher: "label:in-progress"},
			},
			inProgressMatchers: []string{"label:in-progress"},
			inReviewMatchers:   []string{"label:in-review"},
			wantStages:         []string{"In Progress", "In Review"},
			wantCounts:         []int{1, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ComputeWIPStageCounts(tt.items, tt.inProgressMatchers, tt.inReviewMatchers)

			if len(result) != len(tt.wantStages) {
				t.Fatalf("got %d stages, want %d", len(result), len(tt.wantStages))
			}
			for i, sc := range result {
				if sc.Stage != tt.wantStages[i] {
					t.Errorf("stage[%d] = %q, want %q", i, sc.Stage, tt.wantStages[i])
				}
				if sc.Count != tt.wantCounts[i] {
					t.Errorf("count[%d] = %d, want %d", i, sc.Count, tt.wantCounts[i])
				}
			}
		})
	}
}

func TestComputeWIPStageCounts_MatcherBreakdown(t *testing.T) {
	t.Parallel()

	items := []model.WIPItem{
		{Status: "In Progress", MatchedMatcher: "label:in-progress"},
		{Status: "In Progress", MatchedMatcher: "label:in-progress"},
		{Status: "In Progress", MatchedMatcher: "label:wip"},
	}

	result := ComputeWIPStageCounts(items, []string{"label:in-progress", "label:wip"}, nil)
	if len(result) != 1 {
		t.Fatalf("got %d stages, want 1", len(result))
	}
	if len(result[0].MatcherCounts) != 2 {
		t.Fatalf("got %d matcher counts, want 2", len(result[0].MatcherCounts))
	}
	// in-progress: 2, wip: 1
	for _, mc := range result[0].MatcherCounts {
		switch mc.Matcher {
		case "label:in-progress":
			if mc.Count != 2 {
				t.Errorf("in-progress count = %d, want 2", mc.Count)
			}
		case "label:wip":
			if mc.Count != 1 {
				t.Errorf("wip count = %d, want 1", mc.Count)
			}
		default:
			t.Errorf("unexpected matcher %q", mc.Matcher)
		}
	}
}

func TestComputeWIPAssignees(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		items      []model.WIPItem
		limit      int
		wantLogins []string
		wantCounts []int
	}{
		{
			name:  "empty input",
			items: nil,
			limit: 10,
		},
		{
			name: "single assignee",
			items: []model.WIPItem{
				{Assignees: []string{"alice"}, Status: "In Progress", EffortValue: 1},
			},
			limit:      10,
			wantLogins: []string{"alice"},
			wantCounts: []int{1},
		},
		{
			name: "multiple assignees sorted by count desc",
			items: []model.WIPItem{
				{Assignees: []string{"alice"}, Status: "In Progress", EffortValue: 1},
				{Assignees: []string{"alice"}, Status: "In Progress", EffortValue: 1},
				{Assignees: []string{"bob"}, Status: "In Review", EffortValue: 1},
			},
			limit:      10,
			wantLogins: []string{"alice", "bob"},
			wantCounts: []int{2, 1},
		},
		{
			name: "ties broken by login ascending",
			items: []model.WIPItem{
				{Assignees: []string{"charlie"}, EffortValue: 1},
				{Assignees: []string{"alice"}, EffortValue: 1},
			},
			limit:      10,
			wantLogins: []string{"alice", "charlie"},
			wantCounts: []int{1, 1},
		},
		{
			name: "no assignees counted as unassigned",
			items: []model.WIPItem{
				{Assignees: nil, Status: "In Progress", EffortValue: 1},
				{Assignees: []string{}, Status: "In Progress", EffortValue: 1},
			},
			limit:      10,
			wantLogins: []string{"unassigned"},
			wantCounts: []int{2},
		},
		{
			name: "multi-assignee item counted for each",
			items: []model.WIPItem{
				{Assignees: []string{"alice", "bob"}, EffortValue: 3},
			},
			limit:      10,
			wantLogins: []string{"alice", "bob"},
			wantCounts: []int{1, 1},
		},
		{
			name: "limit respected",
			items: []model.WIPItem{
				{Assignees: []string{"alice"}, EffortValue: 1},
				{Assignees: []string{"alice"}, EffortValue: 1},
				{Assignees: []string{"bob"}, EffortValue: 1},
				{Assignees: []string{"charlie"}, EffortValue: 1},
			},
			limit:      2,
			wantLogins: []string{"alice", "bob"},
			wantCounts: []int{2, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ComputeWIPAssignees(tt.items, tt.limit)

			if len(result) != len(tt.wantLogins) {
				t.Fatalf("got %d assignees, want %d", len(result), len(tt.wantLogins))
			}
			for i, a := range result {
				if a.Login != tt.wantLogins[i] {
					t.Errorf("assignee[%d].Login = %q, want %q", i, a.Login, tt.wantLogins[i])
				}
				if a.ItemCount != tt.wantCounts[i] {
					t.Errorf("assignee[%d].ItemCount = %d, want %d", i, a.ItemCount, tt.wantCounts[i])
				}
			}
		})
	}
}

func TestComputeWIPAssignees_EffortAggregation(t *testing.T) {
	t.Parallel()

	items := []model.WIPItem{
		{Assignees: []string{"alice"}, EffortValue: 3, Status: "In Progress"},
		{Assignees: []string{"alice"}, EffortValue: 5, Status: "In Review"},
	}

	result := ComputeWIPAssignees(items, 10)
	if len(result) != 1 {
		t.Fatalf("got %d, want 1", len(result))
	}
	if result[0].TotalEffort != 8 {
		t.Errorf("TotalEffort = %f, want 8", result[0].TotalEffort)
	}
	if result[0].ByStage["In Progress"] != 1 {
		t.Errorf("ByStage[In Progress] = %d, want 1", result[0].ByStage["In Progress"])
	}
	if result[0].ByStage["In Review"] != 1 {
		t.Errorf("ByStage[In Review] = %d, want 1", result[0].ByStage["In Review"])
	}
}

func TestComputeWIPStaleness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		items      []model.WIPItem
		wantActive int
		wantAging  int
		wantStale  int
	}{
		{
			name:  "empty input",
			items: nil,
		},
		{
			name: "all active",
			items: []model.WIPItem{
				{Staleness: model.StalenessActive},
				{Staleness: model.StalenessActive},
			},
			wantActive: 2,
		},
		{
			name: "mixed staleness",
			items: []model.WIPItem{
				{Staleness: model.StalenessActive},
				{Staleness: model.StalenessAging},
				{Staleness: model.StalenessStale},
				{Staleness: model.StalenessStale},
			},
			wantActive: 1,
			wantAging:  1,
			wantStale:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ComputeWIPStaleness(tt.items)

			if result.Active != tt.wantActive {
				t.Errorf("Active = %d, want %d", result.Active, tt.wantActive)
			}
			if result.Aging != tt.wantAging {
				t.Errorf("Aging = %d, want %d", result.Aging, tt.wantAging)
			}
			if result.Stale != tt.wantStale {
				t.Errorf("Stale = %d, want %d", result.Stale, tt.wantStale)
			}
		})
	}
}

func TestGenerateWIPInsights(t *testing.T) {
	t.Parallel()

	t.Run("empty items returns nil", func(t *testing.T) {
		t.Parallel()
		result := GenerateWIPInsights(model.WIPResult{})
		if result != nil {
			t.Errorf("expected nil, got %d insights", len(result))
		}
	})

	t.Run("capacity insight with people", func(t *testing.T) {
		t.Parallel()
		result := GenerateWIPInsights(model.WIPResult{
			Items: []model.WIPItem{{}, {}, {}},
			Assignees: []model.WIPAssignee{
				{Login: "alice", ItemCount: 2},
				{Login: "bob", ItemCount: 1},
			},
		})
		found := findInsight(result, "wip_capacity")
		if found == nil {
			t.Fatal("expected wip_capacity insight")
		}
		if found.Message != "3 items in progress across 2 people." {
			t.Errorf("message = %q", found.Message)
		}
	})

	t.Run("capacity insight no assignees", func(t *testing.T) {
		t.Parallel()
		result := GenerateWIPInsights(model.WIPResult{
			Items: []model.WIPItem{{}},
			Assignees: []model.WIPAssignee{
				{Login: "unassigned", ItemCount: 1},
			},
		})
		found := findInsight(result, "wip_capacity")
		if found == nil {
			t.Fatal("expected wip_capacity insight")
		}
		if found.Message != "1 items in progress (no assignees)." {
			t.Errorf("message = %q", found.Message)
		}
	})

	t.Run("assignee load insight only with >1 person", func(t *testing.T) {
		t.Parallel()

		// Single person — no assignee load insight.
		result := GenerateWIPInsights(model.WIPResult{
			Items: []model.WIPItem{{}},
			Assignees: []model.WIPAssignee{
				{Login: "alice", ItemCount: 1},
			},
		})
		if findInsight(result, "wip_assignee_load") != nil {
			t.Error("should not have assignee load insight with 1 person")
		}

		// Two people.
		result = GenerateWIPInsights(model.WIPResult{
			Items: []model.WIPItem{{}, {}, {}},
			Assignees: []model.WIPAssignee{
				{Login: "alice", ItemCount: 2},
				{Login: "bob", ItemCount: 1},
			},
		})
		found := findInsight(result, "wip_assignee_load")
		if found == nil {
			t.Fatal("expected wip_assignee_load insight")
		}
	})

	t.Run("staleness insight", func(t *testing.T) {
		t.Parallel()
		result := GenerateWIPInsights(model.WIPResult{
			Items:     make([]model.WIPItem, 10),
			Staleness: model.WIPStaleness{Active: 7, Aging: 1, Stale: 2},
			Assignees: []model.WIPAssignee{{Login: "alice", ItemCount: 10}},
		})
		found := findInsight(result, "wip_staleness")
		if found == nil {
			t.Fatal("expected wip_staleness insight")
		}
	})

	t.Run("no staleness insight when stale=0", func(t *testing.T) {
		t.Parallel()
		result := GenerateWIPInsights(model.WIPResult{
			Items:     make([]model.WIPItem, 5),
			Staleness: model.WIPStaleness{Active: 5},
			Assignees: []model.WIPAssignee{{Login: "alice", ItemCount: 5}},
		})
		if findInsight(result, "wip_staleness") != nil {
			t.Error("should not have staleness insight when stale=0")
		}
	})

	t.Run("stage health insight when review > 50%", func(t *testing.T) {
		t.Parallel()
		result := GenerateWIPInsights(model.WIPResult{
			Items: make([]model.WIPItem, 10),
			StageCounts: []model.WIPStageCount{
				{Stage: "In Progress", Count: 3},
				{Stage: "In Review", Count: 7},
			},
			Assignees: []model.WIPAssignee{{Login: "alice", ItemCount: 10}},
		})
		found := findInsight(result, "wip_stage_health")
		if found == nil {
			t.Fatal("expected wip_stage_health insight")
		}
	})

	t.Run("team limit exceeded insight uses human effort", func(t *testing.T) {
		t.Parallel()
		teamLimit := 5.0
		result := GenerateWIPInsights(model.WIPResult{
			Items:       make([]model.WIPItem, 10),
			TotalEffort: 10,
			HumanEffort: 8,
			TeamLimit:   &teamLimit,
			Assignees:   []model.WIPAssignee{{Login: "alice", ItemCount: 10}},
		})
		found := findInsight(result, "wip_team_limit_exceeded")
		if found == nil {
			t.Fatal("expected wip_team_limit_exceeded insight")
		}
		if !strings.Contains(found.Message, "Human WIP") {
			t.Errorf("expected message to reference Human WIP, got %q", found.Message)
		}
	})

	t.Run("team limit not exceeded when human effort within limit", func(t *testing.T) {
		t.Parallel()
		teamLimit := 10.0
		result := GenerateWIPInsights(model.WIPResult{
			Items:       make([]model.WIPItem, 15),
			TotalEffort: 15,
			HumanEffort: 8,
			BotEffort:   7,
			TeamLimit:   &teamLimit,
			Assignees:   []model.WIPAssignee{{Login: "alice", ItemCount: 8}},
		})
		found := findInsight(result, "wip_team_limit_exceeded")
		if found != nil {
			t.Error("should not have team limit insight when human effort is within limit")
		}
	})

	t.Run("person limit exceeded insight", func(t *testing.T) {
		t.Parallel()
		personLimit := 3.0
		result := GenerateWIPInsights(model.WIPResult{
			Items:       make([]model.WIPItem, 5),
			PersonLimit: &personLimit,
			Assignees: []model.WIPAssignee{
				{Login: "alice", ItemCount: 4, TotalEffort: 4, OverLimit: true},
				{Login: "bob", ItemCount: 1, TotalEffort: 1},
			},
		})
		found := findInsight(result, "wip_person_limit_exceeded")
		if found == nil {
			t.Fatal("expected wip_person_limit_exceeded insight")
		}
	})
}

func TestGenerateWIPInsights_BotActivity(t *testing.T) {
	t.Parallel()

	t.Run("bot activity insight when bots present", func(t *testing.T) {
		t.Parallel()
		result := GenerateWIPInsights(model.WIPResult{
			Items:          make([]model.WIPItem, 10),
			BotItemCount:   3,
			HumanItemCount: 7,
			Assignees:      []model.WIPAssignee{{Login: "alice", ItemCount: 7}},
		})
		found := findInsight(result, "wip_bot_activity")
		if found == nil {
			t.Fatal("expected wip_bot_activity insight")
		}
		if !strings.Contains(found.Message, "3 items assigned to bots") {
			t.Errorf("unexpected message: %q", found.Message)
		}
		if !strings.Contains(found.Message, "30%") {
			t.Errorf("expected 30%% in message, got: %q", found.Message)
		}
	})

	t.Run("no bot activity insight when no bots", func(t *testing.T) {
		t.Parallel()
		result := GenerateWIPInsights(model.WIPResult{
			Items:          make([]model.WIPItem, 5),
			HumanItemCount: 5,
			Assignees:      []model.WIPAssignee{{Login: "alice", ItemCount: 5}},
		})
		if findInsight(result, "wip_bot_activity") != nil {
			t.Error("should not have bot activity insight when no bots")
		}
	})
}

func TestPartitionAssignees(t *testing.T) {
	t.Parallel()

	all := []model.WIPAssignee{
		{Login: "alice", IsBot: false, ItemCount: 5},
		{Login: "dependabot[bot]", IsBot: true, ItemCount: 3},
		{Login: "bob", IsBot: false, ItemCount: 2},
		{Login: "renovate", IsBot: true, ItemCount: 1},
	}

	human, bot := PartitionAssignees(all, 10)
	if len(human) != 2 {
		t.Errorf("human count = %d, want 2", len(human))
	}
	if len(bot) != 2 {
		t.Errorf("bot count = %d, want 2", len(bot))
	}
	if human[0].Login != "alice" {
		t.Errorf("human[0] = %q, want alice", human[0].Login)
	}
	if bot[0].Login != "dependabot[bot]" {
		t.Errorf("bot[0] = %q, want dependabot[bot]", bot[0].Login)
	}
}

func TestPartitionAssignees_Limit(t *testing.T) {
	t.Parallel()

	all := []model.WIPAssignee{
		{Login: "alice", IsBot: false, ItemCount: 5},
		{Login: "bob", IsBot: false, ItemCount: 4},
		{Login: "charlie", IsBot: false, ItemCount: 3},
	}

	human, bot := PartitionAssignees(all, 2)
	if len(human) != 2 {
		t.Errorf("human count = %d, want 2", len(human))
	}
	if len(bot) != 0 {
		t.Errorf("bot count = %d, want 0", len(bot))
	}
}

func TestClassifyItemsByBot(t *testing.T) {
	t.Parallel()

	items := []model.WIPItem{
		{Number: 1, Assignees: []string{"alice"}},
		{Number: 2, Assignees: []string{"dependabot[bot]"}},
		{Number: 3, Assignees: []string{"bob", "alice"}},
		{Number: 4, Assignees: nil},                                    // unassigned -> human
		{Number: 5, Assignees: []string{"dependabot[bot]", "renovate"}}, // all bots
		{Number: 6, Assignees: []string{"alice", "dependabot[bot]"}},    // mixed -> human
	}

	human, bot := ClassifyItemsByBot(items, nil)
	if len(human) != 4 {
		t.Errorf("human count = %d, want 4", len(human))
	}
	if len(bot) != 2 {
		t.Errorf("bot count = %d, want 2", len(bot))
	}

	// Bot items should be #2 and #5.
	botNumbers := map[int]bool{}
	for _, b := range bot {
		botNumbers[b.Number] = true
	}
	if !botNumbers[2] {
		t.Error("expected item #2 to be bot")
	}
	if !botNumbers[5] {
		t.Error("expected item #5 to be bot")
	}
}

func TestClassifyItemsByBot_ExcludeUsers(t *testing.T) {
	t.Parallel()

	items := []model.WIPItem{
		{Number: 1, Assignees: []string{"my-ci-bot"}},
	}

	human, bot := ClassifyItemsByBot(items, []string{"my-ci-bot"})
	if len(human) != 0 {
		t.Errorf("human count = %d, want 0", len(human))
	}
	if len(bot) != 1 {
		t.Errorf("bot count = %d, want 1", len(bot))
	}
}

func findInsight(insights []model.Insight, insightType string) *model.Insight {
	for _, i := range insights {
		if i.Type == insightType {
			return &i
		}
	}
	return nil
}
