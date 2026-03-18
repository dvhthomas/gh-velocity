package strategy

import (
	"context"
	"regexp"
	"strconv"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// CommitRef discovers issues by scanning commit messages for references.
// By default only matches closing keywords (fixes #N, closes #N, resolves #N).
// Bare #N matching is opt-in via the "refs" pattern.
type CommitRef struct {
	patterns []string
}

// NewCommitRef creates a new commit-ref strategy.
// patterns controls which patterns to match: "closes" (default), "refs" (bare #N).
func NewCommitRef(patterns []string) *CommitRef {
	if len(patterns) == 0 {
		patterns = []string{"closes"}
	}
	return &CommitRef{patterns: patterns}
}

func (s *CommitRef) Name() string { return "commit-ref" }

// closingPattern matches closing keywords: fixes #N, closes #N, resolves #N.
var closingPattern = regexp.MustCompile(`(?i)(?:fix(?:es|ed)?|close[sd]?|resolve[sd]?)\s+#(\d+)`)

// refsPattern matches bare #N references.
var refsPattern = regexp.MustCompile(`(?:^|\s|\()#(\d+)(?:\)|\s|$|[.,;:])`)

func (s *CommitRef) Discover(ctx context.Context, input DiscoverInput) ([]model.DiscoveredItem, error) {
	// Build the set of active patterns.
	var activePatterns []*regexp.Regexp
	for _, p := range s.patterns {
		switch p {
		case "closes":
			activePatterns = append(activePatterns, closingPattern)
		case "refs":
			activePatterns = append(activePatterns, refsPattern)
		}
	}

	if len(activePatterns) == 0 {
		activePatterns = []*regexp.Regexp{closingPattern}
	}

	// Scan commits and collect issue references.
	issueCommits := make(map[int][]model.Commit)
	for _, c := range input.Commits {
		seen := make(map[int]bool)
		for _, pat := range activePatterns {
			for _, matches := range pat.FindAllStringSubmatch(c.Message, -1) {
				n, err := strconv.Atoi(matches[1])
				if err != nil || n <= 0 {
					continue
				}
				if !seen[n] {
					seen[n] = true
					issueCommits[n] = append(issueCommits[n], c)
				}
			}
		}
	}

	// Build discovered items. We don't have issue metadata at this point —
	// the caller (Runner or release command) fetches full issue data later.
	var items []model.DiscoveredItem
	for issueNum, commits := range issueCommits {
		items = append(items, model.DiscoveredItem{
			Issue: &model.Issue{
				Number: issueNum,
			},
			Commits:  commits,
			Strategy: s.Name(),
		})
	}

	return items, nil
}
