package strategy

import (
	"context"
	"fmt"

	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/dvhthomas/gh-velocity/internal/scope"
)

// PRLink discovers issues via PR → linked issue references.
// Uses the GitHub search API to find merged PRs in the date range,
// then GraphQL closingIssuesReferences to get linked issues.
type PRLink struct{}

// NewPRLink creates a new pr-link strategy.
func NewPRLink() *PRLink {
	return &PRLink{}
}

func (s *PRLink) Name() string { return "pr-link" }

func (s *PRLink) Discover(ctx context.Context, input DiscoverInput) ([]model.DiscoveredItem, error) {
	if input.Client == nil {
		return nil, fmt.Errorf("pr-link strategy requires a GitHub API client")
	}

	// Search for merged PRs in the date range between tags.
	if input.PrevTagDate.IsZero() || input.TagDate.IsZero() {
		return nil, fmt.Errorf("pr-link strategy requires tag dates")
	}

	q := scope.MergedPRQuery(input.Scope, input.PrevTagDate, input.TagDate)
	prs, err := input.Client.SearchPRs(ctx, q.Build())
	if err != nil {
		return nil, fmt.Errorf("search merged PRs: %w", err)
	}

	if len(prs) == 0 {
		return nil, nil
	}

	if len(prs) >= 1000 {
		log.Warn("pr-link: search returned 1000+ PRs (API limit), results may be incomplete")
	}

	// Collect PR numbers for batch GraphQL query.
	prNumbers := make([]int, len(prs))
	prByNumber := make(map[int]*model.PR, len(prs))
	for i := range prs {
		prNumbers[i] = prs[i].Number
		prByNumber[prs[i].Number] = &prs[i]
	}

	// Fetch linked issues for all PRs via batched GraphQL.
	linkedIssues, err := input.Client.FetchPRLinkedIssues(ctx, prNumbers)
	if err != nil {
		return nil, fmt.Errorf("fetch PR linked issues: %w", err)
	}

	var items []model.DiscoveredItem

	for _, prNum := range prNumbers {
		pr := prByNumber[prNum]
		issues := linkedIssues[prNum]

		if len(issues) == 0 {
			// PR with no linked issues — PR is the work unit.
			items = append(items, model.DiscoveredItem{
				PR:       pr,
				Strategy: s.Name(),
			})
			continue
		}

		// One DiscoveredItem per linked issue, each referencing the PR.
		for i := range issues {
			items = append(items, model.DiscoveredItem{
				Issue:    &issues[i],
				PR:       pr,
				Strategy: s.Name(),
			})
		}
	}

	return items, nil
}
