---
title: "Disk cache for cross-invocation reuse and N+1 GetProjectStatus batch fix"
category: performance-issues
date: 2026-03-17
tags: [caching, graphql, n-plus-one, disk-cache, batch, api-optimization]
pr: "#69"
---

# Disk Cache and N+1 GetProjectStatus Batch Fix

## Problem

Running `gh-velocity` commands sequentially (e.g., `report` then `lead-time` for the same repo/window) re-fetched identical GitHub API results every time. The showcase workflow running 5+ commands across 9 repos made ~5x the necessary API calls, taking 25+ minutes for 2 repos due to secondary rate limit backoffs.

Separately, `GetProjectStatus` made one GraphQL call per issue (N+1). For 50 closed issues with project board cycle time strategy, this meant 50 sequential GraphQL calls across three call sites.

## Root Cause

1. **No cross-invocation cache**: The existing `QueryCache` was in-memory only, scoped to a single CLI process. Each invocation started cold.
2. **Per-issue GraphQL**: `GetProjectStatus` in `cyclestart.go` made individual queries. Three call sites (`BulkPipeline.GatherData`, `BuildReleaseMetrics`, `computeMyWeekCycleTime`) each looped over issues calling this.

## Solution

### Disk Cache (`internal/github/diskcache.go`)

Filesystem-backed cache at `os.UserCacheDir()/gh-velocity/v1/` with 5-minute TTL. Layers below the in-memory `QueryCache`.

```go
// Read path: in-memory → disk → API call → write to both
// DoJSON handles disk serialization via json.RawMessage
func (c *QueryCache) DoJSON(key, typeName string, fn func() (any, error),
    unmarshal func(json.RawMessage) (any, error)) (any, error)
```

Key design decisions:
- **Atomic writes**: `os.CreateTemp` + `os.Rename` prevents corruption
- **Lazy expiry**: expired entries deleted on read, no background goroutine
- **Token-aware keys**: `hasProjectToken()` flag in cache key prevents cross-token poisoning
- **`--no-cache` flag**: bypasses disk only; in-memory singleflight always active

### Batch GetProjectStatus (`internal/github/cyclestart.go`)

```go
// BatchGetProjectStatuses fetches project status for multiple issues via
// GraphQL aliases (batch size 20). Results warm the cache for subsequent
// individual GetProjectStatus calls.
func (c *Client) BatchGetProjectStatuses(ctx context.Context,
    numbers []int, projectID, statusFieldID, backlogStatus string)
```

Factored out `matchProjectStatus` for reuse by both single and batch paths. Cache-warm calls added to all three N+1 sites via type-assertion on `*metrics.IssueStrategy`:

```go
if is, ok := strat.(*metrics.IssueStrategy); ok && is.ProjectID != "" {
    is.Client.BatchGetProjectStatuses(ctx, numbers, is.ProjectID, is.StatusFieldID, backlog)
}
```

## Prevention

- **Cache key paranoia**: every parameter that affects API results MUST be in the cache key. A wrong cache hit is worse than a miss. Test with `--no-cache` baseline and diff outputs.
- **Use `GH_VELOCITY_NOW` for deterministic cache parity tests**: clock drift between runs produces different query strings, breaking diff-based validation.
- **Rate limit flakiness**: cache parity tests must skip when the baseline is tainted by rate limits, not report false failures.
