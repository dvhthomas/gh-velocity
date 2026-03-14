---
title: "API Consumption"
weight: 4
---

# API Consumption

How gh-velocity uses the GitHub API, rate limit budget, caching, and optimization tips.

## API types used

gh-velocity uses two GitHub API interfaces:

| API | Rate limit | Used for |
|-----|-----------|----------|
| **REST Search API** | 30 requests/minute, 1000 results/query | Finding issues, PRs by date range, scope, lifecycle |
| **GraphQL API** | 5,000 points/hour | Project board data, timeline events, closing references, iteration fields |

The Search API is the primary bottleneck. Each paginated search query (up to 10 pages of 100 results) counts as one request per page against the 30/minute limit. The GraphQL API is used for richer data that the Search API cannot provide.

## Per-command cost estimates

These are approximate upper bounds. Actual costs depend on the number of results returned and how much data is cached from prior calls in the same process.

### `gh velocity quality release <tag>`

| API call | Estimate | Notes |
|----------|----------|-------|
| Fetch release metadata | 1 REST | Tag lookup |
| Fetch previous release | 1 REST | List releases |
| Search merged PRs in window | 1-10 REST | pr-link strategy, paginated |
| Fetch closing issue refs per PR | 1 GraphQL per 100 PRs | Batched |
| Fetch issue details | 1 REST per issue | For issues found by commit-ref/changelog |
| Cycle time signals (issue strategy) | 1 GraphQL per issue | Label timeline or project status |

**Typical total**: 5-30 API calls for a release with 10-50 issues.

### `gh velocity flow lead-time <issue>`

| API call | Estimate | Notes |
|----------|----------|-------|
| Fetch issue | 1 REST | Issue details |

**Total**: 1 API call.

### `gh velocity flow cycle-time <issue>`

| API call | Estimate | Notes |
|----------|----------|-------|
| Fetch issue | 1 REST | Issue details |
| Label timeline (issue strategy) | 1 GraphQL | Label events for cycle start |
| Project status (fallback) | 1 GraphQL | Board status if no label match |
| Search closing PRs (PR strategy) | 1 REST | Find linked PRs |

**Total**: 2-3 API calls depending on strategy.

### `gh velocity flow velocity`

Cost depends heavily on the iteration strategy and count:

**Fixed iterations (search-based)**:

| API call | Estimate | Notes |
|----------|----------|-------|
| Search per iteration | 1-10 REST per iteration | One search query per iteration window |

With the default `count: 6` and current iteration, that is up to 7 search queries. Each query is throttled by `api_throttle_seconds`.

**Project-field iterations (board-based)**:

| API call | Estimate | Notes |
|----------|----------|-------|
| Resolve project | 1 GraphQL | Project URL to node ID |
| List iteration field | 1 GraphQL | Iteration boundaries |
| List project items | 1+ GraphQL | Paginated, 100 items/page |

Board-based velocity uses fewer API calls because all items are fetched in one paginated query instead of one search per iteration.

**Typical total**: 3-5 GraphQL calls (board-based) or 7-70 REST calls (search-based with 6 iterations).

### `gh velocity report`

| API call | Estimate | Notes |
|----------|----------|-------|
| Search closed issues | 1-10 REST | Paginated |
| Search merged PRs | 1-10 REST | Paginated |
| Search open issues | 1-10 REST | Paginated |
| Search open PRs | 1-10 REST | Paginated |
| Search review-required PRs | 1-10 REST | Paginated |
| List releases | 1 REST | In date window |
| Cycle time per closed issue | 1 GraphQL each | If strategy is configured |

**Typical total**: 5-50+ API calls depending on volume.

### `gh velocity status wip`

| API call | Estimate | Notes |
|----------|----------|-------|
| Resolve project | 1 GraphQL | Project URL to node ID |
| List project items | 1+ GraphQL | Paginated by board status |

**Total**: 2-5 GraphQL calls.

## Search API throttling

GitHub's Search API has two rate limit tiers:

1. **Primary**: 30 requests per minute. The tool detects HTTP 429 or 403 with `X-RateLimit-Remaining: 0`, waits for the reset time, and retries once.

2. **Secondary (abuse detection)**: Undocumented thresholds based on request patterns. Triggered by rapid bursts of search queries. Produces HTTP 403 with "secondary rate limit" in the body. Lockouts typically last 1-5 minutes.

To avoid secondary rate limits, gh-velocity throttles search API calls with a configurable delay:

```yaml
api_throttle_seconds: 2   # 2-second gap between search calls
```

The throttle serializes search calls through a mutex. Concurrent goroutines queue behind it, ensuring only one search request is in-flight at a time with spacing between them.

If a rate limit is hit despite throttling, the tool:
1. Waits for the reset period (primary) or 60 seconds (secondary)
2. Retries the failed request once
3. If the retry also fails, returns an actionable error with suggestions

### Rate limit error suggestions

When rate-limited, the tool provides specific guidance:

- Use `--current` to fetch only the current iteration (fewer API calls)
- Use a board-based strategy (`project.url` in config) which uses GraphQL instead of the Search API
- Reduce `--iterations` to lower the number of search queries
- In CI, space commands apart with 60+ second gaps between invocations

## GraphQL rate limit budget

The GraphQL API uses a point-based system: 5,000 points per hour. Each query costs approximately 1 point per node requested. Typical gh-velocity GraphQL operations cost 1-5 points each.

For a worst-case release with 100 issues using the issue cycle time strategy, the GraphQL cost is approximately:
- 1 point for project resolution
- 100 points for label timeline events (1 per issue)
- ~5 points for PR closing references

Total: ~106 points out of 5,000 -- well within budget for any reasonable usage.

## Caching

gh-velocity uses an in-process query cache to deduplicate identical API calls within a single invocation. The cache key is computed from the API call type and parameters.

Cached operations include:
- Search queries (identical query strings return cached results)
- Project item listings (same project ID and field names)
- Repository node ID resolution

The cache is **per-process** -- it does not persist between invocations. There is no disk cache. Running the same command twice makes the same API calls both times.

### Cache benefits

- The `report` command fetches closed issues once and reuses them for lead time computation and throughput counting
- Velocity with overlapping iteration windows deduplicates shared search queries
- Multiple commands in a pipeline (if invoked separately) do not share cache

## Search result cap

The GitHub Search API returns at most **1,000 results per query** (10 pages of 100 results). If a query matches more than 1,000 items, the tool warns:

```
results capped at 1000; narrow the date range or scope for complete data
```

Strategies to stay under the cap:
- Narrow the `--since` / `--until` date range
- Add scope filters (e.g., specific labels or milestones)
- For velocity, use shorter iteration lengths so each period has fewer items

## Optimization tips

1. **Use board-based velocity** when possible. `iteration.strategy: project-field` fetches all items in one paginated GraphQL query instead of one search per iteration.

2. **Set `api_throttle_seconds: 2`** to avoid secondary rate limit lockouts. The 2-second delay adds ~14 seconds for 7 iterations but prevents multi-minute lockouts.

3. **Use `--current`** during development to test with minimal API calls.

4. **Reduce `velocity.iteration.count`** if you only need recent history. Each iteration costs one search query (fixed strategy).

5. **Narrow scope** with `scope.query` to reduce result counts per search.

6. **In CI, space invocations**. If your workflow runs multiple gh-velocity commands, add 60+ second gaps between them to avoid secondary rate limits.

7. **Use `--config`** with pre-built example configs to avoid repeated `preflight` runs against the API.
