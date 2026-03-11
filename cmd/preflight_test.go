package cmd

import (
	"testing"

	"github.com/bitsbyme/gh-velocity/internal/config"
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
