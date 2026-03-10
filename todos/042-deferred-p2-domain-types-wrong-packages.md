---
status: pending
priority: p2
issue_id: "042"
tags: [code-review, architecture, pattern]
dependencies: []
---

# Domain Types in Wrong Packages

## Problem Statement
Several domain types live in non-model packages, violating the project convention of pure domain types in `internal/model/`:
- `format.WIPItem` — domain type in format package
- `github.ProjectItem` — domain type in API client package
- `format.StatsResult`, `format.StatsThroughput`, `format.StatsQuality` — domain types in format package

## Findings
- Location: `internal/format/wip.go` (WIPItem)
- Location: `internal/github/projectitems.go` (ProjectItem)
- Location: `internal/format/stats.go` (StatsResult, StatsThroughput, StatsQuality)
- Agents: architecture-strategist, pattern-recognition-specialist
- Convention: `internal/model/` holds all pure domain structs

## Proposed Solutions

### Option 1: Move all domain types to internal/model/
- **Pros**: Consistent with convention, breaks import cycles cleanly
- **Cons**: Moderate refactor touching many files
- **Effort**: Medium
- **Risk**: Low (mechanical moves)

## Recommended Action
_To be filled during triage_

## Technical Details
- **Affected Files**: internal/model/, internal/format/wip.go, internal/format/stats.go, internal/github/projectitems.go, cmd/wip.go, cmd/stats.go
- **Related Components**: All consumers of these types
- **Database Changes**: No

## Acceptance Criteria
- [ ] WIPItem, ProjectItem, StatsResult types in internal/model/
- [ ] No import cycles
- [ ] All tests pass

## Work Log

### 2026-03-10 - Created from code review
**By:** Claude Code Review
**Actions:**
- Finding identified by architecture-strategist and pattern-recognition-specialist
