// Package dateutil provides date parsing and validation for CLI date flags.
// All returned times are UTC.
package dateutil

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Parse accepts YYYY-MM-DD, RFC3339, or relative (Nd).
// All returned times are UTC.
//   - YYYY-MM-DD  -> start of day UTC
//   - RFC3339     -> as-is, converted to UTC
//   - Nd          -> now.UTC() minus N days (0d = start of today UTC)
func Parse(s string, now time.Time) (time.Time, error) {
	now = now.UTC()

	// Relative: Nd
	if strings.HasSuffix(s, "d") {
		numStr := strings.TrimSuffix(s, "d")
		n, err := strconv.Atoi(numStr)
		if err != nil || n < 0 {
			return time.Time{}, fmt.Errorf("invalid date %q: expected YYYY-MM-DD, RFC3339, or relative (e.g., 30d)", s)
		}
		if n == 0 {
			// Start of today UTC
			return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
		}
		return now.AddDate(0, 0, -n), nil
	}

	// RFC3339: contains 'T'
	if strings.Contains(s, "T") {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date %q: expected YYYY-MM-DD, RFC3339, or relative (e.g., 30d)", s)
		}
		return t.UTC(), nil
	}

	// YYYY-MM-DD
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: expected YYYY-MM-DD, RFC3339, or relative (e.g., 30d)", s)
	}
	return t.UTC(), nil
}

// MaxWindowDays is the maximum allowed date window to prevent expensive API queries.
const MaxWindowDays = 90

// ValidateWindow checks that since < until, since is not in the future,
// and the window does not exceed MaxWindowDays.
func ValidateWindow(since, until, now time.Time) error {
	now = now.UTC()
	if since.After(now) {
		return fmt.Errorf("--since %s is in the future", since.Format(time.RFC3339))
	}
	if !since.Before(until) {
		return fmt.Errorf("--since %s must be before --until %s", since.Format(time.RFC3339), until.Format(time.RFC3339))
	}
	if until.Sub(since) > time.Duration(MaxWindowDays)*24*time.Hour {
		return fmt.Errorf("date window exceeds maximum of %d days; narrow with --since/--until", MaxWindowDays)
	}
	return nil
}
