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
	Scope     string // user scope from config + flag
	Lifecycle string // command lifecycle qualifiers (e.g., "is:closed closed:2026-01-01..2026-02-01")
	Type      string // "is:issue" or "is:pr" (from strategy)
}

// Build assembles the full search query string by joining non-empty parts.
func (q Query) Build() string {
	var parts []string
	for _, p := range []string{q.Scope, q.Type, q.Lifecycle} {
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
	return "https://github.com/issues?q=" + url.QueryEscape(query)
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
	fmt.Fprintf(&sb, "[query]     %s\n", q.Build())
	if u := q.URL(); u != "" {
		fmt.Fprintf(&sb, "[url]       %s\n", u)
	}
	return sb.String()
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
