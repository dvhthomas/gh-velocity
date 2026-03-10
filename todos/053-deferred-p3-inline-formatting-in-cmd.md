---
status: pending
priority: p3
issue_id: "053"
tags: [code-review, architecture, pattern]
dependencies: []
---

# Inline Single-Item Formatting in cmd/

## Problem Statement
Some commands (e.g., single-issue lead-time, cycle-time) have inline formatting logic in `cmd/` files instead of delegating entirely to `internal/format/`. This is inconsistent with the pattern used by bulk and stats commands.

## Findings
- Agent: architecture-strategist

## Proposed Solutions

### Option 1: Move all formatting to internal/format/
- **Effort**: Medium
- **Risk**: Low

## Acceptance Criteria
- [ ] cmd/ files only call format functions, no inline fmt.Fprintf for output

## Work Log

### 2026-03-10 - Created from code review
