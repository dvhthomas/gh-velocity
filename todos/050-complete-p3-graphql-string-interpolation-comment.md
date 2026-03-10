---
status: pending
priority: p3
issue_id: "050"
tags: [code-review, convention, documentation]
dependencies: []
---

# GraphQL String Interpolation Convention Comment

## Problem Statement
The search query in `internal/github/search.go` uses `fmt.Sprintf` to build the query string, which looks like it violates the "GraphQL variables only" convention in CLAUDE.md. However, the GitHub Search API requires the query as a single string parameter — this is a legitimate exception. A comment explaining why would prevent future reviewers from flagging it.

## Findings
- Location: internal/github/search.go
- Agent: security-sentinel

## Proposed Solutions

### Option 1: Add explanatory comment
- **Effort**: Small
- **Risk**: Low

## Acceptance Criteria
- [ ] Comment explains why string interpolation is used for search queries

## Work Log

### 2026-03-10 - Created from code review
