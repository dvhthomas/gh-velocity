---
title: Command output shape — stats, detail, insights, provenance
category: architecture-patterns
date: 2026-03-13
tags: [output, json, provenance, insights, pipeline, format]
related: [VelocityResult, Provenance, Insight, RenderContext]
---

# Command output shape — stats, detail, insights, provenance

## Problem

As commands grew, each had its own ad-hoc output structure. JSON consumers couldn't predict what fields to expect. No way to reproduce a command's exact context (config, flags, repo).

## Solution

Commands that produce rich output follow a consistent shape with four layers:

### 1. Stats (aggregate numbers)

Summary statistics: mean, median, P90, count, std dev. Used by `report` for single-line summaries.

```go
type Stats struct {
    Count        int
    Mean, Median, P90, P95 *time.Duration
    StdDev       *time.Duration
    OutlierCount int
}
```

### 2. Detail (per-item data)

Per-issue or per-iteration breakdowns. Available in standalone commands, omitted in report summaries.

```go
type IterationVelocity struct {
    Name          string
    Velocity      float64  // effort completed
    Committed     float64  // effort planned
    CompletionPct float64
    // ...
}
```

### 3. Insights (human-readable takeaways)

Computed observations that surface patterns. Stored as `[]Insight` with severity and message.

```go
type Insight struct {
    Level   string // "info", "warning", "success"
    Message string
}
```

### 4. Provenance (reproducibility metadata)

Captures exactly how the output was produced — the command invoked and key config values.

```go
type Provenance struct {
    Command string            // "gh velocity flow velocity --since 30d"
    Config  map[string]string // key config values affecting interpretation
}
```

Built from `cmd.Flags().Visit()` to capture only explicitly-set flags, plus config values that affect interpretation (strategy, field names, project URL).

### Pipeline pattern

Each pipeline follows `GatherData(ctx)` → `ProcessData()` → `Render(rc)`:
- `GatherData` fetches from GitHub API (concurrent, rate-limited)
- `ProcessData` computes metrics (pure, no I/O)
- `Render` outputs in the format specified by `RenderContext`

### JSON output convention

Every JSON output includes:
- `"repository"` — owner/repo
- `"warnings"` — `[]string`, omitempty
- Section-specific data
- `"provenance"` — command + config (standalone commands only, not in report)

## Prevention

- New commands should follow this shape: stats + detail + insights + provenance
- Report embeds summary stats only; standalone commands provide full detail
- Provenance is built from flag visitor pattern, not hardcoded strings
