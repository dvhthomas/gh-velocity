---
status: complete
priority: p2
issue_id: "031"
tags: [code-review, architecture]
dependencies: []
---

# Extract business logic from cmd/ to service/metrics layer

## Problem Statement

`gatherReleaseData()` and `findPreviousTag()` in `cmd/release.go` contain business logic (data orchestration, tag resolution) that belongs in the metrics or a service layer. This makes the logic untestable without Cobra and violates separation of concerns.

**Raised by:** Architecture Strategist (HIGH)

## Findings

- `gatherReleaseData()` at `cmd/release.go:87-144` orchestrates data fetching, commit linking, and issue resolution
- `findPreviousTag()` at `cmd/release.go:146-156` implements tag ordering logic
- Both are unexported and tightly coupled to the command layer
- Testing these requires wiring up Cobra commands instead of calling functions directly

## Proposed Solutions

### Option A: Move to internal/metrics package (Recommended)
- Move `gatherReleaseData` to `internal/metrics/gather.go` as an exported function
- Move `findPreviousTag` to `internal/metrics/tags.go`
- cmd/release.go becomes a thin adapter: parse args → call service → format output
- **Effort:** Medium
- **Risk:** Low — pure refactor, no behavior change

### Option B: Create internal/service package
- New package for orchestration logic
- More packages to maintain
- **Effort:** Medium
- **Risk:** Low

## Acceptance Criteria

- [ ] `gatherReleaseData` and `findPreviousTag` moved out of cmd/
- [ ] cmd/release.go RunE is thin: parse → service call → format
- [ ] Moved functions have direct unit tests (no Cobra needed)
- [ ] All existing tests still pass

## Work Log

### 2026-03-09 - Created from code review
**By:** Review synthesis
**Actions:** Identified by Architecture Strategist
