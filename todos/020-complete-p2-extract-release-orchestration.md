---
status: complete
priority: p2
issue_id: 020
tags: [code-review, architecture, testability]
dependencies: []
---

# Extract Release Orchestration from cmd/release.go RunE

## Problem Statement

The `release` command's `RunE` closure is ~170 lines mixing git ops, API calls, metric computation, warning accumulation, label classification, and output formatting. This "God function" is hard to test, extend, and reason about. The label classification logic is duplicated three times (ReleaseComposition, countByType, unlabeled-check).

**Raised by:** Architecture Strategist, Pattern Recognition Specialist, Code Simplicity Reviewer

## Findings

- `cmd/release.go:26-197` — 170-line RunE function
- `cmd/release.go:218-229` — `countByType` duplicates `metrics.ReleaseComposition`
- `cmd/release.go:142-154` — unlabeled-check re-walks issues (could use `otherCount`)
- `cmd/release.go:231-240` — `hasAnyLabel` duplicated from `metrics/quality.go`

## Proposed Solutions

### Option A: Extract to metrics.BuildReleaseMetrics() (Recommended)
- Move orchestration into `internal/metrics/release.go` accepting interface params
- Return `(model.ReleaseMetrics, []string, error)`
- Command handler becomes: parse args → call function → format output
- Unify label classification into single pass
- **Effort:** Medium
- **Risk:** Low

## Acceptance Criteria

- [ ] `cmd/release.go` RunE is < 50 lines
- [ ] `hasAnyLabel` exists in exactly one place
- [ ] Label classification done in single pass
- [ ] Low-label-coverage warning derived from `otherCount`
