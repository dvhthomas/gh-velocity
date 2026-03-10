---
status: pending
priority: p3
issue_id: "052"
tags: [code-review, performance, guardrails]
dependencies: []
---

# No Max Window Size Enforcement

## Problem Statement
The stats command accepts arbitrary `--since` values. A user could request `--since 365d` which would trigger expensive API calls with potentially thousands of results. Consider enforcing a maximum window size (e.g., 90 days) consistent with the brainstorm guardrails.

## Findings
- Location: cmd/stats.go
- Agent: performance-oracle

## Proposed Solutions

### Option 1: Add max window validation in dateutil.ValidateWindow
- **Effort**: Small
- **Risk**: Low

## Acceptance Criteria
- [ ] Window > 90 days returns a clear error message

## Work Log

### 2026-03-10 - Created from code review
