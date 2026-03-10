---
status: pending
priority: p3
issue_id: "048"
tags: [code-review, pattern, duplication]
dependencies: []
---

# formatStatsSummary vs statsSummaryShort Duplicate

## Problem Statement
Two similar stats summary formatting functions exist — one in the bulk lead-time/cycle-time formatters and `statsSummaryShort` in stats.go. Could be unified.

## Findings
- Location: internal/format/stats.go, internal/format/leadtime.go
- Agent: pattern-recognition-specialist

## Proposed Solutions

### Option 1: Unify into single `FormatStatsSummary()` function
- **Effort**: Small
- **Risk**: Low

## Acceptance Criteria
- [ ] Single stats summary formatter

## Work Log

### 2026-03-10 - Created from code review
