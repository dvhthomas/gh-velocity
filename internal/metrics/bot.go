package metrics

import "strings"

// knownBots lists well-known bot account logins (lowercase).
var knownBots = map[string]bool{
	"dependabot":     true,
	"renovate":       true,
	"copilot":        true,
	"github-actions": true,
}

// IsBotUser returns true if the login matches known bot patterns.
// Checks (in order): wip.bots config list, exclude_users config list,
// common bot suffixes ([bot], -bot), and well-known bot account names.
// All comparisons are case-insensitive exact match.
func IsBotUser(login string, configBots []string, excludeUsers []string) bool {
	// Check explicit bot list from wip.bots config (case-insensitive).
	for _, b := range configBots {
		if strings.EqualFold(login, b) {
			return true
		}
	}

	// Check exclude_users (often includes bots like dependabot[bot]).
	for _, u := range excludeUsers {
		if strings.EqualFold(login, u) {
			return true
		}
	}

	// Pattern-based detection.
	lower := strings.ToLower(login)
	if strings.HasSuffix(lower, "[bot]") {
		return true
	}
	if strings.HasSuffix(lower, "-bot") {
		return true
	}
	return knownBots[lower]
}
