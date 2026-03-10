---
status: pending
priority: p3
issue_id: "046"
tags: [code-review, pattern, duplication]
dependencies: []
---

# Backlog/Done Status Defaults Duplicated

## Problem Statement
Default values for backlog ("Backlog") and done ("Done") status strings are duplicated across 4+ files instead of being defined as constants.

## Findings
- Location: cmd/stats.go, cmd/wip.go, and others
- Agent: pattern-recognition-specialist

## Proposed Solutions

### Option 1: Define constants in internal/config/ or internal/model/
- **Effort**: Small
- **Risk**: Low

## Acceptance Criteria
- [ ] Single source of truth for default status values
- [ ] All consumers reference the constants

## Work Log

### 2026-03-10 - Created from code review
