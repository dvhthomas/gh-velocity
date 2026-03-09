---
status: pending
priority: p2
issue_id: 003
tags: [code-review, architecture, simplicity, yagni]
dependencies: []
---

# Simplify Subcommands: Merge lead-time/cycle-time into summary

## Problem Statement

The plan has 3 separate subcommands (`lead-time`, `cycle-time`, `summary`) that all accept `<issue>` and compute overlapping metrics. `summary` already computes both lead time and cycle time. Having 3 commands means 3 command files, 3 sets of tests, 3 help entries — all to slice the same data differently.

**Raised by:** Code Simplicity Reviewer (HIGH)

## Findings

- `summary` already includes lead time and cycle time
- Users who want only lead time can use `gh velocity issue 42 --format json | jq .lead_time_seconds`
- Removing 2 commands eliminates ~2 files and simplifies help output

## Proposed Solutions

### Option A: Merge into `issue` command (Simplicity recommended)
- Rename `summary` to `gh velocity issue <number>`
- Remove `leadtime.go` and `cycletime.go`
- 5 subcommands total: `issue`, `pr-metrics`, `release`, `throughput`, `version`
- Defer `project` to v2
- **Pros:** Simpler CLI surface, faster v1, less code
- **Effort:** Small (plan change only — no code yet)
- **Risk:** Low — can always add `lead-time` as an alias later

### Option B: Keep separate commands
- More discoverable for users who know exactly what they want
- **Pros:** Explicit, each command does one thing
- **Effort:** Medium (3 files instead of 1)
- **Risk:** Low

## Recommended Action

Decide before Phase 1 implementation begins. Both approaches are valid.

## Acceptance Criteria

- [ ] Decision documented in plan
- [ ] Help output is clean and non-overlapping
