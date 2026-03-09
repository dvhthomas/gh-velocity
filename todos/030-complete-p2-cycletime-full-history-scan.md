---
status: complete
priority: p2
issue_id: "030"
tags: [code-review, performance]
dependencies: []
---

# Cycle-time scans full git history instead of using targeted git log

## Problem Statement

The `cycle-time` command calls `source.AllCommits(ctx, "HEAD")` which scans the entire git history, then filters client-side for commits referencing the target issue. For large repos, this is O(all commits) when it could be O(matching commits) using `git log --grep`.

**Raised by:** Performance Oracle (CRITICAL)

## Findings

- `cmd/cycletime.go:59` calls `AllCommits(ctx, "HEAD")` to get every commit
- `linking.LinkCommitsToIssues` then scans all commits for issue references
- Only commits matching the target issue number are used
- For repos with thousands of commits, this is unnecessarily slow
- `git log --grep="#N"` would return only matching commits directly

## Proposed Solutions

### Option A: Add CommitsForIssue method to git.Runner (Recommended)
- Add `CommitsForIssue(ctx, issueNumber, ref)` that uses `git log --grep="#N"`
- Use this in cycle-time command instead of AllCommits + filter
- Keep AllCommits for release command where full scan is needed
- **Effort:** Small
- **Risk:** Low — `git log --grep` is well-established

### Option B: Add --grep flag passthrough to streamCommits
- More flexible but more complex
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] cycle-time command uses targeted git log instead of full history scan
- [ ] Performance improvement measurable on repos with >1000 commits
- [ ] AllCommits still available for release command
- [ ] Tests verify filtered commit retrieval

## Work Log

### 2026-03-09 - Created from code review
**By:** Review synthesis
**Actions:** Identified by Performance Oracle as critical performance issue
