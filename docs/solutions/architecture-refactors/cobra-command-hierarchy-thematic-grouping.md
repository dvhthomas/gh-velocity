---
title: "Reorganize flat CLI commands into thematic groups (flow, quality, status)"
category: architecture-refactors
tags:
  - cli-ux
  - cobra
  - command-hierarchy
  - breaking-change
module: cmd
symptom: "Flat command tree becomes unnavigable as command count grows; related commands (lead-time, cycle-time) not grouped by intent"
root_cause: "Original design placed all subcommands at root level with no grouping; worked for 3-4 commands but does not scale to planned additions (throughput, DORA, aging)"
date: 2026-03-10
severity: low
resolution_time: "< 1 day"
---

# Reorganize Flat CLI Commands into Thematic Groups

## Problem

The `gh velocity` CLI had a flat command tree where `lead-time`, `cycle-time`, `wip`, `stats`, and `scope` all sat at the root level. As commands grew, this became unnavigable. Commands answered different questions (speed vs quality vs current state) but weren't organized by intent. The `scope` command required explanation and wasn't a metric. `stats` was vague.

## Solution

### 1. Group Parent Commands (No RunE Pattern)

Created bare Cobra commands that serve as group parents. These have no `RunE`, so Cobra automatically prints help text listing subcommands.

**`cmd/flow.go`** — groups speed-related commands:
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

**`cmd/status.go`** — groups current-state commands:
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

### 2. Renamed stats to report

Coordinated rename across all layers:
- File: `cmd/stats.go` -> `cmd/report.go`
- Factory: `NewStatsCmd()` -> `NewReportCmd()`, `Use: "report"`
- Format functions: `WriteStatsJSON/Markdown/Pretty` -> `WriteReportJSON/Markdown/Pretty`
- Format file: `internal/format/stats.go` -> `internal/format/report.go`
- Output strings: `"Stats:"` -> `"Report:"`

### 3. Absorbed scope into quality release --scope

Added `--scope` bool flag to `NewReleaseCmd()`. Modified `gatherReleaseData()` to return `*model.ScopeResult` alongside `metrics.ReleaseInput`. When `--scope` is set, outputs scope diagnostic view via `WriteScope*` formatters instead of release metrics. Deleted standalone `cmd/scope.go`.

### 4. PersistentPreRunE Skip Logic (Generalized)

Replaced explicit name-based skipping with a generic check that catches any group parent:

```go
switch {
case cmd.Name() == "version":
    return nil
case cmd.Parent() != nil && cmd.Parent().Name() == "config":
    return nil
case cmd.RunE == nil && cmd.Run == nil:
    return nil
}
```

The third case is the key insight: group parents have no run function, so they can be skipped generically. Adding future group commands requires zero changes to root.go's pre-run logic.

### 5. Root Command Wiring

```go
// Before (flat)
root.AddCommand(NewLeadTimeCmd())
root.AddCommand(NewCycleTimeCmd())
root.AddCommand(NewScopeCmd())
root.AddCommand(NewWIPCmd())
root.AddCommand(NewStatsCmd())

// After (grouped)
root.AddCommand(NewFlowCmd())      // contains lead-time, cycle-time
root.AddCommand(NewStatusCmd())    // contains wip
root.AddCommand(NewReportCmd())    // renamed from stats
// scope absorbed into: quality release --scope
```

### 6. Smoke Test Updates

All 55 smoke tests updated. Added group help tests and old-command-rejection tests to verify the clean break.

### Key Design Decisions

- **Clean break** — pre-1.0, no deprecated aliases for moved commands
- **Group parents have no RunE** — Cobra auto-prints help
- **`--scope` is a mode toggle** — not a separate command, since it shares data-gathering with release
- **`report` = renamed stats** — same `ComputeDashboard` logic (YAGNI, delegation deferred)

## Prevention & Best Practices

### Cobra Group Parent Patterns

Group commands should never have a `RunE` or `Run` function. Cobra automatically prints help when a command lacks a run function. This also enables the generic PersistentPreRunE skip.

**Pitfall: Cobra forbids sharing command instances across parents.** `NewLeadTimeCmd()` returns a `*cobra.Command` that can only be added to one parent. For pre-1.0 software, pick one canonical path and drop the other.

### PersistentPreRunE Pitfalls

**The core problem:** `PersistentPreRunE` on root fires for every command, including group parents. Setup logic (config loading, auth) runs even when Cobra is about to print help, causing confusing errors.

**Solution:** Skip commands with `cmd.RunE == nil && cmd.Run == nil`. This is strictly better than a name-based skip list because it requires zero maintenance when adding new groups.

**Additional pitfall:** A child command that defines its own `PersistentPreRunE` silently replaces the parent's — it does not chain. If a subcommand needs its own persistent pre-run logic, it must explicitly call the parent's.

### When to Use Mode Flags vs Separate Commands

**Use a flag** when the data source is the same, output is a variation of the same report, and a separate command would duplicate 80%+ of setup logic.

**Use a command** when inputs differ, the data pipeline is fundamentally different, or the mental model is "these are different things."

### Migration Strategy for CLI Restructuring

1. **Plan the rename map up front** — enumerate every layer (cmd, format, tests, docs, CI)
2. **Change from the inside out** — internal layer first, then commands, then tests. Compilation errors guide you.
3. **Add negative smoke tests** — verify old invocations fail with clear errors
4. **Pre-1.0: prefer clean breaks** — deprecated aliases have ongoing cost

## References

### Planning Documents
- Brainstorm: `docs/brainstorms/2026-03-10-command-hierarchy-v2-brainstorm.md`
- Plan: `docs/plans/2026-03-10-refactor-command-hierarchy-v2-plan.md`

### GitHub
- Issue: https://github.com/dvhthomas/gh-velocity/issues/11
- PR: https://github.com/dvhthomas/gh-velocity/pull/12

### Related Solutions
- `docs/solutions/cycle-time-signal-hierarchy.md` — signal priority pattern used by flow commands
- `docs/solutions/three-state-metric-status-pattern.md` — metric status pattern shared across hierarchy
- `docs/solutions/go-gh-tableprinter-migration.md` — terminal detection pattern used by all formatters

### Key Files
- Group parents: `cmd/flow.go`, `cmd/status.go`, `cmd/quality.go`
- Root wiring + PersistentPreRunE: `cmd/root.go`
- Report formatters: `internal/format/report.go`
- Scope formatters (used by --scope): `internal/format/scope.go`
- Smoke tests: `scripts/smoke-test.sh`
