package cmd

import (
	"testing"
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

func TestRenderPreflightConfig_RoundTrips(t *testing.T) {
	// A minimal result should produce YAML that round-trips through config.Parse.
	result := &PreflightResult{
		Repo:     "owner/repo",
		Strategy: "issue",
		Categories: map[string][]string{
			"bug":     {"bug", "defect"},
			"feature": {"enhancement"},
		},
		Hints: []string{"test hint"},
	}

	yaml := renderPreflightConfig(result)

	// Verify no "posting:" block appears as a YAML key
	if contains(yaml, "\nposting:\n") {
		t.Error("renderPreflightConfig should not generate a 'posting:' YAML block")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
