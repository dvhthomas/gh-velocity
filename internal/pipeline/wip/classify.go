// Package wip implements the WIP detail report pipeline.
package wip

import (
	"github.com/dvhthomas/gh-velocity/internal/classify"
)

// classifyItem determines the lifecycle stage for an item based on its labels,
// issue type, title, and PR-specific signals.
//
// Returns:
//   - stage: "In Progress" or "In Review"
//   - matchedMatcher: the matcher string that classified this item (e.g. "label:in-progress", "draft", "open-pr")
//   - excluded: true if the item should be excluded from WIP (no matcher match and not a PR)
func classifyItem(
	labels []string,
	issueType string,
	title string,
	isPR bool,
	isDraft bool,
	inProgressMatchers []string,
	inReviewMatchers []string,
) (stage string, matchedMatcher string, excluded bool) {
	input := classify.Input{
		Labels:    labels,
		IssueType: issueType,
		Title:     title,
	}

	// Check in-review first (more specific).
	for _, matcherStr := range inReviewMatchers {
		m, err := classify.ParseMatcher(matcherStr)
		if err != nil {
			continue
		}
		if m.Matches(input) {
			return "In Review", matcherStr, false
		}
	}

	// Check in-progress matchers.
	for _, matcherStr := range inProgressMatchers {
		m, err := classify.ParseMatcher(matcherStr)
		if err != nil {
			continue
		}
		if m.Matches(input) {
			return "In Progress", matcherStr, false
		}
	}

	// No matcher matched — use native signals for PRs.
	if isPR {
		if isDraft {
			return "In Progress", "draft", false
		}
		return "In Review", "open-pr", false
	}

	// Issues with no matcher match are excluded (not WIP).
	return "", "", true
}
