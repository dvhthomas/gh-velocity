// Package linking implements commit-to-issue linking heuristics.
package linking

import (
	"regexp"
	"strconv"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// Patterns for issue references in commit messages.
var (
	// Matches #N or (#N) — note: also matches PR numbers; caller resolves type.
	hashPattern = regexp.MustCompile(`(?:^|\s|\()#(\d+)(?:\)|\s|$|[.,;:])`)

	// Matches closing keywords: fixes #N, closes #N, resolves #N (case-insensitive).
	closingPattern = regexp.MustCompile(`(?i)(?:fix(?:es|ed)?|close[sd]?|resolve[sd]?)\s+#(\d+)`)
)

// ExtractIssueNumbers returns unique issue numbers referenced in a commit message.
func ExtractIssueNumbers(message string) []int {
	seen := make(map[int]bool)
	var numbers []int

	for _, matches := range closingPattern.FindAllStringSubmatch(message, -1) {
		if n, err := strconv.Atoi(matches[1]); err == nil && n > 0 {
			if !seen[n] {
				seen[n] = true
				numbers = append(numbers, n)
			}
		}
	}

	for _, matches := range hashPattern.FindAllStringSubmatch(message, -1) {
		if n, err := strconv.Atoi(matches[1]); err == nil && n > 0 {
			if !seen[n] {
				seen[n] = true
				numbers = append(numbers, n)
			}
		}
	}

	return numbers
}

// LinkCommitsToIssues groups commits by the issue numbers they reference.
// Returns a map of issue number → commits referencing that issue.
func LinkCommitsToIssues(commits []model.Commit) map[int][]model.Commit {
	result := make(map[int][]model.Commit)
	for _, c := range commits {
		for _, n := range ExtractIssueNumbers(c.Message) {
			result[n] = append(result[n], c)
		}
	}
	return result
}
