---
title: "feat: Label sync Action + project board lifecycle removal"
type: feat
status: active
date: 2026-03-19
origin: docs/brainstorms/2026-03-19-label-sync-and-board-removal-requirements.md
---

# feat: Label sync Action + project board lifecycle removal

## Overview

Two coordinated efforts: (A) a standalone Go-based GitHub Action that bidirectionally syncs issue labels with GitHub Projects v2 status fields, and (B) removal of project board as a lifecycle signal from gh-velocity, making labels the sole source of truth for cycle-time and WIP.

This is a breaking change with no deprecation shim. (See origin: `docs/brainstorms/2026-03-19-label-sync-and-board-removal-requirements.md`)

## Problem Statement

GitHub Projects v2 `updatedAt` on status fields reflects last-any-field-update, not the status transition time. This makes project-board-based cycle-time fundamentally unreliable (negative cycle times, stale data). The existing deprecation (completed in plan `2026-03-13-003`) added labels-first priority and warnings, but the project board fallback path remains — confusing users and producing misleading data.

## Part A: Standalone Label-Sync GitHub Action

### Architecture

New repository: `dvhthomas/gh-project-label-sync` (or similar)

**Go binary** distributed as a GitHub Action via goreleaser. Runs on a cron schedule.

**Core loop:**
1. Query all open issues in the configured project via GraphQL (batched, paginated)
2. For each issue, read its Status field value and its current labels
3. Compare: does the status match the expected label? Does a status-label exist that doesn't match the board?
4. Reconcile using most-recent-write-wins (label `createdAt` vs board item `updatedAt`)
5. Apply changes: add/remove labels via `gh` CLI, update board status via GraphQL mutation

**Implementation constraints** (see origin):
- Prefer `gh` CLI for label management (`gh issue edit --add-label`, `gh issue edit --remove-label`). Use custom Go + GraphQL only for project item reads and field value updates.
- Rate limit detection with exponential backoff, respect `Retry-After` headers
- Retry on transient failures (5xx, network errors) — 3 attempts with jitter
- Structured logging: every decision logged (what changed, what skipped, why)
- Dry-run by default — live mode requires explicit `dry-run: false` input

**Action inputs:**
```yaml
inputs:
  project-url:
    description: 'GitHub Projects v2 URL'
    required: true
  token:
    description: 'Classic PAT with project + repo scopes'
    required: true
  label-prefix:
    description: 'Label prefix for status labels'
    default: 'status:'
  dry-run:
    description: 'Log changes without applying them'
    default: 'true'
```

**Example usage:**
```yaml
name: Sync Project Labels
on:
  schedule:
    - cron: '*/15 * * * *'
  workflow_dispatch:

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: dvhthomas/gh-project-label-sync@v1
        with:
          project-url: https://github.com/users/dvhthomas/projects/1
          token: ${{ secrets.PROJECT_PAT }}
          dry-run: false
```

**Token requirements:** Classic PAT with `project` + `repo` scopes. `GITHUB_TOKEN` cannot access project data. Fine-grained PATs do not support Projects v2 GraphQL API.

**Scope:** Open issues only. Single project, single repo. No PR sync. No cross-repo sync.

### Key technical decisions

- **Cron polling, not event-driven**: `projects_v2_item` is not a GitHub Actions trigger. User-owned projects have no webhooks. Polling is the only universal option.
- **Conflict resolution**: Most-recent-write-wins. If label `createdAt` > board `updatedAt`, label wins (apply label's status to board). Otherwise board wins (apply board's status as label). 15-minute polling interval makes clock skew irrelevant.
- **Auto-create labels**: First sync creates any missing `status:*` labels with neutral gray color.
- **Competing labels**: When applying `status:In Progress`, remove any other `status:*` labels (only one status label at a time).

## Part B: gh-velocity Project Board Lifecycle Removal

### Phase 1: Delete project board cycle-time code

**Delete functions/types** from `internal/github/cyclestart.go` (lines 166-422):
- `projectStatusResponse`, `projectItemNode`, `fieldValueNode` structs
- `ProjectStatus` struct
- `GetProjectStatus`, `fetchProjectStatus`, `matchProjectStatus`
- `BatchGetProjectStatuses`, `fetchProjectStatusBatch`
- `projectStatusCacheKey`, `hasProjectToken`

**Delete from `internal/metrics/cycletime.go`:**
- `IssueStrategy` fields: `ProjectID`, `StatusFieldID`, `BacklogStatus`
- `computeFromProject` method (lines 66-88)
- Project board fallback block in `Compute` (lines 56-59)

**Delete from `internal/pipeline/cycletime/cycletime.go`:**
- `BulkPipeline` fields: `ProjectID`, `StatusFieldID`, `BacklogStatus`
- Batch pre-fetch block in `GatherData` (lines 192-203)

**Delete from `cmd/helpers.go`:**
- Project board block in `buildCycleTimeStrategy` (lines 29-38)
- "Project board only signal" warning (lines 40-43)
- Entire `setCycleTimeBatchParams` function (lines 56-62)

**Delete batch pre-fetch calls from:**
- `cmd/cycletime.go` line 268
- `cmd/report.go` line 145
- `cmd/myweek.go` lines 244-255
- `internal/metrics/release.go` lines 50-61

**Delete tests** from `internal/github/cyclestart_test.go`: 9 project status test functions.

**Update `cmd/helpers.go`:**
- Simplify "no signal source" check: `if len(strat.InProgressMatch) == 0 { strat.Client = nil }`
- Remove `ProjectID` check

### Phase 2: Remove `lifecycle.*.project_status` config field

**`internal/config/config.go`:**
- Delete `ProjectStatus []string` from `LifecycleStage` struct (line 113)
- Delete validation block requiring `project.url` + `status_field` when `project_status` is used (lines 347-358)
- Delete `"project-board"` deprecated strategy warning (lines 322-328)
- Add validation: if `cycle_time.strategy == "issue"`, require `lifecycle.in-progress.match` to be non-empty

**`internal/config/config_test.go`:**
- Delete 3 `project_status` validation test cases (lines 461-479)
- Add test: issue strategy without `lifecycle.in-progress.match` fails validation

**`cmd/config.go`:**
- Update `defaultConfigTemplate`: remove all `project_status` examples from lifecycle section, show only `match` examples
- Update `config show` pretty-print: remove `project.status_field` line (or keep only when velocity uses it)

### Phase 3: Rewrite WIP command to use labels

**`cmd/wip.go`** — complete rewrite of `runWIP`:

Current flow: requires `project.url` → resolves project → lists board items → filters by `lifecycle.*.project_status` → renders

New flow:
1. Read `lifecycle.in-progress.match` and `lifecycle.in-review.match` from config
2. Build GitHub search query: `is:open is:issue` + scope query + label matchers
3. Fetch matching issues via search API
4. Group by stage (in-progress vs in-review)
5. Render same output format

The `model.WIPItem` struct already supports label-based display. The command no longer requires `project.url` — it works for anyone with lifecycle label config.

**Fallback:** If neither `lifecycle.in-progress.match` nor `lifecycle.in-review.match` is configured, error with: `"wip requires lifecycle.in-progress.match or lifecycle.in-review.match in config"`

### Phase 4: Update preflight and documentation

**`cmd/preflight.go`:**
- `writeLifecycleMapping` and `writeStage` functions: emit only `match` entries, never `project_status`
- Verification check: require `hasMatch` (not `hasProjectStatus`) for issue strategy
- Update hint text about lifecycle config

**Documentation (11 files):**
- `docs/guide.md` — remove project board cycle-time references, update "labels vs board" section
- `site/content/concepts/labels-vs-board.md` — rewrite: labels are the only option, not a recommendation
- `site/content/guides/cycle-time-setup.md` — remove project_status config examples
- `site/content/reference/config.md` — remove `lifecycle.*.project_status` field docs
- `site/content/reference/metrics/cycle-time.md` — update signal documentation
- `docs/examples/dvhthomas-gh-velocity.yml` — replace project_status with match
- `.gh-velocity.yml` — replace project_status with match (our own config)
- Other docs with `project_status` references

**Add to docs:** Recommended label-sync Action for project board users (link to Part A repo). Frame as "if you use GitHub Projects, here's how to keep labels in sync" — optional, not required.

### Phase 5: Update the `issue` command and workflow

- Update `internal/pipeline/issue/issue.go` cycle-time N/A reason: "no in-progress signal found" → "configure lifecycle.in-progress.match for cycle time"
- Verify the `velocity-item.yaml` workflow still works after changes

## What Stays

| Feature | Why |
|---------|-----|
| `project.url` config field | Used by velocity iteration/effort strategies |
| `project.status_field` config field | Used by velocity to read board fields |
| `ResolveProject` | Used by velocity, config validate |
| `ListProjectItems` | Used by preflight for type discovery |
| `ListProjectItemsWithFields` | Used by velocity pipeline |

## Acceptance Criteria

### Part A (Label-Sync Action)
- [ ] New repo with Go binary + Action metadata (`action.yml`)
- [ ] Bidirectional sync: board→labels and labels→board
- [ ] Cron-scheduled (configurable interval)
- [ ] Dry-run by default
- [ ] Robust: rate limit backoff, retry on transient failures, structured logging
- [ ] Works for user-owned and org-owned projects
- [ ] Auto-creates missing labels
- [ ] Removes competing status labels

### Part B (gh-velocity Cleanup)
- [ ] `lifecycle.*.project_status` removed from config struct and validation
- [ ] All project-board cycle-time code deleted (11 functions/types in cyclestart.go)
- [ ] `IssueStrategy` uses only label signals
- [ ] WIP command works with label queries (no project.url required)
- [ ] Config validation requires `lifecycle.in-progress.match` for issue strategy
- [ ] Preflight generates only `match` entries (no `project_status`)
- [ ] `defaultConfigTemplate` updated
- [ ] All 24 test packages pass
- [ ] Documentation consistent — no `project_status` lifecycle references
- [ ] Own repo's `.gh-velocity.yml` uses label-based lifecycle

## Dependencies & Risks

- **Part A and Part B are independent.** Part B can ship first (labels required, sync Action recommended). Part A can ship whenever — it's a separate repo.
- **WIP command regression risk:** Users relying on board-based WIP lose functionality until they configure `lifecycle.*.match`. Mitigate with clear error message and preflight generating match config.
- **Preflight label detection gaps:** Issue #109 notes that preflight's label detection uses exact string matching and misses variants. Users migrating from board-based lifecycle may not get good suggestions. Consider improving fuzzy matching in preflight as part of Phase 4.

## Sources & References

### Origin
- **Origin document:** [docs/brainstorms/2026-03-19-label-sync-and-board-removal-requirements.md](docs/brainstorms/2026-03-19-label-sync-and-board-removal-requirements.md) — Key decisions: breaking change, labels sole lifecycle signal, bidirectional cron sync, separate repo, project board kept for velocity reads.

### Internal References
- Completed deprecation plan: `docs/plans/2026-03-13-003-fix-deprecate-project-board-cycle-start-plan.md`
- Cycle time signal hierarchy: `docs/solutions/cycle-time-signal-hierarchy.md`
- Label-based lifecycle pattern: `docs/solutions/architecture-patterns/label-based-lifecycle-for-cycle-time.md`
- Preflight label detection gaps: `docs/solutions/architecture-patterns/preflight-lifecycle-label-detection.md` (issue #109)
- Config validation checklist: `docs/solutions/architecture-refactors/pipeline-per-metric-and-preflight-first-config.md`
- Breaking change pattern: `docs/solutions/architecture-refactors/cobra-command-hierarchy-thematic-grouping.md`

### Key files (blast radius)
- Cycle time core: `internal/metrics/cycletime.go`, `internal/github/cyclestart.go`
- Pipeline: `internal/pipeline/cycletime/cycletime.go`
- Commands: `cmd/helpers.go`, `cmd/wip.go`, `cmd/cycletime.go`, `cmd/report.go`, `cmd/myweek.go`
- Config: `internal/config/config.go`, `cmd/config.go`, `cmd/preflight.go`
- Release metrics: `internal/metrics/release.go`
