package classify

import (
	"testing"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestLabelMatcher(t *testing.T) {
	tests := []struct {
		name   string
		label  string
		issue  model.Issue
		expect bool
	}{
		{"exact match", "bug", model.Issue{Labels: []string{"bug"}}, true},
		{"case insensitive", "BUG", model.Issue{Labels: []string{"bug"}}, true},
		{"no match", "bug", model.Issue{Labels: []string{"feature"}}, false},
		{"empty labels", "bug", model.Issue{Labels: nil}, false},
		{"multiple labels", "bug", model.Issue{Labels: []string{"docs", "bug", "urgent"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := LabelMatcher{Label: tt.label}
			if got := m.Matches(tt.issue); got != tt.expect {
				t.Errorf("LabelMatcher{%q}.Matches() = %v, want %v", tt.label, got, tt.expect)
			}
		})
	}
}

func TestTitleMatcher(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		title   string
		expect  bool
	}{
		{"simple regex", "^fix:", "fix: broken thing", true},
		{"no match", "^fix:", "feat: new thing", false},
		{"case insensitive flag", "/^regression:/i", "Regression: something broke", true},
		{"bare regex", "hotfix", "apply hotfix for crash", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := ParseMatcher("title:" + tt.pattern)
			if err != nil {
				t.Fatalf("ParseMatcher error: %v", err)
			}
			issue := model.Issue{Title: tt.title}
			if got := m.Matches(issue); got != tt.expect {
				t.Errorf("TitleMatcher{%q}.Matches(%q) = %v, want %v", tt.pattern, tt.title, got, tt.expect)
			}
		})
	}
}

func TestParseMatcher(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"label:bug", false},
		{"title:/regex/i", false},
		{"title:simple", false},
		{"unknown:foo", true},
		{"nocolon", true},
		{"label:", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := ParseMatcher(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMatcher(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestClassifier(t *testing.T) {
	cats := map[string][]string{
		"bug":        {"label:bug", "label:defect"},
		"feature":    {"label:enhancement"},
		"regression": {"title:/^regression:/i"},
	}

	c, err := New(cats)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	tests := []struct {
		name   string
		issue  model.Issue
		expect string
	}{
		{"bug label", model.Issue{Labels: []string{"bug"}}, "bug"},
		{"defect label", model.Issue{Labels: []string{"defect"}}, "bug"},
		{"feature label", model.Issue{Labels: []string{"enhancement"}}, "feature"},
		{"regression title", model.Issue{Title: "Regression: login broken"}, "regression"},
		{"no match", model.Issue{Labels: []string{"docs"}, Title: "update docs"}, "other"},
		{"empty issue", model.Issue{}, "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Classify(tt.issue)
			if got != tt.expect {
				t.Errorf("Classify() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestNew_InvalidMatcher(t *testing.T) {
	cats := map[string][]string{
		"bad": {"title:/[invalid"},
	}
	_, err := New(cats)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}
