package classify

import (
	"testing"

	"github.com/dvhthomas/gh-velocity/internal/model"
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

func TestFieldMatcher(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		value  string
		input  Input
		expect bool
	}{
		{"exact match", "Size", "M", Input{Fields: map[string]string{"Size": "M"}}, true},
		{"case insensitive field", "size", "M", Input{Fields: map[string]string{"Size": "M"}}, true},
		{"case insensitive value", "Size", "m", Input{Fields: map[string]string{"Size": "M"}}, true},
		{"both case insensitive", "SIZE", "xl", Input{Fields: map[string]string{"Size": "XL"}}, true},
		{"no match wrong value", "Size", "L", Input{Fields: map[string]string{"Size": "M"}}, false},
		{"no match wrong field", "Priority", "M", Input{Fields: map[string]string{"Size": "M"}}, false},
		{"nil fields", "Size", "M", Input{}, false},
		{"empty fields map", "Size", "M", Input{Fields: map[string]string{}}, false},
		{"multiple fields", "Size", "S", Input{Fields: map[string]string{"Status": "Done", "Size": "S"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := FieldMatcher{Field: tt.field, Value: tt.value}
			if got := m.Matches(tt.input); got != tt.expect {
				t.Errorf("FieldMatcher{%q, %q}.Matches() = %v, want %v", tt.field, tt.value, got, tt.expect)
			}
		})
	}
}

func TestParseFieldMatcher(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "field:Size/M", false},
		{"valid with spaces", "field:Story Size/XL", false},
		{"no slash", "field:SizeM", true},
		{"empty name", "field:/M", true},
		{"empty value", "field:Size/", true},
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
		{"field matcher", "field:Size/M", false},
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
		name           string
		input          Input
		expectPrimary  string
		expectMulti    bool
		expectAllCount int
	}{
		{"bug by label", Input{Labels: []string{"bug"}}, "bug", false, 1},
		{"bug by defect label", Input{Labels: []string{"defect"}}, "bug", false, 1},
		{"bug by type", Input{IssueType: "Bug"}, "bug", false, 1},
		{"feature by label", Input{Labels: []string{"enhancement"}}, "feature", false, 1},
		{"feature by type", Input{IssueType: "Feature"}, "feature", false, 1},
		{"regression by label", Input{Labels: []string{"regression"}}, "regression", false, 1},
		{"regression by title", Input{Title: "regression: flaky test"}, "regression", false, 1},
		{"other when no match", Input{Labels: []string{"documentation"}}, "other", false, 0},
		{"empty input", Input{}, "other", false, 0},
		{"multi-match returns all", Input{Labels: []string{"bug", "regression"}}, "bug", true, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(tt.input)
			if result.Category() != tt.expectPrimary {
				t.Errorf("Classify().Category() = %q, want %q", result.Category(), tt.expectPrimary)
			}
			if result.MultiMatch() != tt.expectMulti {
				t.Errorf("Classify().MultiMatch() = %v, want %v", result.MultiMatch(), tt.expectMulti)
			}
			if len(result.Categories) != tt.expectAllCount {
				t.Errorf("Classify().Categories = %v (len %d), want len %d", result.Categories, len(result.Categories), tt.expectAllCount)
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

func TestNewClassifier_InvalidMatcher(t *testing.T) {
	categories := []model.CategoryConfig{
		{Name: "bad", Matchers: []string{"invalid"}},
	}
	_, err := NewClassifier(categories)
	if err == nil {
		t.Error("expected error for invalid matcher, got nil")
	}
}

func TestHasTypeMatchers(t *testing.T) {
	tests := []struct {
		name       string
		categories []model.CategoryConfig
		want       bool
	}{
		{
			"with type matcher",
			[]model.CategoryConfig{
				{Name: "bug", Matchers: []string{"type:Bug"}},
			},
			true,
		},
		{
			"label only",
			[]model.CategoryConfig{
				{Name: "bug", Matchers: []string{"label:bug"}},
			},
			false,
		},
		{
			"title only",
			[]model.CategoryConfig{
				{Name: "bug", Matchers: []string{`title:/^fix/i`}},
			},
			false,
		},
		{
			"mixed - type in second category",
			[]model.CategoryConfig{
				{Name: "bug", Matchers: []string{"label:bug"}},
				{Name: "feature", Matchers: []string{"type:Feature"}},
			},
			true,
		},
		{
			"empty categories",
			[]model.CategoryConfig{},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClassifier(tt.categories)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.HasTypeMatchers(); got != tt.want {
				t.Errorf("HasTypeMatchers() = %v, want %v", got, tt.want)
			}
		})
	}
}
