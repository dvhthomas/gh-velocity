---
title: "Directory-per-metric pipeline layout with preflight-first configuration"
category: architecture-refactors
tags:
  - pipeline
  - architecture
  - onboarding
  - config
  - preflight
  - categories
module: internal/pipeline, cmd/preflight, internal/config
symptom: "Adding a new metric required changes in 4+ packages (metrics/, metric/, format/, cmd/) with duplicated render code scattered across format/ and format/templates/; new users had no guided path to a working config"
root_cause: "Original layout split each metric across multiple packages by concern (compute here, format there, template elsewhere). This spread knowledge thin and made the codebase hard to navigate. Legacy bug_labels/feature_labels config added confusion alongside the newer categories system."
date: 2026-03-12
severity: medium
resolution_time: "2 days"
---

# Directory-per-Metric Pipeline Layout with Preflight-First Configuration

## Problem

Two related problems compounded each other:

**1. Adding a new metric was hard.** Each metric was scattered across 4+ packages:
- `internal/metrics/` — computation (e.g., `busfactor.go`)
- `internal/metric/` — Pipeline struct (e.g., `busfactor.go`)
- `internal/format/` — render functions (e.g., `busfactor.go`, `pretty.go`)
- `internal/format/templates/` — markdown templates
- `cmd/` — command wiring

Finding all the pieces for one metric required grep. Adding a new metric meant touching all of these and knowing which types to export where.

**2. New users had no clear path to a working config.** The config system supported both `bug_labels`/`feature_labels` (legacy) and `categories` (modern) with auto-promotion logic in `resolveCategories()`. This dual-path confused both the code and the user. Config validation was possible, but discovery wasn't guided.

## Root Cause

The original architecture organized code by **technical concern** (compute, format, template, command) rather than by **metric**. This is a common pattern in Go projects, but it breaks down when the number of metrics grows — each new metric requires coordinated changes across all concern packages.

The `internal/cycletime/` package (Strategy interface) existed separately from both `internal/metrics/` and `internal/pipeline/cycletime/` because of import cycle avoidance, adding a third location to check for cycle-time related code.

## Solution

### 1. Directory-per-metric layout

Reorganized into `internal/pipeline/<metric>/` where each metric owns everything:

```
internal/pipeline/
  pipeline.go                    # Pipeline interface (GatherData, ProcessData, Render)
  busfactor/
    busfactor.go                 # Pipeline struct + GatherData/ProcessData
    render.go                    # WriteJSON, WriteMarkdown, WritePretty
    busfactor_test.go
    templates/busfactor.md.tmpl
  cycletime/
    cycletime.go                 # IssuePipeline, PRPipeline, BulkPipeline
    render.go
    cycletime_test.go
    templates/cycletime.md.tmpl
    templates/cycletime-bulk.md.tmpl
  leadtime/    ...same pattern...
  release/     ...same pattern...
  reviews/     ...same pattern...
  throughput/  ...same pattern...
```

**Adding a new metric now means:**
1. Create `internal/pipeline/newmetric/` with `newmetric.go`, `render.go`, `templates/`, tests
2. Wire it in `cmd/newmetric.go`
3. Done. No changes to `format/`, `metrics/`, or other packages.

### 2. Two-layer separation

- **`internal/metrics/`** — pure computation: `ComputeStats()`, `LeadTime()`, `BuildReleaseMetrics()`, cycle-time strategies (`IssueStrategy`, `PRStrategy`, `ProjectBoardStrategy`). No API calls, no formatting. Shared by multiple pipelines.
- **`internal/pipeline/<metric>/`** — command orchestration: API calls (GatherData), computation delegation (ProcessData calls `metrics.*`), formatting (Render). Each pipeline is self-contained.

The standalone `internal/cycletime/` package was merged into `internal/metrics/` since the Strategy interface is pure computation used by multiple consumers.

### 3. Report wiring with pipelines

`cmd/report.go` was rewritten from a monolithic `metrics.ComputeDashboard()` call to constructing individual pipelines:

```go
leadPipeline := &leadtime.BulkPipeline{...}
cyclePipeline := &cycletimepipe.BulkPipeline{...}
throughputPipeline := &throughput.Pipeline{...}

// GatherData concurrently
g, gctx := errgroup.WithContext(ctx)
g.Go(func() error { return leadPipeline.GatherData(gctx) })
g.Go(func() error { return cyclePipeline.GatherData(gctx) })
g.Go(func() error { return throughputPipeline.GatherData(gctx) })
_ = g.Wait()

// ProcessData sequentially, assemble StatsResult
```

Each section degrades gracefully — a failure in lead-time doesn't block cycle-time.

### 4. Categories-only config

Removed `bug_labels`/`feature_labels` entirely. Config now uses only `categories`:

```yaml
quality:
  categories:
    - name: bug
      match:
        - "label:bug"
    - name: feature
      match:
        - "label:enhancement"
```

This eliminated `resolveCategories()`, `FromLegacyLabels()`, `slicesEqual()`, and the dual-path confusion.

### 5. Preflight as the guided onboarding path

The `config preflight` command is the critical first-run experience. It:

1. **Analyzes the repo** — discovers labels, issue types, recent activity
2. **Generates a tailored config** — scope, categories with evidence-based matchers, lifecycle
3. **Shows match evidence** — proves each matcher finds real issues (e.g., `bug / label:bug — 33 matches`)
4. **Validates the config** — round-trip YAML parse + validation before writing
5. **Writes with `--write`** — produces a `.gh-velocity.yml` ready for all commands

The flow for a new user:
```
gh velocity config preflight -R owner/repo     # see what it would generate
gh velocity config preflight -R owner/repo --write  # save it
gh velocity report --since 30d                  # immediately works
```

Every command except `preflight` itself requires a config file. This is intentional — `preflight` is the gateway.

## Impact

- **~2,900 lines net reduction** across the codebase
- Deleted 3 packages entirely: `internal/metric/`, `internal/cycletime/`, `internal/metrics/busfactor.go`
- Removed 7 template files from `internal/format/templates/`
- Each metric is now self-contained in one directory
- Config has one path, not two

## Prevention / Best Practices

**When adding a new metric:**
1. Copy an existing `internal/pipeline/<metric>/` directory as a template
2. Implement the 3-method Pipeline interface: `GatherData`, `ProcessData`, `Render`
3. Use `metrics.ComputeStats()` for aggregate statistics
4. Use `format.TemplateFuncMap()` as the base for markdown templates
5. Wire in `cmd/<metric>.go` — single file, single command

**When adding new config fields:**
- Add to the appropriate struct in `internal/config/config.go`
- Add defaults in `defaults()`
- Add validation in `validate()`
- Update `config preflight` to discover/suggest the value
- Update `config create` template
- Update `config show` output
- No legacy/migration paths needed — categories-only going forward

**Preflight is the source of truth for config correctness.** If preflight can't generate it, users shouldn't have to write it by hand.
