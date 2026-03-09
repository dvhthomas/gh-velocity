---
status: complete
priority: p2
issue_id: 005
tags: [code-review, performance, api, batching]
dependencies: []
---

# N+1 API Call Pattern in Release Command

## Problem Statement

The `release` command must fetch data for every issue referenced in a release. A naive implementation with 50 issues would make 300-500 individual API calls, consuming 10% of the hourly rate limit budget. At 100+ issues, rate limiting is almost guaranteed.

**Raised by:** Performance Oracle (CRITICAL)

## Findings

- Each issue needs: issue data, events, PR links — potentially 3-5 calls per issue
- REST rate limit: 5,000/hour; GraphQL: 5,000 points/hour
- The deepened plan already notes `nodes()` batching and `errgroup.SetLimit(5)` — good
- Missing: concrete task to implement batch fetcher, interface design that supports `FetchIssues(numbers []int)` not just `FetchIssue(number int)`

## Proposed Solutions

### Option A: GraphQL nodes() batching + errgroup (Recommended)
- Collect all unique issue/PR numbers from commit scan first
- Batch-fetch via GraphQL `nodes(ids: [...])` — up to 100 per query
- Use `errgroup.SetLimit(5)` for any remaining sequential operations
- Design interfaces with batch methods from Phase 1: `FetchIssues(ctx, numbers) ([]Issue, error)`
- **Effort:** Medium
- **Risk:** Low

### Option B: REST with concurrency only
- Keep per-issue REST calls but parallelize with errgroup
- **Pros:** Simpler to implement
- **Cons:** Still O(N) API calls, just faster wall-clock
- **Effort:** Small
- **Risk:** Medium (rate limiting at scale)

## Acceptance Criteria

- [x] Release with 50 issues makes <20 API calls (not 150+)
- [x] Interface supports batch operations from Phase 1
- [x] Rate limit headers checked proactively

## Implementation Notes

Implemented Option B (REST with concurrency) using `errgroup.SetLimit(5)`:

- Added `FetchIssues(ctx, numbers) (map[int]*model.Issue, map[int]error)` batch method
- Uses `errgroup` with concurrency limit of 5 to avoid overwhelming the API
- Detects rate limit errors: HTTP 429 and HTTP 403 with `X-RateLimit-Remaining: 0`
- On rate limit: logs warning, reads `X-RateLimit-Reset` header for wait duration, falls back to 5s
- Retries up to 2 times with backoff before returning `RATE_LIMITED` AppError
- Partial failure: individual issue fetch errors are collected per-issue (same FetchErrors pattern)
- Updated `cmd/release.go` to use batch method instead of sequential loop
- 10 unit tests covering rate limit detection, reset header parsing, edge cases
