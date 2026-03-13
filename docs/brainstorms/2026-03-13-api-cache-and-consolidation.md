---
title: API Call Consolidation and Cache Layer
date: 2026-03-13
status: brainstorm
---

# API Call Consolidation and Cache Layer

## Problem

gh-velocity hits GitHub's secondary rate limits because commands make many
sequential search API calls. The velocity command with 6 iterations makes
7 search calls; my-week makes 7 concurrent search calls; running multiple
commands compounds the problem.

## Approach 1: Query Result Cache (internal/github/cache.go)

A thin, in-process cache layer in the GitHub client that deduplicates
identical API calls within a short TTL window. This benefits:

- **velocity + wip in the same session**: both query the project board
- **report command**: runs lead-time, cycle-time, throughput concurrently —
  overlapping date-range queries can share results
- **CI pipelines**: running multiple `gh velocity` commands sequentially

### Design

```go
// internal/github/cache.go

// QueryCache provides short-lived deduplication of GitHub API responses.
// Cache key = hash(method + url/query + variables). TTL = 10 minutes.
// Not thread-safe across processes — single CLI invocation only.
type QueryCache struct {
    entries map[string]cacheEntry
    ttl     time.Duration
}

type cacheEntry struct {
    data      []byte    // raw JSON response
    createdAt time.Time
}

func NewQueryCache(ttl time.Duration) *QueryCache {
    return &QueryCache{
        entries: make(map[string]cacheEntry),
        ttl:     ttl,
    }
}

func (c *QueryCache) Get(key string) ([]byte, bool) {
    e, ok := c.entries[key]
    if !ok || time.Since(e.createdAt) > c.ttl {
        delete(c.entries, key)
        return nil, false
    }
    return e.data, true
}

func (c *QueryCache) Set(key string, data []byte) {
    c.entries[key] = cacheEntry{data: data, createdAt: time.Now()}
}

// Key returns a cache key for the given query parameters.
func Key(parts ...string) string {
    h := sha256.New()
    for _, p := range parts {
        h.Write([]byte(p))
        h.Write([]byte{0})
    }
    return hex.EncodeToString(h.Sum(nil))[:16]
}
```

### Integration Point

Add `cache *QueryCache` to `Client`. Wrap `SearchIssues`, `SearchPRs`,
`ListProjectItemsWithFields` to check cache before making HTTP calls.

```go
func (c *Client) SearchIssues(ctx context.Context, query string) ([]model.Issue, error) {
    key := cache.Key("search-issues", query)
    if cached, ok := c.cache.Get(key); ok {
        return unmarshalIssues(cached), nil
    }
    result, raw, err := c.searchIssuesRaw(ctx, query)
    if err != nil {
        return nil, err
    }
    c.cache.Set(key, raw)
    return result, nil
}
```

### Scope

Cache is per-process, in-memory only. No filesystem persistence.
10-minute TTL means a CI pipeline running multiple commands benefits,
but stale data is never a concern. Cache is opt-in via `Client` constructor.

---

## Approach 2: Reduce my-week Search Calls (7 → 5)

### Current: 7 search queries

| # | Query | Can Combine? |
|---|-------|-------------|
| 1 | Closed issues by author (lookback) | No — `is:issue` exclusive |
| 2 | Merged PRs by author (lookback) | No — `is:pr` exclusive |
| 3 | Reviewed PRs by author (lookback) | No — uses `reviewed-by:` |
| 4 | Open issues assigned to me | See below |
| 5 | Open PRs by me | See below |
| 6 | Open PRs needing review (`review:none`) | **Eliminate** — subset of #5 |
| 7 | PRs awaiting my review (`review-requested:`) | No — different qualifier |

### Optimization A: Eliminate query #6

Query #6 (open PRs needing review, `review:none`) is a strict subset of
query #5 (all open PRs by author). Fetch #5, then filter client-side
for PRs with zero reviews.

**Savings**: 1 search call.

**Requirement**: The search API response must include review state, or we
need a way to detect "no reviews" from the PR data. Currently the code uses
a separate `PRsNeedingReview` field that gets populated from query #6's
results. After the change, we'd filter from query #5's results where
the PR has no reviews.

Check: does the search API return review count? No — but we can match
by number. If a PR from query #5 also appears in query #6, it needs review.
Wait, that defeats the purpose. Better approach: use the `review:none`
qualifier info to flag PRs after fetching them.

Actually, the simplest approach: drop query #6 entirely. In ProcessData,
mark a PR as "needing review" if it's in `PRsOpen` and NOT in any review
state. The `PRsNeedingReview` field is only used for status annotation in
the lookahead section — we can derive it from the GitHub Reviews API or
simply by checking if the PR has any reviews via a lightweight REST call.

OR: use the GraphQL PR node which includes `reviewDecision` field. One
batched GraphQL call for all open PRs (≤20 typically) replaces the
search query.

**Recommended**: Keep it simple. Fetch all open PRs (query #5), then
for each, check `reviewDecision` via a single batched GraphQL query.
This trades 1 search call for 1 GraphQL call (batch), which is better
because GraphQL doesn't hit the search rate limit.

### Optimization B: Combine open issues + open PRs (queries #4 + #5)

GitHub search without `is:issue` or `is:pr` returns both. We could:

```
<scope> is:open author:<login> OR assignee:<login>
```

Problem: `author:` and `assignee:` are different qualifiers with OR
semantics that GitHub search doesn't support cleanly. The search API
doesn't support `OR` between qualifiers.

**Verdict**: Not feasible. Keep separate.

### Optimization C: Batch lookback queries with delays

Instead of firing queries 1-3 concurrently (which looks like abuse to
GitHub), add a small delay between them:

```go
g.SetLimit(3) // was 8
// Add 500ms delay between search API calls
```

This doesn't reduce call count but avoids secondary rate limits.

**Recommended**: Yes, as a safety measure.

### Summary

| Change | Saves | Complexity |
|--------|-------|-----------|
| Eliminate query #6, use GraphQL batch | 1 search call, +1 GraphQL | Low |
| Throttle concurrent searches | 0 calls, avoids abuse limit | Trivial |
| Cache layer for cross-command dedup | Variable (0-7 per command) | Medium |

---

## Approach 3: Shared Data Source for velocity + wip

Both velocity (board-based) and wip query the same project board.

- **velocity**: `ListProjectItemsWithFields(projectID, iterField, numField)`
- **wip**: `ListProjectItems(projectID)` filtered by status

With the cache layer, the second call is a cache hit if the fields
requested are the same. But the field sets differ (velocity wants
iteration + numeric fields; wip wants status).

**Better approach**: A unified `FetchProjectBoard` method that fetches
ALL items with ALL fields, then let each consumer filter what it needs.
This is one paginated GraphQL call regardless of how many commands consume
the data.

```go
// FetchProjectBoard fetches all items with all field values.
// Consumers filter by field name as needed.
func (c *Client) FetchProjectBoard(ctx context.Context, projectID string) ([]model.ProjectBoardItem, error) {
    // Single paginated GraphQL query returning all items + all fields
    // Cache result for TTL
}
```

This would require a richer `ProjectBoardItem` type that carries all
field values as a map, with typed accessors for iteration, number,
status, etc.

---

## Priority Order

1. **Throttle my-week concurrency** — trivial, prevents abuse limits now
2. **Fix velocity to only fetch needed iterations** — done (this PR)
3. **Reduce preflight default count to 3** — done (this PR)
4. **Query cache layer** — medium effort, high value for CI pipelines
5. **Eliminate my-week query #6** — low effort, saves 1 call
6. **Unified board fetch** — medium effort, benefits multi-command runs
