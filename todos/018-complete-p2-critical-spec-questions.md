---
status: complete
priority: p2
issue_id: 018
tags: [code-review, spec, decisions-needed]
dependencies: []
---

# Resolve Critical Spec Questions Before Implementation

## Problem Statement

The spec flow analysis identified several unresolved questions that affect core data model and command behavior. These need answers before Phase 1a begins.

**Raised by:** Spec Flow Analyzer

## Questions Requiring Answers

### Q1: Cycle time with zero commits
What happens when `cycle-time` is run on an issue with no referencing commits?
**Suggested default:** Output `N/A`, exit 0 with stderr note.

### Q2: First release with huge commit history
`release` with one tag diffs from initial commit. A 3-year repo = thousands of commits.
**Suggested default:** Warn if >500 commits; document `--since <sha>` to limit scope.

### Q3: Tag format acceptance
Does `release v1.0.0` also match tag `1.0.0` (without `v` prefix)? Do we try both?
**Suggested default:** Exact match only; error if not found with hint about the actual tag.

### Q4: `auto` workflow detection algorithm
How many merged PRs constitute "pr mode"? Cached or re-detected each run?
**Note:** Auto-detect was removed; workflow mode is default `pr` with manual override to `local`.

### Q5: Idempotent marker for range-based commands
`throughput` and `project` operate on date ranges, not entities. What's the marker?
**Suggested default:** `<!-- gh-velocity:throughput:{from}:{to}:{group-by} -->`

### Q6: Discussion creation vs. update for release --post
Does `release --post` create a new discussion per release or update an existing one?
**Suggested default:** Create new discussion per release (one discussion = one release).

### Q7: Low label coverage warning
When >50% of issues in a release have no bug/feature labels, should we warn?
**Suggested default:** Yes, warn on stderr.

## Acceptance Criteria

- [ ] All questions answered and documented in the plan
- [ ] Answers reflected in acceptance criteria
