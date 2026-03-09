---
status: complete
priority: p2
issue_id: 016
tags: [code-review, error-handling, resilience]
dependencies: []
---

# Define Partial Failure Strategy for Batch Operations

## Problem Statement

The `release` command fetches data for N issues. If one issue fetch fails (deleted issue, permission error, transient API failure), the default `errgroup` behavior cancels everything and returns the first error. This all-or-nothing failure is a poor experience for large releases.

**Raised by:** Spec Flow Analyzer (CRITICAL), Performance Oracle (mentioned)

## Findings

- `errgroup` cancels context on first error by default
- A release with 50 issues failing on 1 deleted issue loses all 49 good results
- Users would prefer partial results with warnings over total failure

## Proposed Solutions

### Option A: Skip-and-warn (Recommended)
- Catch per-issue errors, log warning to stderr, continue with remaining issues
- Include `"warnings": [...]` in JSON output listing skipped issues
- Final table shows only successful issues, footer notes "N issues skipped due to errors"
- **Effort:** Small
- **Risk:** Low

### Option B: Fail fast (current errgroup default)
- Simpler to implement
- **Risk:** High — poor UX on large releases

## Acceptance Criteria

- [ ] Release command succeeds with partial results when individual issues fail
- [ ] Warnings logged to stderr for each skipped issue
- [ ] JSON output includes `warnings` array
- [ ] Exit code 0 for partial success (with warnings)
