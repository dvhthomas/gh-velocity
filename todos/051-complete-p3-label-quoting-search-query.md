---
status: pending
priority: p3
issue_id: "051"
tags: [code-review, correctness]
dependencies: []
---

# Label Quoting in Search Query

## Problem Statement
`SearchOpenIssuesWithLabels` uses `fmt.Sprintf(" label:%q", l)` which adds Go-style quotes around labels. GitHub Search API expects unquoted labels or labels with double quotes only for multi-word labels. The `%q` format may produce incorrect escaping for some label names.

## Findings
- Location: internal/github/search.go:SearchOpenIssuesWithLabels
- Agent: security-sentinel

## Proposed Solutions

### Option 1: Use `"label:\"%s\""` for multi-word labels, plain for single-word
- **Effort**: Small
- **Risk**: Low

## Acceptance Criteria
- [ ] Labels with spaces are properly quoted for GitHub Search API
- [ ] Single-word labels work correctly

## Work Log

### 2026-03-10 - Created from code review
