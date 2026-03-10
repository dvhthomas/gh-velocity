// Package classify provides flexible issue/PR classification with user-defined
// categories and multiple matcher types (label, type, title regex).
package classify

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// Input holds the fields available for classification matching.
type Input struct {
	Labels    []string // issue or PR labels
	IssueType string   // GitHub Issue Type (from GraphQL)
	Title     string   // issue or PR title
}

// Matcher tests whether an input matches a classification rule.
type Matcher interface {
	Matches(input Input) bool
}

// Classifier evaluates items against ordered category rules.
// Categories are evaluated in order; first match wins. Unmatched items
// are classified as "other".
type Classifier struct {
	categories []model.CategoryConfig
	matchers   [][]Matcher // parsed matchers per category, same index as categories
}

// NewClassifier creates a Classifier from category configs. Returns an error
// if any matcher string is malformed.
func NewClassifier(categories []model.CategoryConfig) (*Classifier, error) {
	c := &Classifier{
		categories: categories,
		matchers:   make([][]Matcher, len(categories)),
	}
	for i, cat := range categories {
		for _, s := range cat.Matchers {
			m, err := ParseMatcher(s)
			if err != nil {
				return nil, fmt.Errorf("category %q: %w", cat.Name, err)
			}
			c.matchers[i] = append(c.matchers[i], m)
		}
	}
	return c, nil
}

// ClassifyResult holds a classification outcome and any warnings.
type ClassifyResult struct {
	Category string
	Warnings []string
}

// Classify returns the category name for the given input.
// Returns "other" if no category matches. If the input matches multiple
// categories, the first match wins and a warning is included.
func (c *Classifier) Classify(input Input) ClassifyResult {
	var matched []string
	for i, ms := range c.matchers {
		for _, m := range ms {
			if m.Matches(input) {
				matched = append(matched, c.categories[i].Name)
				break // one match per category is enough
			}
		}
	}

	if len(matched) == 0 {
		return ClassifyResult{Category: "other"}
	}

	result := ClassifyResult{Category: matched[0]}
	if len(matched) > 1 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("matches multiple categories %v — classified as %q (first match)", matched, matched[0]))
	}
	return result
}

// CategoryNames returns the ordered list of category names (excluding "other").
func (c *Classifier) CategoryNames() []string {
	names := make([]string, len(c.categories))
	for i, cat := range c.categories {
		names[i] = cat.Name
	}
	return names
}

// ParseMatcher parses a matcher string into a Matcher implementation.
// Supported formats:
//   - "label:<name>"   — case-insensitive label match
//   - "type:<name>"    — exact match on GitHub Issue Type
//   - "title:/<regex>/<flags>" — regex match on title (flag: i = case-insensitive)
func ParseMatcher(s string) (Matcher, error) {
	prefix, value, ok := strings.Cut(s, ":")
	if !ok || value == "" {
		return nil, fmt.Errorf("invalid matcher %q: expected format \"label:<name>\", \"type:<name>\", or \"title:/<regex>/\"", s)
	}

	switch prefix {
	case "label":
		return LabelMatcher{Label: value}, nil
	case "type":
		return TypeMatcher{Type: value}, nil
	case "title":
		return parseTitleMatcher(value)
	default:
		return nil, fmt.Errorf("unknown matcher type %q in %q: expected \"label\", \"type\", or \"title\"", prefix, s)
	}
}

// LabelMatcher matches issues/PRs that have a specific label (case-insensitive).
type LabelMatcher struct {
	Label string
}

func (m LabelMatcher) Matches(input Input) bool {
	for _, l := range input.Labels {
		if strings.EqualFold(l, m.Label) {
			return true
		}
	}
	return false
}

// TypeMatcher matches issues with a specific GitHub Issue Type (exact match).
type TypeMatcher struct {
	Type string
}

func (m TypeMatcher) Matches(input Input) bool {
	return input.IssueType == m.Type
}

// TitleMatcher matches issues/PRs whose title matches a regex.
type TitleMatcher struct {
	Pattern *regexp.Regexp
}

func (m TitleMatcher) Matches(input Input) bool {
	return m.Pattern.MatchString(input.Title)
}

// parseTitleMatcher parses a regex pattern like "/regex/" or "/regex/i".
func parseTitleMatcher(value string) (Matcher, error) {
	if !strings.HasPrefix(value, "/") {
		return nil, fmt.Errorf("title matcher must be a regex like \"/pattern/\" or \"/pattern/i\", got %q", value)
	}

	// Find the closing slash
	lastSlash := strings.LastIndex(value[1:], "/")
	if lastSlash < 0 {
		return nil, fmt.Errorf("title matcher missing closing /: %q", value)
	}
	lastSlash++ // adjust for the offset from value[1:]

	pattern := value[1:lastSlash]
	flags := value[lastSlash+1:]

	if strings.Contains(flags, "i") {
		pattern = "(?i)" + pattern
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid title regex %q: %w", value, err)
	}

	return TitleMatcher{Pattern: re}, nil
}

// FromLegacyLabels creates category configs from the legacy bug_labels/feature_labels
// config format. This provides backward compatibility.
func FromLegacyLabels(bugLabels, featureLabels []string) []model.CategoryConfig {
	var categories []model.CategoryConfig

	if len(bugLabels) > 0 {
		matchers := make([]string, len(bugLabels))
		for i, l := range bugLabels {
			matchers[i] = "label:" + l
		}
		categories = append(categories, model.CategoryConfig{Name: "bug", Matchers: matchers})
	}

	if len(featureLabels) > 0 {
		matchers := make([]string, len(featureLabels))
		for i, l := range featureLabels {
			matchers[i] = "label:" + l
		}
		categories = append(categories, model.CategoryConfig{Name: "feature", Matchers: matchers})
	}

	return categories
}
