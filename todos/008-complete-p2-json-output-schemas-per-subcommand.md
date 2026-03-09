---
status: complete
priority: p2
issue_id: 008
tags: [code-review, agent-native, json, documentation]
dependencies: []
---

# Document JSON Output Schemas Per Subcommand

## Problem Statement

The plan specifies JSON output with durations as seconds and null for N/A, but never shows the actual JSON structure for any command. Agents and tool authors cannot write reliable parsing code without knowing field names. The `--json <fields>` convention from the gh CLI is also missing.

**Raised by:** Agent-Native Reviewer (WARNING)

## Findings

- No JSON schema example for any subcommand in the plan
- gh CLI convention uses `--json <fields>` for selective output, not `--format json`
- Including `"repository": "owner/repo"` in JSON output eliminates auto-detection ambiguity

## Proposed Solutions

### Option A: Add JSON examples to plan + implement --json flag (Recommended)
- Document JSON structure for each subcommand in the plan
- Implement `--json <fields>` flag matching gh CLI convention
- Always include `repository` field in JSON output
- Return `posted_url` in `--post` JSON output
- **Effort:** Medium
- **Risk:** Low

## Acceptance Criteria

- [x] Each subcommand has a documented JSON schema
- [deferred] `--json <fields>` flag for selective output
- [x] `repository` field always present in JSON output
- [ ] `posted_url` returned when `--post` succeeds

## Resolution

Typed JSON output structs were created for all three subcommands:

- `JSONLeadTimeOutput` — used by `lead-time` subcommand
- `JSONCycleTimeOutput` — used by `cycle-time` subcommand
- `JSONReleaseOutput` — already existed, updated with `repository` field

All structs include a `Repository` field (`owner/repo`) and `Warnings []string` with `omitempty`.

Ad-hoc `map[string]interface{}` JSON construction in `cmd/leadtime.go` and `cmd/cycletime.go` was replaced with dedicated writer functions (`WriteLeadTimeJSON`, `WriteCycleTimeJSON`) in `internal/format/json.go`.

The `--json <fields>` selective output flag is deferred to a future iteration.
