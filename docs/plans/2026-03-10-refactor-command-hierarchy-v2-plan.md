---
title: "refactor: Reorganize commands into flow/quality/status groups"
type: refactor
status: active
date: 2026-03-10
issue: https://github.com/dvhthomas/gh-velocity/issues/11
---

# Reorganize Commands into flow/quality/status Groups

## Overview

Restructure the flat `gh velocity` command tree into three thematic groups (`flow`, `quality`, `status`) plus a top-level `report` command. This improves discoverability ("how fast are we?" → `flow`) and extensibility (DORA, throughput, aging land naturally in existing groups).

## Problem Statement

The current command tree is a flat grab-bag: `lead-time`, `cycle-time`, `wip`, `stats`, `scope`, `quality release` sit at the same level despite answering different questions. As commands grow (throughput, DORA, aging), the top-level list becomes unnavigable. `scope` requires explanation — it's a diagnostic, not a metric. `stats` is vague — `report` communicates "composite view" better.

## Proposed Solution

### Target Command Tree

```
gh velocity
├── flow                              # "How fast are we?"
│   ├── lead-time [<issue> | --since]
│   └── cycle-time [<issue> | --pr N | --since]
│
├── quality                           # "How good is our output?" (already exists)
│   └── release <tag> [--since <tag>] [--scope]
│
├── status                            # "What's happening now?"
│   └── wip [-R owner/repo]
│
├── report [--since 30d] [--until]    # Composite dashboard (replaces stats)
│
├── config show|validate|create|discover
└── version
```

### Migration Strategy

**Clean break for all moves.** This is pre-1.0 software with no public API contract. No hidden deprecated aliases for moved commands. The only existing alias (`release` → `quality release`) stays as-is since it's already shipped.

Commands removed from top-level:
- `lead-time` → `flow lead-time`
- `cycle-time` → `flow cycle-time`
- `wip` → `status wip`
- `stats` → `report` (renamed, not grouped)
- `scope` → absorbed into `quality release --scope`

### Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Clean break, no aliases | Pre-1.0; `gh` supports user aliases for shortcuts |
| `--scope` is a mode toggle | When set, `quality release` outputs the scope diagnostic view instead of metrics. Reuses existing `WriteScope*` formatters |
| `report` = renamed `stats` | Same `metrics.ComputeDashboard` logic. Rename only, no rewrite. Delegation to group logic deferred (YAGNI) |
| Group parents have no `RunE` | Cobra auto-prints help. PersistentPreRunE skip logic updated to handle group parents |
| Help text includes leaf hints | `Short` descriptions: `"Flow metrics (lead-time, cycle-time)"` |
| Future commands omitted | `throughput`, `dora`, `aging` are not stubbed — only implemented commands appear |

## Technical Approach

### Phase 1: Create Group Parents and Rewire Root

New files and changes:

#### `cmd/flow.go` (new)

```go
func NewFlowCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "flow",
        Short: "Flow metrics (lead-time, cycle-time)",
        Long:  "Velocity and throughput metrics: how fast is work flowing?",
    }
    cmd.AddCommand(NewLeadTimeCmd())
    cmd.AddCommand(NewCycleTimeCmd())
    return cmd
}
```

#### `cmd/status.go` (new — not the current stats.go)

```go
func NewStatusCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "status",
        Short: "Current work status (wip)",
        Long:  "What is happening right now? In-progress work and aging.",
    }
    cmd.AddCommand(NewWIPCmd())
    return cmd
}
```

#### `cmd/root.go` changes

```go
// Before:
root.AddCommand(NewLeadTimeCmd())
root.AddCommand(NewCycleTimeCmd())
root.AddCommand(NewWIPCmd())
root.AddCommand(NewStatsCmd())
root.AddCommand(NewScopeCmd())

// After:
root.AddCommand(NewFlowCmd())
root.AddCommand(NewStatusCmd())
root.AddCommand(NewReportCmd())
// NewQualityCmd() already exists, no change
// Remove NewScopeCmd() — absorbed into quality release
```

#### `PersistentPreRunE` skip logic

The current guard at `root.go:100` must handle group parents. When a user runs `gh velocity flow` (bare), Cobra invokes the parent command which has no `RunE` but inherits `PersistentPreRunE`. Since there's no repo context needed for help display, we must skip:

```go
// Skip Deps setup for commands that don't need it.
// Group parents (flow, status) print help only — no RunE.
switch {
case cmd.Name() == "version":
    return nil
case cmd.Parent() != nil && cmd.Parent().Name() == "config":
    return nil
case !cmd.HasParent() || cmd.RunE == nil && cmd.Run == nil:
    return nil
}
```

The last case catches any command with no `RunE`/`Run` (group parents). This is general and doesn't need updating when new groups are added.

### Phase 2: Rename stats → report

- [ ] Rename `cmd/stats.go` → `cmd/report.go`
- [ ] Change `NewStatsCmd()` → `NewReportCmd()`
- [ ] Change `Use: "stats"` → `Use: "report"`
- [ ] Change `Short` and `Long` descriptions
- [ ] Update `root.go` wiring (Phase 1 handles this)
- [ ] Rename format functions: `WriteStatsJSON` → `WriteReportJSON`, etc. in `internal/format/stats.go` → `report.go`
- [ ] Update `model.StatsResult` → `model.ReportResult` (optional — internal only, can defer)

### Phase 3: Absorb scope into quality release --scope

- [ ] Add `--scope` bool flag to `NewReleaseCmd()` in `cmd/release.go`
- [ ] When `--scope` is set, run the existing scope data-gathering logic (strategy runner) and output using `WriteScope*` formatters instead of the release metrics formatters
- [ ] The scope and release commands already share identical data-gathering code (tag resolution, commit fetching, strategy running). Extract shared logic into a helper if not already shared
- [ ] Delete `cmd/scope.go`
- [ ] Remove `NewScopeCmd()` from root wiring

### Phase 4: Update Tests and Smoke Tests

- [ ] Update `scripts/smoke-test.sh`: all `$BINARY lead-time` → `$BINARY flow lead-time`, etc.
- [ ] Update `$BINARY scope` tests → `$BINARY quality release <tag> --scope`
- [ ] Update `$BINARY stats` tests → `$BINARY report`
- [ ] Update output string assertions (`"Stats:"` → `"Report:"`, etc.)
- [ ] Update `cmd/root_test.go` if any tests reference moved commands
- [ ] Add new tests: bare `flow`, `status` show help text
- [ ] Verify `--help` on each group parent shows leaf commands

### Phase 5: Clean Up

- [ ] Delete `cmd/scope.go`
- [ ] Rename `internal/format/stats.go` → `internal/format/report.go`
- [ ] Rename format functions (`WriteStats*` → `WriteReport*`)
- [ ] Update any references in `internal/metrics/dashboard.go`
- [ ] `go build ./...` and `task quality` pass

## Acceptance Criteria

### Functional

- [ ] `gh velocity flow lead-time 42` works (single issue)
- [ ] `gh velocity flow lead-time --since 30d` works (bulk)
- [ ] `gh velocity flow cycle-time 42` works (single issue)
- [ ] `gh velocity flow cycle-time --pr 99` works (single PR)
- [ ] `gh velocity flow cycle-time --since 30d` works (bulk)
- [ ] `gh velocity quality release v1.0` works (unchanged path)
- [ ] `gh velocity quality release v1.0 --scope` outputs scope diagnostic view
- [ ] `gh velocity status wip` works
- [ ] `gh velocity report --since 30d` works (composite dashboard)
- [ ] `gh velocity report` defaults to 30-day window (same as old stats)
- [ ] `gh velocity flow` prints help listing lead-time and cycle-time
- [ ] `gh velocity status` prints help listing wip
- [ ] `gh velocity --help` shows flow, quality, status, report with descriptive hints
- [ ] Old commands (`lead-time`, `cycle-time`, `wip`, `stats`, `scope` at root) return "unknown command" error

### Non-Functional

- [ ] All smoke tests pass (47+)
- [ ] `task quality` passes (lint + staticcheck)
- [ ] No new dependencies added
- [ ] JSON output schemas unchanged for moved commands (only command path changes)

## Dependencies & Risks

| Risk | Mitigation |
|------|------------|
| PersistentPreRunE fires on group parents | Skip logic generalized to check `RunE == nil && Run == nil` |
| Scope absorption changes release JSON schema | `--scope` is a mode toggle — when set, outputs scope schema; when unset, outputs release schema. No schema merge |
| Smoke test rewrite scope (~39 assertions) | Tests are string-based; changes are mechanical find-replace |
| Users with existing `gh` aliases | Pre-1.0, documented breaking change. Users recreate aliases |

## References

- Brainstorm: `docs/brainstorms/2026-03-10-command-hierarchy-v2-brainstorm.md`
- Previous hierarchy plan (completed): `docs/plans/2026-03-10-feat-command-hierarchy-redesign-plan.md`
- Issue: https://github.com/dvhthomas/gh-velocity/issues/11
- PR: https://github.com/dvhthomas/gh-velocity/pull/12
- Group parent pattern: `cmd/quality.go`, `cmd/config.go`
- PersistentPreRunE: `cmd/root.go:98-171`
- Scope command: `cmd/scope.go`
- Stats command: `cmd/stats.go`
- Smoke tests: `scripts/smoke-test.sh`
