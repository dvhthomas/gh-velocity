---
title: "feat: reduce GitHub API consumption"
type: feat
status: completed
date: 2026-03-13
origin: docs/brainstorms/2026-03-13-api-cache-and-consolidation.md
---

# feat: reduce GitHub API consumption

## Overview

gh-velocity triggers GitHub's secondary (abuse) rate limits because commands make too many sequential/concurrent search API calls. The velocity command with default config makes 7 search calls; my-week fires 7 concurrent search queries; the report command executes the **same** closed-issue search 3 times in parallel. This plan reduces API consumption through a query-level cache, smarter defaults, and query consolidation.

## Problem Statement / Motivation

Users running `gh velocity flow velocity` or `gh velocity my-week` in CI or locally regularly hit GitHub's secondary rate limits (HTTP 403), which have undocumented triggers and multi-minute lockouts. The primary rate limits (30 search/min, 5000 REST/hr, 5000 GraphQL pts/hr) are not the issue — the secondary "abuse detection" limits are.

Root causes:
1. Commands fire many search API calls in rapid succession
2. The `report` command duplicates the same search query 3x across pipelines
3. No deduplication exists for identical queries within a single invocation
4. Default iteration count (6) means 7 search windows fetched even when only 1 is needed
5. `gatherFromSearch` fetched ALL iterations upfront regardless of `--current` flag

## Proposed Solution

Three layers of improvement, ordered by priority:

### Phase 1: Quick Wins (already done in current PR)

- [x] Fix `gatherFromSearch` to only fetch needed iterations (`--current` = 1 call, not 7)
- [x] Reduce preflight default `count` from 6 to 3
- [x] Throttle `my-week` concurrency 8→3 (search queries run in 3 waves, not all at once)
- [x] Throttle `preflight` concurrency 5→3

### Phase 2: Query Result Cache

Add an in-memory, per-process cache to `internal/github/` that deduplicates identical API calls within a single CLI invocation. The primary beneficiary is the `report` command, where 3 concurrent pipelines issue the identical closed-issue search query.

> **Scope correction** (from SpecFlow analysis): The cache is per-process and provides **zero cross-process benefit**. Each `gh velocity` invocation is a separate process. The brainstorm's claim that "CI pipelines running multiple commands sequentially" benefit is incorrect. The cache only deduplicates within a single command invocation — primarily `report`.

- [x] Implement `QueryCache` in `internal/github/cache.go` with `sync.RWMutex` for thread safety
- [x] Add `cache *QueryCache` field to `Client` struct, created in `NewClient`
- [x] Wrap `SearchIssues` to cache by query string
- [x] Wrap `SearchPRs` to cache by query string
- [x] Wrap `ListProjectItemsWithFields` to cache by (projectID, fields)
- [x] Add `--debug` logging for cache hits/misses
- [x] Unit test cache TTL expiry, hit/miss, concurrent access (race detector)

**Key design decisions** (see brainstorm, with SpecFlow corrections):
- Cache key = `sha256(method + query_string)[:16]` — method prefix (`search-issues` vs `search-prs`) disambiguates issue/PR searches
- **Store typed Go objects (`any`), not `[]byte`** — the current `searchPaginated` accumulates results page-by-page and returns typed slices. Serializing back to JSON for caching is wasteful. Since the cache is per-process with no serialization needs, store `[]model.Issue` / `[]model.PR` directly.
- TTL is effectively irrelevant — CLI processes complete in seconds. The TTL exists as a safety net, not a functional requirement.
- In-memory only, no filesystem persistence
- Cache is created in `NewClient` — no configuration needed
- Errors are NOT cached — only successful responses
- The retry path in `searchPaginated` must also populate the cache after a successful retry

**Thread safety is critical**: The `report` command runs 3 pipelines concurrently via `errgroup`, all sharing one `*Client`. Without `sync.RWMutex`, concurrent map access will panic. Consider `golang.org/x/sync/singleflight` for the `report` case (3 goroutines hitting the same key simultaneously — singleflight coalesces them into one actual API call).

**Biggest win**: The `report` command calls `SearchIssues(ctx, issueQuery.Build())` **3 times with the identical query string** (leadtime, cycletime, throughput pipelines all search for the same closed issues). With the cache (or singleflight), calls 2 and 3 are instant cache hits, saving 2 search API calls per report run.

### Phase 3: Eliminate my-week query #6 (Deferred — semantic mismatch)

> **SpecFlow finding**: The GraphQL `reviewDecision` field has **different semantics** than the search qualifier `review:none`. `review:none` means "zero reviews submitted." `reviewDecision` means "review policy not satisfied" — repos without branch protection return `null` for all PRs, and a PR with mixed reviews (1 approve + 1 changes-requested) would show `CHANGES_REQUESTED` via GraphQL but would NOT match `review:none` (it has reviews).
>
> This is a **behavioral change**, not just an optimization. Replacing the search with GraphQL would change which PRs appear in the "needs review" section. The savings (1 search call) do not justify a user-visible regression.

**Decision**: Keep query #6 as-is. The throttling (concurrency 8→3) already mitigates the secondary rate limit risk for my-week. Revisit if we find a GraphQL field that matches `review:none` semantics exactly (e.g., checking `reviews(first: 1) { totalCount }` for zero).

### Phase 4: GraphQL Rate Limit Handling (Future)

Currently only `searchPaginated` has rate limit detection/retry. GraphQL calls propagate errors directly. As we shift more work to GraphQL (Phase 3, board queries), we should add:

- [ ] Detect GraphQL rate limit errors (check `errors[].type == "RATE_LIMITED"`)
- [ ] Add retry-once with backoff (matching the search pattern in `ratelimit.go`)
- [ ] Apply to all GraphQL methods via a wrapper

This is lower priority since GraphQL has a generous 5000 pts/hr budget, but important for robustness.

## Technical Considerations

### Cache Concurrency Safety

The `report` command runs 3 pipelines concurrently via `errgroup`. All share one `*Client`. The cache must handle concurrent reads/writes safely.

Options:
1. `sync.Map` — lock-free for read-heavy workloads (our case: few writes, many reads)
2. `sync.RWMutex` around a regular map — simpler, well-understood
3. `singleflight` — coalesce concurrent identical requests into one flight

**Recommended**: Use `golang.org/x/sync/singleflight` for the cache lookup. This is the exact use case singleflight was designed for: 3 goroutines in `report` call `SearchIssues` with the identical query simultaneously. Singleflight coalesces them into a single API call — the first goroutine executes, the other two block and receive the same result. This is simpler than RWMutex-around-a-map because it eliminates the thundering herd problem entirely.

```go
type QueryCache struct {
    mu      sync.RWMutex
    entries map[string]cacheEntry
    flight  singleflight.Group
}
```

The flow: `Get()` checks the map (under RLock). If miss, `flight.Do(key, func)` ensures only one goroutine fetches. After fetch, `Set()` stores under write lock. Subsequent callers hit the map.

### Cache Key Design

The query string from `scope.Query.Build()` is deterministic — same inputs always produce the same string. This makes it a reliable cache key. The method prefix (`search-issues` vs `search-prs`) disambiguates between issue and PR searches that might share a query structure.

For GraphQL queries (project board items), the cache key should include the project ID and field names.

### What NOT to Cache

- **Mutations** (posting comments, creating discussions) — never cache writes
- **Error responses** — a transient failure shouldn't prevent retry
- **Authenticated user info** (`GetAuthenticatedUser`) — called once, not worth caching

### Testing the Cache

The cache is a pure data structure — test it in isolation:
- Set/Get with TTL expiry
- Concurrent Set/Get (race detector)
- Cache miss returns (nil, false)
- Different keys don't collide

Integration testing: verify that `report` makes 1 search call instead of 3. This requires either:
- A mock HTTP server counting requests
- Debug logging that counts cache hits

## Acceptance Criteria

- [x] `report` command makes 1 search call for closed issues instead of 3 (verified via `--debug` output)
- [x] `velocity --current` makes exactly 1 search API call (Phase 1, done)
- [x] Cache TTL expires correctly — stale entries are not served
- [x] Concurrent cache access doesn't panic (`go test -race` clean)
- [x] `singleflight` coalesces concurrent identical queries (test with report command)
- [x] All existing tests pass
- [x] `docs/api-consumption.cm` model updated with post-cache estimates

## Success Metrics

- Zero secondary rate limit errors in normal CI usage (velocity + report + my-week in sequence)
- `report` command latency reduced ~30% (2 fewer API round trips)
- No user-visible behavior changes (cache is transparent)

## API Budget Summary (from docs/api-consumption.cm)

| Command | Before (default 6 iters) | After Phase 1 | After Phase 2 (cache) |
|---------|--------------------------|---------------|----------------------|
| `velocity --current` | 7 search | 1 search | 1 search |
| `velocity` (default) | 7 search | 4 search (3 iters) | 4 search |
| `report` | 4 search | 4 search | **2 search** |
| `my-week` | 7 search (8 concurrent) | 7 search (3 concurrent) | 7 search (3 concurrent) |
| `throughput` | 2 search | 2 search | 2 search |
| **Total (CI sequence)** | **27 search** | **18 search** | **16 search** |

Reduction: 27 → 16 search calls (41% fewer). The concurrency throttle (8→3) is the most important change for secondary rate limits — it spaces calls out so GitHub doesn't see a burst.

## Dependencies & Risks

- **`sync.RWMutex` + `singleflight` in Client**: The `Client` struct is currently nearly stateless (only `repoNodeID`). Adding mutable cache state changes its threading model. Risk is low since all commands already share a single client within their goroutines. `singleflight` is already a dependency via `golang.org/x/sync`.
- **Cache invalidation is a non-issue**: CLI processes complete in seconds. The TTL is a safety net, not a functional requirement. Metrics are inherently backward-looking.
- **Unified board fetch deferred**: The brainstorm's Approach 3 (shared board data for velocity + wip) only helps if both run in the same process, which they don't. Deferred until a composite command exists.
- **Phase 3 deferred**: `reviewDecision` has different semantics than `review:none` (SpecFlow finding). Keeping query #6 avoids a behavioral regression.

## Sources & References

- **Origin brainstorm:** [docs/brainstorms/2026-03-13-api-cache-and-consolidation.md](docs/brainstorms/2026-03-13-api-cache-and-consolidation.md) — cache design, my-week query analysis, priority order
- **API consumption model:** [docs/api-consumption.cm](docs/api-consumption.cm) — parametric estimates per command
- Duplicate search in report: `cmd/report.go:118,133,141` — all call `issueQuery.Build()` identically
- Rate limit handling: `internal/github/ratelimit.go` — current retry logic (search only)
- Search pagination: `internal/github/search.go:75` — single retry on rate limit
- Client struct: `internal/github/client.go:30` — where cache field goes
