package classify

import (
	"testing"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestLabelMatcher(t *testing.T) {
	tests := []struct {
		name   string
		label  string
		input  Input
		expect bool
	}{
		{"exact match", "bug", Input{Labels: []string{"bug"}}, true},
		{"case insensitive", "Bug", Input{Labels: []string{"bug"}}, true},
		{"no match", "bug", Input{Labels: []string{"enhancement"}}, false},
		{"empty labels", "bug", Input{Labels: nil}, false},
		{"multiple labels", "bug", Input{Labels: []string{"priority:high", "bug"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := LabelMatcher{Label: tt.label}
			if got := m.Matches(tt.input); got != tt.expect {
				t.Errorf("LabelMatcher{%q}.Matches() = %v, want %v", tt.label, got, tt.expect)
			}
		})
	}
}

func TestTypeMatcher(t *testing.T) {
	tests := []struct {
		name   string
		typ    string
		input  Input
		expect bool
	}{
		{"exact match", "Bug", Input{IssueType: "Bug"}, true},
		{"case sensitive", "bug", Input{IssueType: "Bug"}, false},
		{"no match", "Bug", Input{IssueType: "Feature"}, false},
		{"empty type", "Bug", Input{IssueType: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := TypeMatcher{Type: tt.typ}
			if got := m.Matches(tt.input); got != tt.expect {
				t.Errorf("TypeMatcher{%q}.Matches() = %v, want %v", tt.typ, got, tt.expect)
			}
		})
	}
}

func TestTitleMatcher(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		input   Input
		expect  bool
	}{
		{"simple match", "/fix/", Input{Title: "fix: login timeout"}, true},
		{"no match", "/fix/", Input{Title: "feat: add dashboard"}, false},
		{"case insensitive", "/fix/i", Input{Title: "FIX: login timeout"}, true},
		{"regex anchored", "/^regression:/i", Input{Title: "regression: flaky test"}, true},
		{"regex anchored no match", "/^regression:/i", Input{Title: "fix regression in parser"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := parseTitleMatcher(tt.pattern)
			if err != nil {
				t.Fatalf("parseTitleMatcher(%q) error: %v", tt.pattern, err)
			}
			if got := m.Matches(tt.input); got != tt.expect {
				t.Errorf("TitleMatcher{%q}.Matches() = %v, want %v", tt.pattern, got, tt.expect)
			}
		})
	}
}

func TestParseMatcher(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"label matcher", "label:bug", false},
		{"type matcher", "type:Bug", false},
		{"title matcher", "title:/fix/i", false},
		{"unknown prefix", "foo:bar", true},
		{"no colon", "labelonly", true},
		{"empty value", "label:", true},
		{"title no slash", "title:fix", true},
		{"title no closing slash", "title:/fix", true},
		{"title invalid regex", "title:/[invalid/", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseMatcher(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMatcher(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestClassifier_Classify(t *testing.T) {
	categories := []model.CategoryConfig{
		{Name: "bug", Matchers: []string{"label:bug", "label:defect", "type:Bug"}},
		{Name: "feature", Matchers: []string{"label:enhancement", "type:Feature"}},
		{Name: "regression", Matchers: []string{"label:regression", "title:/^regression:/i"}},
	}

	c, err := NewClassifier(categories)
	if err != nil {
		t.Fatalf("NewClassifier error: %v", err)
	}

	tests := []struct {
		name        string
		input       Input
		expect      string
		wantWarning bool
	}{
		{"bug by label", Input{Labels: []string{"bug"}}, "bug", false},
		{"bug by defect label", Input{Labels: []string{"defect"}}, "bug", false},
		{"bug by type", Input{IssueType: "Bug"}, "bug", false},
		{"feature by label", Input{Labels: []string{"enhancement"}}, "feature", false},
		{"feature by type", Input{IssueType: "Feature"}, "feature", false},
		{"regression by label", Input{Labels: []string{"regression"}}, "regression", false},
		{"regression by title", Input{Title: "regression: flaky test"}, "regression", false},
		{"other when no match", Input{Labels: []string{"documentation"}}, "other", false},
		{"empty input", Input{}, "other", false},
		{"first match wins with warning", Input{Labels: []string{"bug", "regression"}}, "bug", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(tt.input)
			if result.Category != tt.expect {
				t.Errorf("Classify().Category = %q, want %q", result.Category, tt.expect)
			}
			if tt.wantWarning && len(result.Warnings) == 0 {
				t.Error("expected warning for multi-category match, got none")
			}
			if !tt.wantWarning && len(result.Warnings) > 0 {
				t.Errorf("unexpected warnings: %v", result.Warnings)
			}
		})
	}
}

func TestClassifier_CategoryNames(t *testing.T) {
	categories := []model.CategoryConfig{
		{Name: "bug", Matchers: []string{"label:bug"}},
		{Name: "feature", Matchers: []string{"label:enhancement"}},
	}
	c, err := NewClassifier(categories)
	if err != nil {
		t.Fatalf("NewClassifier error: %v", err)
	}
	names := c.CategoryNames()
	if len(names) != 2 || names[0] != "bug" || names[1] != "feature" {
		t.Errorf("CategoryNames() = %v, want [bug feature]", names)
	}
}

func TestFromLegacyLabels(t *testing.T) {
	cats := FromLegacyLabels([]string{"bug", "defect"}, []string{"enhancement"})
	if len(cats) != 2 {
		t.Fatalf("got %d categories, want 2", len(cats))
	}
	if cats[0].Name != "bug" || len(cats[0].Matchers) != 2 {
		t.Errorf("bug category: %+v", cats[0])
	}
	if cats[0].Matchers[0] != "label:bug" || cats[0].Matchers[1] != "label:defect" {
		t.Errorf("bug matchers: %v", cats[0].Matchers)
	}
	if cats[1].Name != "feature" || cats[1].Matchers[0] != "label:enhancement" {
		t.Errorf("feature category: %+v", cats[1])
	}
}

func TestFromLegacyLabels_Empty(t *testing.T) {
	cats := FromLegacyLabels(nil, nil)
	if len(cats) != 0 {
		t.Errorf("got %d categories from empty labels, want 0", len(cats))
	}
}

func TestNewClassifier_InvalidMatcher(t *testing.T) {
	categories := []model.CategoryConfig{
		{Name: "bad", Matchers: []string{"invalid"}},
	}
	_, err := NewClassifier(categories)
	if err == nil {
		t.Error("expected error for invalid matcher, got nil")
	}
}
