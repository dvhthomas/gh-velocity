---
status: complete
priority: p3
issue_id: 026
tags: [code-review, simplicity, yagni]
dependencies: []
---

# Remove ~150 LOC of Unused Code

## Problem Statement

Multiple types, interfaces, methods, and fields are defined but never used. This is speculative code built for future features that don't exist yet.

**Raised by:** Code Simplicity Reviewer

## Findings — Code to Remove

- `internal/github/client.go:14-29` — 3 unused interfaces (IssueQuerier, ReleaseQuerier, CommitQuerier)
- `internal/github/client.go:34,46-48` — unused GraphQL client field + initialization
- `internal/github/client.go:58-61` — unused `Owner()`/`Repo()` accessors
- `internal/github/commits.go` — `CompareCommits` never called (release uses local git)
- `internal/github/issues.go:59-81` — `ListIssueEvents` never called
- `internal/github/releases.go:22-43` — `ListReleases` never called
- `internal/git/git.go:49-55` — `CommitsSince` never called
- `internal/model/types.go:19-31` — `PullRequest` type never used
- `internal/model/types.go:53-58` — `Event` type never used
- `cmd/root.go:19` — dead `formatKey` constant
- Various unused fields on model types (URL, IsDraft, IsPrerelease, etc.)

## Proposed Solutions

### Option A: Delete all unused code now (Recommended)
- Remove everything listed above
- Re-add when actually needed (interfaces when Deps takes them, GraphQL when queries exist)
- **Effort:** Small
- **Risk:** None

## Acceptance Criteria

- [ ] No unused interfaces, methods, or types
- [ ] `task build` and `task test` still pass
