package posting

import (
	"fmt"
	"strings"
)

// ParsedTarget represents a parsed owner/repo/category discussion target.
type ParsedTarget struct {
	Owner    string
	Repo     string
	Category string
}

// ParseTarget parses an "owner/repo/category" string.
// If the category name contains a slash, it must be quoted:
//
//	myorg/myrepo/"My / Category"
func ParseTarget(s string) (ParsedTarget, error) {
	var dt ParsedTarget

	// Need at least owner/repo/category — two slashes minimum.
	firstSlash := strings.Index(s, "/")
	if firstSlash == -1 {
		return dt, fmt.Errorf("discussions.category must be owner/repo/category, got %q", s)
	}

	dt.Owner = s[:firstSlash]
	rest := s[firstSlash+1:]

	// Find the second slash (separates repo from category).
	secondSlash := strings.Index(rest, "/")
	if secondSlash == -1 {
		return dt, fmt.Errorf("discussions.category must be owner/repo/category, got %q", s)
	}

	dt.Repo = rest[:secondSlash]
	catRaw := rest[secondSlash+1:]

	// Handle quoted category: "My / Category"
	if strings.HasPrefix(catRaw, `"`) {
		if !strings.HasSuffix(catRaw, `"`) || len(catRaw) < 2 {
			return dt, fmt.Errorf("discussions.category has unclosed quote in %q", s)
		}
		dt.Category = catRaw[1 : len(catRaw)-1]
	} else {
		dt.Category = catRaw
	}

	// Validate non-empty parts.
	if dt.Owner == "" {
		return dt, fmt.Errorf("discussions.category has empty owner in %q", s)
	}
	if dt.Repo == "" {
		return dt, fmt.Errorf("discussions.category has empty repo in %q", s)
	}
	if dt.Category == "" {
		return dt, fmt.Errorf("discussions.category has empty category in %q", s)
	}

	return dt, nil
}
