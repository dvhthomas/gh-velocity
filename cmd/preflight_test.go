package cmd

import (
	"strings"
	"testing"

	"github.com/bitsbyme/gh-velocity/internal/config"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestMatchesWord(t *testing.T) {
	tests := []struct {
		label   string
		pattern string
		want    bool
	}{
		// Exact matches
		{"bug", "bug", true},
		{"feature", "feature", true},
		{"chore", "chore", true},

		// Prefix matches (word boundary at end)
		{"bug-report", "bug", true},
		{"feature-request", "feature", true},
		{"chores-daily", "chore", false}, // "chores" is not "chore" at word boundary

		// Suffix matches (word boundary at start)
		{"type:bug", "bug", true},
		{"kind/feature", "feature", true},

		// Multi-boundary
		{"kind:bug-fix", "bug", true},
		{"some_chore_task", "chore", true},

		// False positives eliminated
		{"debugging", "bug", false},
		{"defeat", "feat", false},
		{"proactive", "active", false},
		{"translator", "later", false},
		{"networking", "working", false},
		{"interactive", "active", false},
		{"reactive", "active", false},

		// Status labels
		{"in-progress", "in-progress", true},
		{"in progress", "in progress", true},
		{"wip", "wip", true},
		{"backlog", "backlog", true},

		// Case handling (matchesWord works on lowered input)
		{"bug", "bug", true},
		{"enhancement", "enhancement", true},

		// Docs
		{"documentation", "documentation", true},
		{"docs", "docs", true},

		// Tech debt
		{"tech-debt", "tech-debt", true},
	}

	for _, tt := range tests {
		t.Run(tt.label+"_"+tt.pattern, func(t *testing.T) {
			got := matchesWord(tt.label, tt.pattern)
			if got != tt.want {
				t.Errorf("matchesWord(%q, %q) = %v, want %v", tt.label, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestClassifyLabels(t *testing.T) {
	labels := []string{
		"bug", "Bug-report", "enhancement", "debugging",
		"chore", "documentation", "in-progress", "backlog",
		"defeat", "proactive",
	}

	result := &PreflightResult{}
	classifyLabels(result, labels)

	// Bug category should have "bug" and "Bug-report" but NOT "debugging"
	bugLabels := result.Categories["bug"]
	if len(bugLabels) != 2 {
		t.Errorf("expected 2 bug labels, got %d: %v", len(bugLabels), bugLabels)
	}

	// Feature category should have "enhancement" but NOT "defeat"
	featureLabels := result.Categories["feature"]
	if len(featureLabels) != 1 || featureLabels[0] != "enhancement" {
		t.Errorf("expected [enhancement], got %v", featureLabels)
	}

	// Chore should have "chore"
	choreLabels := result.Categories["chore"]
	if len(choreLabels) != 1 || choreLabels[0] != "chore" {
		t.Errorf("expected [chore], got %v", choreLabels)
	}

	// Docs should have "documentation"
	docsLabels := result.Categories["docs"]
	if len(docsLabels) != 1 || docsLabels[0] != "documentation" {
		t.Errorf("expected [documentation], got %v", docsLabels)
	}

	// Active should have "in-progress" but NOT "proactive"
	if len(result.ActiveLabels) != 1 || result.ActiveLabels[0] != "in-progress" {
		t.Errorf("expected active [in-progress], got %v", result.ActiveLabels)
	}

	// Backlog should have "backlog"
	if len(result.BacklogLabels) != 1 || result.BacklogLabels[0] != "backlog" {
		t.Errorf("expected backlog [backlog], got %v", result.BacklogLabels)
	}
}

func TestClassifyLabels_IgnorePrefixes(t *testing.T) {
	labels := []string{
		"bug",
		"event/terraform-docs-day",      // should be ignored (event/ prefix)
		"do-not-merge/work-in-progress", // should be ignored (do-not-merge prefix)
		"needs-investigation",           // should be ignored (needs- prefix)
		"kind/bug",                      // should match bug
		"documentation",                 // should match docs
		"in-progress",                   // should match active
	}

	result := &PreflightResult{}
	classifyLabels(result, labels)

	// Docs should NOT contain "event/terraform-docs-day"
	for _, l := range result.Categories["docs"] {
		if l == "event/terraform-docs-day" {
			t.Error("event/terraform-docs-day should be excluded by ignore prefix")
		}
	}
	if len(result.Categories["docs"]) != 1 || result.Categories["docs"][0] != "documentation" {
		t.Errorf("expected docs [documentation], got %v", result.Categories["docs"])
	}

	// Active should NOT contain "do-not-merge/work-in-progress"
	for _, l := range result.ActiveLabels {
		if l == "do-not-merge/work-in-progress" {
			t.Error("do-not-merge/work-in-progress should be excluded by ignore prefix")
		}
	}
	if len(result.ActiveLabels) != 1 || result.ActiveLabels[0] != "in-progress" {
		t.Errorf("expected active [in-progress], got %v", result.ActiveLabels)
	}

	// Bug should have both "bug" and "kind/bug"
	if len(result.Categories["bug"]) != 2 {
		t.Errorf("expected 2 bug labels, got %v", result.Categories["bug"])
	}
}

func TestRenderPreflightConfig_RoundTrips(t *testing.T) {
	tests := []struct {
		name   string
		result *PreflightResult
	}{
		{
			name: "minimal",
			result: &PreflightResult{
				Repo:     "owner/repo",
				Strategy: "issue",
				Categories: map[string][]string{
					"bug":     {"bug", "defect"},
					"feature": {"enhancement"},
				},
				Hints: []string{"test hint"},
			},
		},
		{
			name: "labels with colons",
			result: &PreflightResult{
				Repo:     "facebook/react",
				Strategy: "pr",
				Categories: map[string][]string{
					"bug":     {"Type: Bug", "Type: Regression"},
					"feature": {"Type: Enhancement", "Type: Feature Request"},
				},
			},
		},
		{
			name: "backlog labels with spaces",
			result: &PreflightResult{
				Repo:     "facebook/react",
				Strategy: "pr",
				Categories: map[string][]string{
					"bug": {"bug"},
				},
				BacklogLabels: []string{"Resolution: Backlog"},
			},
		},
		{
			name: "backlog labels simple",
			result: &PreflightResult{
				Repo:     "owner/repo",
				Strategy: "issue",
				Categories: map[string][]string{
					"bug": {"bug"},
				},
				BacklogLabels: []string{"backlog", "icebox"},
			},
		},
		{
			name: "project board with status options",
			result: &PreflightResult{
				Repo:          "owner/repo",
				Strategy:      "project-board",
				HasProject:    true,
				ProjectURL:    "https://github.com/users/test/projects/1",
				StatusOptions: []string{"Backlog", "In Progress", "In Review", "Done"},
				Categories: map[string][]string{
					"bug":     {"bug"},
					"feature": {"enhancement"},
				},
			},
		},
		{
			name: "no categories detected",
			result: &PreflightResult{
				Repo:     "owner/repo",
				Strategy: "issue",
			},
		},
		{
			name: "labels with special characters",
			result: &PreflightResult{
				Repo:     "owner/repo",
				Strategy: "pr",
				Categories: map[string][]string{
					"bug":     {"kind/bug", "priority:critical-bug"},
					"feature": {"kind/feature", "kind/api-change"},
				},
				BacklogLabels: []string{"priority/backlog", "lifecycle/frozen"},
			},
		},
		{
			name: "hints with newlines",
			result: &PreflightResult{
				Repo:     "owner/repo",
				Strategy: "issue",
				Hints:    []string{"line one\nline two", "normal hint"},
			},
		},
		{
			name: "with match evidence",
			result: &PreflightResult{
				Repo:     "owner/repo",
				Strategy: "pr",
				Categories: map[string][]string{
					"bug":     {"bug"},
					"feature": {"enhancement"},
				},
				MatchEvidence: []CategoryEvidence{
					{
						Category: "bug",
						Matchers: []MatcherEvidence{
							{Matcher: "label:bug", Count: 12, Example: "#42 Fix crash on startup"},
						},
					},
					{
						Category: "feature",
						Matchers: []MatcherEvidence{
							{Matcher: "label:enhancement", Count: 0}, // no matches
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlStr := renderPreflightConfig(tt.result)

			// Suppress warnings during parse (defaults may overlap with categories).
			origWarn := config.WarnFunc
			config.WarnFunc = func(string, ...any) {}
			defer func() { config.WarnFunc = origWarn }()

			cfg, err := config.Parse([]byte(yamlStr))
			if err != nil {
				t.Fatalf("generated YAML does not parse:\n%s\nerror: %v", yamlStr, err)
			}

			// Basic sanity: config should have a valid strategy
			if cfg.CycleTime.Strategy == "" {
				t.Error("expected non-empty cycle_time.strategy")
			}
		})
	}
}

func TestVerifyConfig_ValidConfig(t *testing.T) {
	result := &PreflightResult{
		Repo:     "owner/repo",
		Strategy: "issue",
		Categories: map[string][]string{
			"bug":     {"bug"},
			"feature": {"enhancement"},
		},
	}
	repoLabels := []string{"bug", "enhancement", "docs"}

	vr := verifyConfig(result, repoLabels)

	if !vr.Valid {
		t.Errorf("expected valid, got invalid: warnings=%v", vr.Warnings)
	}
	if !vr.ConfigParses {
		t.Error("expected config to parse")
	}
	if !vr.MatchersValid {
		t.Error("expected matchers to be valid")
	}
	if vr.CategoryCount != 2 {
		t.Errorf("expected 2 categories, got %d", vr.CategoryCount)
	}
	if len(vr.MissingLabels) != 0 {
		t.Errorf("expected no missing labels, got %v", vr.MissingLabels)
	}
}

func TestVerifyConfig_MissingLabel(t *testing.T) {
	result := &PreflightResult{
		Repo:     "owner/repo",
		Strategy: "issue",
		Categories: map[string][]string{
			"bug": {"bug", "regression"},
		},
	}
	// "regression" is NOT in the repo's labels
	repoLabels := []string{"bug", "enhancement"}

	vr := verifyConfig(result, repoLabels)

	if vr.Valid {
		t.Error("expected invalid due to missing label")
	}
	if len(vr.MissingLabels) != 1 || vr.MissingLabels[0] != "regression" {
		t.Errorf("expected missing [regression], got %v", vr.MissingLabels)
	}
}

func TestVerifyConfig_NoRepoLabels(t *testing.T) {
	result := &PreflightResult{
		Repo:     "owner/repo",
		Strategy: "issue",
	}

	// When no repo labels are available, skip cross-reference.
	vr := verifyConfig(result, nil)

	if !vr.ConfigParses {
		t.Error("expected config to parse")
	}
	// Valid because we can't verify labels without repo data.
	if !vr.Valid {
		t.Errorf("expected valid when no repo labels, got warnings=%v", vr.Warnings)
	}
}

func TestCollectMatchEvidence(t *testing.T) {
	categories := map[string][]string{
		"bug":     {"bug", "crash"},
		"feature": {"enhancement"},
	}
	issues := []model.Issue{
		{Number: 1, Title: "Fix crash on startup", Labels: []string{"bug"}},
		{Number: 2, Title: "Null pointer in parser", Labels: []string{"bug", "crash"}},
		{Number: 3, Title: "Add dark mode", Labels: []string{"enhancement"}},
		{Number: 4, Title: "Clean up docs", Labels: []string{"documentation"}},
	}
	prs := []model.PR{
		{Number: 10, Title: "Fix memory leak", Labels: []string{"bug"}},
		{Number: 11, Title: "New widget", Labels: []string{"enhancement"}},
	}

	evidence := collectMatchEvidence(categories, nil, issues, prs)

	// Find bug and feature categories in results.
	findCat := func(name string) *CategoryEvidence {
		for i := range evidence {
			if evidence[i].Category == name {
				return &evidence[i]
			}
		}
		return nil
	}
	findMatcher := func(ce *CategoryEvidence, matcher string) *MatcherEvidence {
		for i := range ce.Matchers {
			if ce.Matchers[i].Matcher == matcher {
				return &ce.Matchers[i]
			}
		}
		return nil
	}

	bugCat := findCat("bug")
	if bugCat == nil {
		t.Fatal("expected bug category in evidence")
	}

	// label:bug should match issues #1, #2 and PR #10 = 3
	bugLabel := findMatcher(bugCat, "label:bug")
	if bugLabel == nil {
		t.Fatal("expected label:bug matcher")
	}
	if bugLabel.Count != 3 {
		t.Errorf("label:bug count = %d, want 3", bugLabel.Count)
	}
	if bugLabel.Example == "" {
		t.Error("expected an example for label:bug")
	}

	// label:crash should match issue #2 = 1
	crashLabel := findMatcher(bugCat, "label:crash")
	if crashLabel == nil {
		t.Fatal("expected label:crash matcher")
	}
	if crashLabel.Count != 1 {
		t.Errorf("label:crash count = %d, want 1", crashLabel.Count)
	}

	// Title probe "Fix crash" and "Fix memory leak" should be found by title:/^fix/i
	fixTitle := findMatcher(bugCat, `title:/^fix[\(: ]/i`)
	if fixTitle == nil {
		t.Fatal("expected title fix probe")
	}
	if fixTitle.Count != 2 {
		t.Errorf("title fix count = %d, want 2 (Fix crash + Fix memory)", fixTitle.Count)
	}
	if !fixTitle.Suggested {
		t.Error("title probe should be marked as suggested")
	}

	// Feature category: label:enhancement should match #3 and #11 = 2
	featCat := findCat("feature")
	if featCat == nil {
		t.Fatal("expected feature category in evidence")
	}
	enhLabel := findMatcher(featCat, "label:enhancement")
	if enhLabel == nil {
		t.Fatal("expected label:enhancement matcher")
	}
	if enhLabel.Count != 2 {
		t.Errorf("label:enhancement count = %d, want 2", enhLabel.Count)
	}

	// Matchers should be sorted by count descending.
	if bugCat.Matchers[0].Count < bugCat.Matchers[len(bugCat.Matchers)-1].Count {
		t.Error("expected matchers sorted by count descending")
	}
}

func TestCollectMatchEvidence_NoItems(t *testing.T) {
	categories := map[string][]string{"bug": {"bug"}}
	evidence := collectMatchEvidence(categories, nil, nil, nil)
	if evidence != nil {
		t.Errorf("expected nil evidence with no items, got %v", evidence)
	}
}

func TestCollectMatchEvidence_TitleFallback(t *testing.T) {
	// No labels at all, but titles follow conventional commits.
	categories := map[string][]string{}
	issues := []model.Issue{
		{Number: 1, Title: "feat: add dark mode"},
		{Number: 2, Title: "fix: crash on startup"},
		{Number: 3, Title: "docs: update readme"},
		{Number: 4, Title: "chore: update deps"},
	}

	evidence := collectMatchEvidence(categories, nil, issues, nil)

	findCat := func(name string) *CategoryEvidence {
		for i := range evidence {
			if evidence[i].Category == name {
				return &evidence[i]
			}
		}
		return nil
	}

	// Feature should find "feat: add dark mode" via title probe
	featCat := findCat("feature")
	if featCat == nil {
		t.Fatal("expected feature category from title probes")
	}
	hasMatch := false
	for _, me := range featCat.Matchers {
		if me.Count > 0 {
			hasMatch = true
			break
		}
	}
	if !hasMatch {
		t.Error("expected title probes to find feat: prefix")
	}

	// Bug should find "fix: crash on startup"
	bugCat := findCat("bug")
	if bugCat == nil {
		t.Fatal("expected bug category from title probes")
	}
	hasMatch = false
	for _, me := range bugCat.Matchers {
		if me.Count > 0 {
			hasMatch = true
			break
		}
	}
	if !hasMatch {
		t.Error("expected title probes to find fix: prefix")
	}
}

func TestRenderPreflightConfig_AutoDetectedHint(t *testing.T) {
	result := &PreflightResult{
		Repo:             "owner/repo",
		Strategy:         "issue",
		RepoAutoDetected: true,
		Hints:            []string{"existing hint"},
	}

	// Simulate what RunE does: append auto-detection hint.
	result.Hints = append(result.Hints,
		"Repo owner/repo auto-detected from git remote. Use -R owner/repo to target a different repository.")

	yamlStr := renderPreflightConfig(result)

	// The auto-detection hint should appear in the YAML comments.
	if !strings.Contains(yamlStr, "auto-detected from git remote") {
		t.Errorf("expected auto-detection hint in YAML output, got:\n%s", yamlStr)
	}
}

func TestRenderPreflightConfig_NoAutoDetectedHint(t *testing.T) {
	result := &PreflightResult{
		Repo:     "owner/repo",
		Strategy: "issue",
		Hints:    []string{"some other hint"},
	}

	yamlStr := renderPreflightConfig(result)

	if strings.Contains(yamlStr, "auto-detected from git remote") {
		t.Error("should not contain auto-detection hint when RepoAutoDetected is false")
	}
}

func TestCollectMatchEvidence_WithDiscoveredTypes(t *testing.T) {
	categories := map[string][]string{"bug": {"bug"}}
	discoveredTypes := []string{"Bug", "Feature", "Task"}
	issues := []model.Issue{
		{Number: 1, Title: "Fix crash", Labels: []string{"bug"}, IssueType: "Bug"},
		{Number: 2, Title: "Add feature", Labels: []string{}, IssueType: "Feature"},
		{Number: 3, Title: "Clean up", Labels: []string{}, IssueType: "Task"},
	}

	evidence := collectMatchEvidence(categories, discoveredTypes, issues, nil)

	findCat := func(name string) *CategoryEvidence {
		for i := range evidence {
			if evidence[i].Category == name {
				return &evidence[i]
			}
		}
		return nil
	}
	findMatcher := func(ce *CategoryEvidence, matcher string) *MatcherEvidence {
		for i := range ce.Matchers {
			if ce.Matchers[i].Matcher == matcher {
				return &ce.Matchers[i]
			}
		}
		return nil
	}

	// Bug category should have both label:bug and type:Bug matchers.
	bugCat := findCat("bug")
	if bugCat == nil {
		t.Fatal("expected bug category")
	}
	typeBug := findMatcher(bugCat, "type:Bug")
	if typeBug == nil {
		t.Fatal("expected type:Bug matcher")
	}
	if typeBug.Count != 1 {
		t.Errorf("type:Bug count = %d, want 1", typeBug.Count)
	}
	if typeBug.Suggested {
		t.Error("type: matchers should not be marked as suggested")
	}

	// Feature category should have type:Feature.
	featCat := findCat("feature")
	if featCat == nil {
		t.Fatal("expected feature category")
	}
	typeFeat := findMatcher(featCat, "type:Feature")
	if typeFeat == nil {
		t.Fatal("expected type:Feature matcher")
	}
	if typeFeat.Count != 1 {
		t.Errorf("type:Feature count = %d, want 1", typeFeat.Count)
	}

	// Chore category should have type:Task.
	choreCat := findCat("chore")
	if choreCat == nil {
		t.Fatal("expected chore category")
	}
	typeTask := findMatcher(choreCat, "type:Task")
	if typeTask == nil {
		t.Fatal("expected type:Task matcher")
	}
	if typeTask.Count != 1 {
		t.Errorf("type:Task count = %d, want 1", typeTask.Count)
	}
}

func TestCollectMatchEvidence_UnmappedTypesIgnored(t *testing.T) {
	// Types that don't map to any category should not generate probe jobs.
	categories := map[string][]string{}
	discoveredTypes := []string{"Spike", "Epic"}
	issues := []model.Issue{
		{Number: 1, Title: "Research spike", IssueType: "Spike"},
	}

	evidence := collectMatchEvidence(categories, discoveredTypes, issues, nil)

	// No type: matchers should exist since Spike and Epic don't map to any category.
	for _, ce := range evidence {
		for _, me := range ce.Matchers {
			if strings.HasPrefix(me.Matcher, "type:") {
				t.Errorf("unexpected type matcher %q for unmapped type", me.Matcher)
			}
		}
	}
}

func TestRenderPreflightConfig_BaselineCategories(t *testing.T) {
	// When no evidence or labels detected, baseline should include bug, feature, chore.
	result := &PreflightResult{
		Repo:     "owner/repo",
		Strategy: "issue",
	}

	yamlStr := renderPreflightConfig(result)

	if !strings.Contains(yamlStr, "- name: bug") {
		t.Error("baseline should include bug category")
	}
	if !strings.Contains(yamlStr, "- name: feature") {
		t.Error("baseline should include feature category")
	}
	if !strings.Contains(yamlStr, "- name: chore") {
		t.Error("baseline should include chore category")
	}
}

func TestTypePatterns(t *testing.T) {
	// Verify typePatterns maps expected types to categories.
	if types, ok := typePatterns["bug"]; !ok || len(types) == 0 {
		t.Error("expected bug type patterns")
	}
	if types, ok := typePatterns["feature"]; !ok || len(types) == 0 {
		t.Error("expected feature type patterns")
	}
	if types, ok := typePatterns["chore"]; !ok || len(types) == 0 {
		t.Error("expected chore type patterns")
	}
	if _, ok := typePatterns["docs"]; ok {
		t.Error("docs should not be in typePatterns")
	}
}

func TestVerifyConfig_ProjectBoardWithoutID(t *testing.T) {
	result := &PreflightResult{
		Repo:       "owner/repo",
		Strategy:   "project-board",
		HasProject: true,
		// ProjectURL intentionally empty
	}

	vr := verifyConfig(result, nil)

	// The generated YAML won't include project.url, but strategy is project-board.
	// config.Parse should reject this.
	if vr.ConfigParses && len(vr.Warnings) == 0 {
		t.Error("expected warning about project-board requiring project.url")
	}
}
