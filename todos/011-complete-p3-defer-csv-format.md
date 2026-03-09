---
status: pending
priority: p3
issue_id: 011
tags: [code-review, simplicity, yagni]
dependencies: []
---

# Consider Deferring CSV Format to Post-v1

## Problem Statement

The plan requires 4 output formats from Phase 1. CSV has the most edge-case pain (quoting, escaping, headers) for the least unique value — anyone who needs spreadsheet data can convert from JSON.

**Raised by:** Code Simplicity Reviewer

## Findings

- JSON covers the machine-readable case
- CSV quoting/escaping rules are finicky
- `jq -r` or any modern spreadsheet can import JSON directly
- Removing CSV saves 1 formatter file and its tests

## Proposed Solutions

### Option A: Ship v1 with 3 formats (json, pretty, markdown)
- Add CSV later if users request it
- **Effort:** Saves small effort
- **Risk:** Low

### Option B: Keep CSV
- Some users expect CSV from metrics tools
- **Effort:** Small to implement
- **Risk:** Low

## Recommended Action

Decision for plan author. Either is fine — this is a P3 simplification.
