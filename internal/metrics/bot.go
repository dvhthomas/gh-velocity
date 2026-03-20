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
// Checks the exclude_users config list, common bot suffixes, and
// well-known bot account names.
func IsBotUser(login string, excludeUsers []string) bool {
	for _, u := range excludeUsers {
		if strings.EqualFold(login, u) {
			return true
		}
	}

	lower := strings.ToLower(login)
	if strings.HasSuffix(lower, "[bot]") {
		return true
	}
	if strings.HasSuffix(lower, "-bot") {
		return true
	}
	return knownBots[lower]
}
