---
status: pending
priority: p3
issue_id: "047"
tags: [code-review, pattern, duplication]
dependencies: []
---

# searchResponse-to-model.Issue Conversion Helper

## Problem Statement
Multiple search functions in `internal/github/search.go` have near-identical loops converting GitHub search response items to `model.Issue`. This could be extracted into a shared conversion helper.

## Findings
- Location: internal/github/search.go
- Agent: pattern-recognition-specialist

## Proposed Solutions

### Option 1: Extract `searchItemToIssue()` helper
- **Effort**: Small
- **Risk**: Low

## Acceptance Criteria
- [ ] Single conversion helper used by all search functions

## Work Log

### 2026-03-10 - Created from code review
