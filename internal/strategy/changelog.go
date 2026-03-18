package strategy

import (
	"context"
	"regexp"
	"strconv"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// Changelog discovers issues/PRs by parsing the GitHub Release body.
type Changelog struct{}

// NewChangelog creates a new changelog strategy.
func NewChangelog() *Changelog {
	return &Changelog{}
}

func (s *Changelog) Name() string { return "changelog" }

// changelogRefPattern matches #N, owner/repo#N, and full GitHub URLs.
var changelogRefPattern = regexp.MustCompile(`(?:^|\s|[([]?)#(\d+)(?:[)\].,;:\s]|$)`)

func (s *Changelog) Discover(ctx context.Context, input DiscoverInput) ([]model.DiscoveredItem, error) {
	if input.Release == nil {
		return nil, nil
	}

	body := input.Release.Body
	if body == "" {
		return nil, nil
	}

	seen := make(map[int]bool)
	var items []model.DiscoveredItem

	for _, matches := range changelogRefPattern.FindAllStringSubmatch(body, -1) {
		n, err := strconv.Atoi(matches[1])
		if err != nil || n <= 0 {
			continue
		}
		if seen[n] {
			continue
		}
		seen[n] = true

		// We don't know if it's an issue or PR at this point —
		// the caller resolves the type via API.
		items = append(items, model.DiscoveredItem{
			Issue: &model.Issue{
				Number: n,
			},
			Strategy: s.Name(),
		})
	}

	return items, nil
}
