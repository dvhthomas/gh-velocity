---
status: pending
priority: p2
issue_id: "044"
tags: [code-review, agent-native, json]
dependencies: []
---

# null vs [] in JSON Arrays

## Problem Statement
WIP and bulk JSON output emit `null` instead of `[]` when slices are nil. Machine consumers and agents treat `null` and `[]` differently — `null` may cause NPEs or require special handling, while `[]` is universally safe to iterate.

## Findings
- Location: `internal/format/wip.go` (items slice)
- Location: `internal/format/leadtime.go`, `internal/format/cycletime.go` (bulk issues array)
- Agent: agent-native-reviewer
- Go's `json.Marshal` emits `null` for nil slices, `[]` for empty slices

## Proposed Solutions

### Option 1: Initialize slices to empty instead of nil
- **Pros**: Simple, Go-idiomatic (`make([]T, 0)` or `[]T{}`)
- **Cons**: Need to audit all JSON-emitting paths
- **Effort**: Small
- **Risk**: Low

## Recommended Action
_To be filled during triage_

## Technical Details
- **Affected Files**: internal/format/wip.go, internal/format/leadtime.go, internal/format/cycletime.go
- **Related Components**: All JSON formatters
- **Database Changes**: No

## Acceptance Criteria
- [ ] All JSON array fields emit `[]` not `null` when empty
- [ ] Tests validate empty-state JSON output

## Work Log

### 2026-03-10 - Created from code review
**By:** Claude Code Review
**Actions:**
- Finding identified by agent-native-reviewer
