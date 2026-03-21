package github

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// EnrichIssueTypes batch-fetches IssueType for the given issues via GraphQL
// and sets it in-place. Issues that already have IssueType set are skipped.
// Batches in groups of 20 (matching FetchIssues pattern).
// Errors are non-fatal: failed batches log a warning and leave IssueType
// empty, allowing classification to fall through to label/title matchers.
func (c *Client) EnrichIssueTypes(ctx context.Context, issues []model.Issue) error {
	if len(issues) == 0 {
		return nil
	}

	// Collect issue numbers that need enrichment (IssueType not already set).
	type enrichTarget struct {
		index  int // position in the original slice
		number int
	}
	var targets []enrichTarget
	for i := range issues {
		if issues[i].IssueType == "" {
			targets = append(targets, enrichTarget{index: i, number: issues[i].Number})
		}
	}
	if len(targets) == 0 {
		return nil
	}

	// Build a sorted list of numbers for a stable cache key.
	numbers := make([]int, len(targets))
	for i, t := range targets {
		numbers[i] = t.number
	}
	sort.Ints(numbers)

	numberStrs := make([]string, len(numbers))
	for i, n := range numbers {
		numberStrs[i] = strconv.Itoa(n)
	}
	key := CacheKey("enrich-issue-types", c.owner, c.repo, strings.Join(numberStrs, ","))

	typeMap, err := c.cache.DoJSON(key, "enrich-issue-types",
		func() (any, error) {
			return c.fetchIssueTypesBatched(ctx, numbers)
		},
		func(raw json.RawMessage) (any, error) {
			var m map[int]string
			return m, json.Unmarshal(raw, &m)
		},
	)
	if err != nil {
		log.Debug("issue type enrichment failed: %v", err)
		return nil // non-fatal
	}

	// Apply enriched types to original slice by index.
	resolved := typeMap.(map[int]string)
	for _, t := range targets {
		if typeName, ok := resolved[t.number]; ok {
			issues[t.index].IssueType = typeName
		}
	}

	log.Debug("enriched %d/%d issues with IssueType", len(resolved), len(targets))
	return nil
}

const enrichBatchSize = 20

// fetchIssueTypesBatched fetches only the issueType for the given issue
// numbers via aliased GraphQL queries. Returns a map of number -> type name.
func (c *Client) fetchIssueTypesBatched(ctx context.Context, numbers []int) (map[int]string, error) {
	result := make(map[int]string)

	for i := 0; i < len(numbers); i += enrichBatchSize {
		end := min(i+enrichBatchSize, len(numbers))
		batch := numbers[i:end]

		batchResult, err := c.fetchIssueTypesBatch(ctx, batch)
		if err != nil {
			log.Debug("issue type enrichment batch failed (issues %d-%d): %v", batch[0], batch[len(batch)-1], err)
			continue // non-fatal: skip failed batches
		}
		for k, v := range batchResult {
			result[k] = v
		}
	}

	return result, nil
}

// fetchIssueTypesBatch fetches issueType for a single batch via GraphQL aliases.
func (c *Client) fetchIssueTypesBatch(ctx context.Context, numbers []int) (map[int]string, error) {
	var queryFragments strings.Builder
	for _, num := range numbers {
		queryFragments.WriteString(fmt.Sprintf(`
    issue%d: issue(number: %d) {
      number
      issueType { name }
    }`, num, num))
	}

	query := fmt.Sprintf(`query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {%s
  }
}`, queryFragments.String())

	variables := map[string]any{
		"owner": c.owner,
		"name":  c.repo,
	}

	var resp struct {
		Repository map[string]json.RawMessage
	}
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("enrich issue types batch: %w", err)
	}

	result := make(map[int]string)
	for _, num := range numbers {
		alias := fmt.Sprintf("issue%d", num)
		raw, ok := resp.Repository[alias]
		if !ok {
			continue
		}
		var node struct {
			Number    int `json:"number"`
			IssueType *struct {
				Name string `json:"name"`
			} `json:"issueType"`
		}
		if err := json.Unmarshal(raw, &node); err != nil {
			return nil, fmt.Errorf("unmarshal issue #%d type: %w", num, err)
		}
		if node.IssueType != nil {
			result[node.Number] = node.IssueType.Name
		}
	}

	return result, nil
}
