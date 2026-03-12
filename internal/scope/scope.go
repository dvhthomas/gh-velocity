// Package scope assembles GitHub search queries from config and flags.
// It has zero API dependency — pure string manipulation and URL encoding.
package scope

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Query holds the components of a search query.
type Query struct {
	Scope        string // user scope from config + flag
	Lifecycle    string // command lifecycle qualifiers (e.g., "is:closed closed:2026-01-01..2026-02-01")
	Type         string // "is:issue" or "is:pr" (from strategy)
	ExcludeUsers string // "-author:bot1 -author:bot2" exclusion qualifiers
}

// Build assembles the full search query string by joining non-empty parts.
func (q Query) Build() string {
	var parts []string
	for _, p := range []string{q.Scope, q.Type, q.Lifecycle, q.ExcludeUsers} {
		if s := strings.TrimSpace(p); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// URL returns a clickable GitHub search URL for the assembled query.
func (q Query) URL() string {
	query := q.Build()
	if query == "" {
		return ""
	}
	return "https://github.com/search?q=" + url.QueryEscape(query)
}

// Verbose returns a multi-line diagnostic string for --debug output.
func (q Query) Verbose() string {
	var sb strings.Builder
	if q.Scope != "" {
		fmt.Fprintf(&sb, "[scope]     %s\n", q.Scope)
	}
	if q.Type != "" {
		fmt.Fprintf(&sb, "[type]      %s\n", q.Type)
	}
	if q.Lifecycle != "" {
		fmt.Fprintf(&sb, "[lifecycle] %s\n", q.Lifecycle)
	}
	if q.ExcludeUsers != "" {
		fmt.Fprintf(&sb, "[exclude]   %s\n", q.ExcludeUsers)
	}
	fmt.Fprintf(&sb, "[query]     %s\n", q.Build())
	if u := q.URL(); u != "" {
		fmt.Fprintf(&sb, "[url]       %s\n", u)
	}
	return sb.String()
}

// BuildExclusions builds a search qualifier string that excludes the given
// usernames from results. Each user becomes a "-author:username" qualifier.
// Returns empty string if no users are provided.
func BuildExclusions(users []string) string {
	if len(users) == 0 {
		return ""
	}
	parts := make([]string, len(users))
	for i, u := range users {
		parts[i] = "-author:" + u
	}
	return strings.Join(parts, " ")
}

// ClosedIssueQuery returns a Query for issues closed in the given date range.
func ClosedIssueQuery(scopeStr string, since, until time.Time) Query {
	return Query{
		Scope:     scopeStr,
		Type:      "is:issue",
		Lifecycle: fmt.Sprintf("is:closed closed:%s..%s", since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339)),
	}
}

// MergedPRQuery returns a Query for PRs merged in the given date range.
func MergedPRQuery(scopeStr string, since, until time.Time) Query {
	return Query{
		Scope:     scopeStr,
		Type:      "is:pr",
		Lifecycle: fmt.Sprintf("is:merged merged:%s..%s", since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339)),
	}
}

// ClosedIssuesByAuthorQuery returns a Query for issues closed by a specific author.
func ClosedIssuesByAuthorQuery(scopeStr, login string, since, until time.Time) Query {
	return Query{
		Scope:     scopeStr,
		Type:      "is:issue",
		Lifecycle: fmt.Sprintf("is:closed author:%s closed:%s..%s", login, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339)),
	}
}

// MergedPRsByAuthorQuery returns a Query for PRs merged by a specific author.
func MergedPRsByAuthorQuery(scopeStr, login string, since, until time.Time) Query {
	return Query{
		Scope:     scopeStr,
		Type:      "is:pr",
		Lifecycle: fmt.Sprintf("is:merged author:%s merged:%s..%s", login, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339)),
	}
}

// ReviewedPRsByAuthorQuery returns a Query for PRs reviewed by a specific user.
func ReviewedPRsByAuthorQuery(scopeStr, login string, since, until time.Time) Query {
	return Query{
		Scope:     scopeStr,
		Type:      "is:pr",
		Lifecycle: fmt.Sprintf("reviewed-by:%s updated:%s..%s", login, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339)),
	}
}

// OpenIssuesByAssigneeQuery returns a Query for open issues assigned to a user.
func OpenIssuesByAssigneeQuery(scopeStr, login string) Query {
	return Query{
		Scope:     scopeStr,
		Type:      "is:issue",
		Lifecycle: fmt.Sprintf("is:open assignee:%s", login),
	}
}

// OpenPRsByAuthorQuery returns a Query for open PRs authored by a user.
func OpenPRsByAuthorQuery(scopeStr, login string) Query {
	return Query{
		Scope:     scopeStr,
		Type:      "is:pr",
		Lifecycle: fmt.Sprintf("is:open author:%s", login),
	}
}

// OpenPRsNeedingReviewQuery returns a Query for open PRs authored by a user
// that have received zero reviews. Used to detect "waiting for review" state.
func OpenPRsNeedingReviewQuery(scopeStr, login string) Query {
	return Query{
		Scope:     scopeStr,
		Type:      "is:pr",
		Lifecycle: fmt.Sprintf("is:open author:%s review:none", login),
	}
}

// ReviewRequestedQuery returns a Query for open PRs where the user has been
// requested as a reviewer. These are PRs from other people waiting on you.
func ReviewRequestedQuery(scopeStr, login string) Query {
	return Query{
		Scope:     scopeStr,
		Type:      "is:pr",
		Lifecycle: fmt.Sprintf("is:open review-requested:%s", login),
	}
}

// OpenPRsAwaitingReviewSearchURL returns a clickable GitHub search URL for
// open PRs awaiting review in the given repository.
func OpenPRsAwaitingReviewSearchURL(owner, repo string) string {
	q := Query{
		Scope:     fmt.Sprintf("repo:%s/%s", owner, repo),
		Type:      "is:pr",
		Lifecycle: "is:open review:required",
	}
	return q.URL()
}

// MergeScope combines config scope and flag scope with AND semantics.
// Both are GitHub search query fragments; they're joined with a space.
// Empty strings are ignored.
func MergeScope(configScope, flagScope string) string {
	c := strings.TrimSpace(configScope)
	f := strings.TrimSpace(flagScope)
	switch {
	case c != "" && f != "":
		return c + " " + f
	case c != "":
		return c
	default:
		return f
	}
}
