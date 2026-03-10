// Package classify provides flexible issue classification using user-defined
// categories and matchers. Each category has one or more matchers; the first
// matching category wins.
package classify

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// Matcher tests whether an issue belongs to a category.
type Matcher interface {
	Matches(issue model.Issue) bool
}

// LabelMatcher matches issues that have a specific label (case-insensitive).
type LabelMatcher struct {
	Label string
}

func (m LabelMatcher) Matches(issue model.Issue) bool {
	for _, l := range issue.Labels {
		if strings.EqualFold(l, m.Label) {
			return true
		}
	}
	return false
}

// TitleMatcher matches issues whose title matches a compiled regex.
type TitleMatcher struct {
	Pattern *regexp.Regexp
}

func (m TitleMatcher) Matches(issue model.Issue) bool {
	return m.Pattern.MatchString(issue.Title)
}

// ParseMatcher parses a matcher string like "label:bug" or "title:/regex/i".
func ParseMatcher(s string) (Matcher, error) {
	prefix, value, ok := strings.Cut(s, ":")
	if !ok || value == "" {
		return nil, fmt.Errorf("invalid matcher %q: expected \"type:value\"", s)
	}

	switch prefix {
	case "label":
		return LabelMatcher{Label: value}, nil
	case "title":
		re, err := parseRegex(value)
		if err != nil {
			return nil, fmt.Errorf("invalid title matcher %q: %w", s, err)
		}
		return TitleMatcher{Pattern: re}, nil
	default:
		return nil, fmt.Errorf("unknown matcher type %q in %q", prefix, s)
	}
}

// parseRegex parses a regex string, supporting /pattern/flags syntax.
func parseRegex(s string) (*regexp.Regexp, error) {
	if strings.HasPrefix(s, "/") {
		// Find the last "/" for flags.
		lastSlash := strings.LastIndex(s[1:], "/")
		if lastSlash == -1 {
			return nil, fmt.Errorf("unterminated regex: missing closing /")
		}
		pattern := s[1 : lastSlash+1]
		flags := s[lastSlash+2:]
		if strings.Contains(flags, "i") {
			pattern = "(?i)" + pattern
		}
		return regexp.Compile(pattern)
	}
	return regexp.Compile(s)
}

// Classifier assigns categories to issues based on configured matchers.
type Classifier struct {
	// Categories in order of evaluation. First match wins.
	categories []category
}

type category struct {
	Name     string
	Matchers []Matcher
}

// New creates a Classifier from a categories map (category name → matcher strings).
// Returns an error if any matcher string is invalid.
func New(cats map[string][]string) (*Classifier, error) {
	c := &Classifier{}
	for name, matcherStrs := range cats {
		var matchers []Matcher
		for _, s := range matcherStrs {
			m, err := ParseMatcher(s)
			if err != nil {
				return nil, fmt.Errorf("category %q: %w", name, err)
			}
			matchers = append(matchers, m)
		}
		c.categories = append(c.categories, category{Name: name, Matchers: matchers})
	}
	return c, nil
}

// Classify returns the first matching category name, or "other" if none match.
func (c *Classifier) Classify(issue model.Issue) string {
	for _, cat := range c.categories {
		for _, m := range cat.Matchers {
			if m.Matches(issue) {
				return cat.Name
			}
		}
	}
	return "other"
}
