---
status: complete
priority: p2
issue_id: "032"
tags: [code-review, security]
dependencies: []
---

# Cap pagination in tags.go to prevent unbounded API calls

## Problem Statement

`ListTags` in `internal/github/tags.go` has no upper bound on pagination. A repository with thousands of tags would cause unbounded API calls, potentially exhausting rate limits or causing long hangs.

**Raised by:** Security Sentinel (MEDIUM)

## Findings

- `ListTags` at `internal/github/tags.go:22-42` loops with `page++` with no max
- `CompareCommits` at `internal/github/tags.go:63-91` has the same unbounded pagination
- No timeout or page cap prevents runaway API consumption
- Context cancellation would help but explicit caps are better defense-in-depth

## Proposed Solutions

### Option A: Add maxPages constant (Recommended)
- Add `const maxPages = 50` (5000 tags covers virtually all repos)
- Break loop when `page > maxPages`
- Add warning when cap is hit
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] ListTags and CompareCommits have page caps
- [ ] Warning emitted when cap is reached
- [ ] Tests verify pagination cap behavior

## Work Log

### 2026-03-09 - Created from code review
**By:** Review synthesis
**Actions:** Identified by Security Sentinel
