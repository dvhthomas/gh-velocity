---
title: "feat: Disk cache and N+1 project status fix"
type: feat
status: completed
date: 2026-03-15
origin: docs/brainstorms/2026-03-15-showcase-improvements-roadmap-brainstorm.md
---

# feat: Disk Cache and N+1 Project Status Fix

## Overview

Add a short-TTL disk cache to avoid redundant GitHub API calls across `gh-velocity` invocations, and batch the per-issue `GetProjectStatus` GraphQL calls that currently create an N+1 problem. Together these changes make the daily showcase workflow viable at scale and improve interactive CLI usage.

(see brainstorm: `docs/brainstorms/2026-03-15-showcase-improvements-roadmap-brainstorm.md` — Phase 1)

## Problem Statement

**Disk cache:** Each `gh-velocity` command starts cold. Running `report` then `lead-time` for the same repo/window re-fetches identical search results. The showcase workflow runs 5+ commands per repo across 9 repos — roughly 5x the necessary API calls. A 2-repo test took 25 minutes due to secondary rate limit backoffs.

**N+1:** `GetProjectStatus` makes one GraphQL call per issue. For repos using the `issue` cycle time strategy with a project board, 50 closed issues = 50 sequential GraphQL calls. This is uncached and occurs in three call sites: `BulkPipeline.ProcessData()`, `BuildReleaseMetrics()`, and `computeMyWeekCycleTime()`.

## Proposed Solution

### Part A: Disk Cache

A filesystem-based cache at `~/.cache/gh-velocity/v1/` (via `os.UserCacheDir()`) that layers below the existing in-memory `QueryCache`. Results from GitHub API calls are serialized to JSON files keyed by the same SHA256 hash the in-memory cache uses.

**Read path:** in-memory → disk → API call → write to both
**Write path:** API result → write to memory cache + write to disk (atomic)
**`--no-cache`:** bypasses disk only. In-memory singleflight cache always active (it prevents redundant API calls within a single `report` invocation).

### Part B: Batch GetProjectStatus

Replace per-issue `GetProjectStatus` calls with a batch pre-fetch using GraphQL aliases (same pattern as `FetchIssues` and `FetchPRLinkedIssues`). Batch size of 20 issues per query. Uses "cache-warm" approach: the batch populates the in-memory/disk cache, then individual `GetProjectStatus` calls hit cache. No strategy interface changes needed.

## Technical Approach

### Disk Cache Implementation

**New file:** `internal/github/diskcache.go`

```go
// DiskCache provides a filesystem-backed cache with short TTL.
// It is an internal optimization — the format is opaque and may
// change between versions without notice.
type DiskCache struct {
    dir string        // e.g. ~/.cache/gh-velocity/v1/
    ttl time.Duration // 5 minutes
}

// Get reads a cached value from disk. Returns nil, false if not found or expired.
func (dc *DiskCache) Get(key string) (json.RawMessage, bool)

// Set writes a value to disk atomically (write-to-temp, then rename).
func (dc *DiskCache) Set(key string, data json.RawMessage) error
```

**Key design decisions:**
- Stores `json.RawMessage` (raw bytes), not typed Go objects. The caller provides the deserialization type.
- Atomic writes: `os.CreateTemp` in the cache dir, write, then `os.Rename`. Safe for concurrent processes ("last writer wins").
- Lazy cleanup: expired entries deleted on read. No background goroutine or startup scan needed — 5-minute TTL means stale files are tiny and short-lived.
- Cache key = existing `CacheKey()` hash (already filesystem-safe 16-char hex string).
- Token-aware keys for project queries: include whether `GH_VELOCITY_TOKEN` is set in the key to prevent cross-token cache poisoning.

**Integration with QueryCache:**

Modify `QueryCache.Do()` to check disk before executing the function:

```
Do(key, fn):
  1. Check in-memory map → return if hit
  2. Singleflight gate (coalesce concurrent callers)
  3. Check disk cache → deserialize, store in memory, return if hit
  4. Execute fn() → store in memory + write to disk
```

The `QueryCache` gains a `DiskCache` field (nil when `--no-cache`). The `Do` method signature stays the same (`func() (any, error)`) but internally handles disk serialization via a type-tagged wrapper:

```go
type diskEntry struct {
    Type string          `json:"type"` // "search-issues", "search-prs", etc.
    Data json.RawMessage `json:"data"`
}
```

The cache key prefix (already the first arg to `CacheKey`) serves as the type discriminator. Callers of `cache.Do()` already pass a prefix like `"search-issues"` — this prefix determines how to deserialize `Data`.

### N+1 Batch Fix

**New function:** `internal/github/cyclestart.go` — `BatchGetProjectStatuses`

```go
// BatchGetProjectStatuses fetches project status for multiple issues in a single
// GraphQL query using aliases. Batch size: 20 issues per query.
// Results are written to the cache for subsequent GetProjectStatus calls.
func (c *Client) BatchGetProjectStatuses(ctx context.Context, numbers []int,
    projectID, statusFieldID string, backlogStatuses []string) (map[int]*ProjectStatusResult, error)
```

Pattern matches `FetchIssues` (batch.go) and `FetchPRLinkedIssues` (pullrequests.go):
- Chunk `numbers` into groups of 20
- Build aliased query: `issue42: issue(number: 42) { projectItems(first: 20) { ... } }`
- Parse response via `map[string]json.RawMessage`
- Extract and process `fieldValues` per issue (reuse existing `matchProjectStatus` logic)
- Write individual results to cache so `GetProjectStatus` calls hit cache
- Uses `c.projectClient()` (not `c.gql`) — project board queries require `GH_VELOCITY_TOKEN`

**Cache-warm integration points:**

Each call site that loops over issues calling `strategy.Compute()` adds a one-line pre-fetch before the loop:

| Call site | File | Line | Change |
|-----------|------|------|--------|
| `BulkPipeline.GatherData()` | `internal/pipeline/cycletime/cycletime.go` | ~line 80 | Add `client.BatchGetProjectStatuses(ctx, issueNumbers, ...)` |
| `BuildReleaseMetrics()` | `internal/metrics/release.go` | ~line 77 | Add batch pre-fetch before loop |
| `computeMyWeekCycleTime()` | `cmd/myweek.go` | ~line 284 | Add batch pre-fetch before loop |

After the batch pre-fetch, individual `GetProjectStatus` calls within `IssueStrategy.computeFromProject()` hit the cache. No changes to the `CycleTimeStrategy` interface or `IssueStrategy` struct.

### --no-cache Flag

**File:** `cmd/root.go`

- New persistent flag: `--no-cache` (bool, default false)
- Flows through `Deps` to `NewClient()`
- When set, `NewClient()` creates `QueryCache` with `DiskCache: nil`
- In-memory cache remains active regardless

### Debug Output

Three-level cache logging:
- `[debug] cache hit (memory): search-issues key=abc123`
- `[debug] cache hit (disk): search-issues key=abc123`
- `[debug] cache miss: search-issues key=abc123 (N results)`

## Implementation Phases

### Phase 1: Disk Cache Infrastructure

- [x] Create `internal/github/diskcache.go` with `DiskCache` struct, `Get`, `Set`, `cleanup` methods
- [x] Atomic writes via `os.CreateTemp` + `os.Rename`
- [x] Lazy cleanup: delete expired entries on read
- [x] Cache dir: `os.UserCacheDir() + "/gh-velocity/v1/"`
- [x] Unit tests: write, read, expiry, concurrent access, corrupt file handling

### Phase 2: Integrate Disk Cache with QueryCache

- [x] Add `DiskCache *DiskCache` field to `QueryCache`
- [x] Add `DoJSON` method for disk-backed caching (Do remains memory-only for backward compat)
- [x] Serialize via `diskEntry{Type, Data}` JSON wrapper
- [x] Deserialize using type prefix to determine Go type
- [x] Update `NewClient()` to create `DiskCache` (nil when `--no-cache`)
- [x] Add `--no-cache` persistent flag in `cmd/root.go`, flow through `Deps`
- [x] Update debug logging: memory hit, disk hit, miss
- [x] Token-aware cache keys for project queries (include `hasProjectToken` in key)
- [x] Unit tests: layered cache behavior, --no-cache bypass, type-safe roundtrip

### Phase 3: Batch GetProjectStatus (N+1 Fix)

- [x] Create `BatchGetProjectStatuses()` in `internal/github/cyclestart.go`
- [x] GraphQL alias pattern matching `FetchIssues` (batch size 20)
- [x] Use `c.projectClient()` for the query
- [x] Factor out `matchProjectStatus` logic from `GetProjectStatus` for reuse
- [x] Write individual results to cache for subsequent `GetProjectStatus` hits
- [x] Add cache-warm calls in BulkPipeline.GatherData() and cycle-time command
- [x] Cache-warm in `BuildReleaseMetrics()` and `computeMyWeekCycleTime()`
- [x] Unit tests: matchProjectStatus, cache key isolation, empty batches, cache warming
- [x] Integration test: batch cache-warm → GetProjectStatus hits cache (no API call)

### Phase 4: Validation

- [x] Run `task test` — all existing tests pass
- [x] Run `task quality` — lint + staticcheck clean
- [x] Verify `--no-cache` still works correctly (cache parity test #6)
- [x] Verify cache files appear on disk (cache parity test #7)
- [ ] Run showcase with 2 repos, compare API call count (debug output) before vs after

## System-Wide Impact

- **No changes to command output.** Cache is transparent to users — same results, fewer API calls.
- **No changes to config format.** Only new flag: `--no-cache`.
- **Existing tests unaffected.** Tests don't use disk cache (TestClient creates QueryCache without DiskCache).
- **CI impact:** Cache files written to ephemeral CI filesystem, cleaned up naturally. No persistence concerns.
- **The showcase workflow** benefits immediately — second command for the same repo hits disk cache for search results.

## Acceptance Criteria

- [ ] `gh velocity report --since 30d` followed by `gh velocity flow lead-time --since 30d` for the same repo reuses cached search results (visible in `--debug` output as `cache hit (disk)`)
- [ ] `GetProjectStatus` uses batched queries (visible in `--debug` as single GraphQL call for N issues instead of N calls)
- [ ] `--no-cache` bypasses disk cache but keeps in-memory singleflight active
- [ ] Cache files are atomically written (no corruption on concurrent access)
- [ ] Expired cache entries are cleaned up on read
- [ ] All existing tests pass unchanged
- [ ] No changes to command output format or behavior

## Dependencies & Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Disk cache adds latency (file I/O) | Low | Negligible — JSON read/write is microseconds vs. seconds for API calls | Benchmark if concerned |
| Cache corruption from partial write | Low | Stale/missing cache entry, falls through to API | Atomic writes via rename |
| Cross-token cache poisoning | Medium | Wrong project status for different token scopes | Token-aware cache keys |
| GraphQL complexity limit for batched project queries | Low | Batch fails, falls back to individual calls | Batch size 20 (conservative) |
| `BacklogStatus` slice inconsistency | Low | Existing bug, not introduced by this change | Note but don't fix in this PR |

## Success Metrics

- Showcase workflow API calls reduced by ~60-70% (5 commands per repo sharing cached search results)
- Showcase wall-clock time for 2 repos drops from ~25 minutes to ~10 minutes
- N+1 for 50-issue cycle time: 50 GraphQL calls → 3 batched calls

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-15-showcase-improvements-roadmap-brainstorm.md](docs/brainstorms/2026-03-15-showcase-improvements-roadmap-brainstorm.md)
  - Key decisions: disk cache at ~/.cache/gh-velocity/v1/, 5-min TTL, --no-cache flag, internal-only format, N+1 bundled with cache

### Internal References

- Existing in-memory cache: `internal/github/cache.go` — `QueryCache`, `CacheKey`, singleflight
- Existing batch pattern: `internal/github/batch.go` — `FetchIssues` with GraphQL aliases
- Existing batch pattern: `internal/github/pullrequests.go:79-179` — `FetchPRLinkedIssues`
- N+1 source: `internal/github/cyclestart.go:147` — `GetProjectStatus` (one call per issue)
- N+1 call site: `internal/metrics/cycletime.go:71` — `computeFromProject()`
- N+1 call site: `internal/metrics/release.go:77` — `BuildReleaseMetrics` loop
- N+1 call site: `cmd/myweek.go:284` — `computeMyWeekCycleTime` loop
- Client construction: `cmd/root.go:66-71` — `Deps.NewClient()`
- Prior API optimization: `docs/plans/2026-03-13-001-feat-reduce-github-api-consumption-plan.md` (completed)

### Key Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `internal/github/diskcache.go` | **Create** | DiskCache struct, Get/Set/cleanup |
| `internal/github/diskcache_test.go` | **Create** | Unit tests for disk cache |
| `internal/github/cache.go` | **Modify** | Add DiskCache field, modify Do() for layered lookup |
| `internal/github/cyclestart.go` | **Modify** | Add BatchGetProjectStatuses, add cache to GetProjectStatus |
| `internal/github/client.go` | **Modify** | Wire DiskCache into NewClient |
| `cmd/root.go` | **Modify** | Add --no-cache flag, flow through Deps |
| `internal/pipeline/cycletime/cycletime.go` | **Modify** | Add batch pre-fetch in GatherData |
| `internal/metrics/release.go` | **Modify** | Add batch pre-fetch before loop |
| `cmd/myweek.go` | **Modify** | Add batch pre-fetch before loop |
