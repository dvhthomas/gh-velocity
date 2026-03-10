---
status: pending
priority: p3
issue_id: "054"
tags: [code-review, naming, simplicity]
dependencies: []
---

# Rename `parts` to `query` in SearchOpenIssuesWithLabels

## Problem Statement
The variable `parts` in `SearchOpenIssuesWithLabels` is misleading — it's actually building a complete query string, not holding separate parts. Should be renamed to `query` for clarity.

## Findings
- Location: internal/github/search.go:SearchOpenIssuesWithLabels
- Agent: code-simplicity-reviewer

## Proposed Solutions

### Option 1: Rename `parts` → `query`
- **Effort**: Small
- **Risk**: Low

## Acceptance Criteria
- [ ] Variable renamed

## Work Log

### 2026-03-10 - Created from code review
