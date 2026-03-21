---
title: "feat: Enrich REST-sourced issues with IssueType for type: matchers"
type: feat
status: completed
date: 2026-03-21
origin: docs/brainstorms/2026-03-21-issue-type-enrichment-requirements.md
---

# feat: Enrich REST-sourced issues with IssueType for type: matchers

## Overview

REST Search API does not return GitHub Issue Types. Commands that classify issues (report quality, WIP, velocity, config validate) use REST search, so `type:` matchers in the config silently fail — `IssueType` is always empty. The release command already works (uses GraphQL `FetchIssues`), but all other metric commands do not.

This plan adds a lightweight `EnrichIssueTypes()` GraphQL method and gates it on `HasTypeMatchers()` so repos without `type:` matchers pay zero API cost (see origin: `docs/brainstorms/2026-03-21-issue-type-enrichment-requirements.md`).

## Problem Statement

Repos relying on GitHub Issue Types for classification get correct configs from preflight (shipped in companion PR) but broken classification at metric time. `type:Bug` matchers never match because `model.Issue.IssueType` is empty for all REST-sourced issues.

## Proposed Solution

Three components:

1. **`EnrichIssueTypes(ctx, issues []model.Issue) error`** on `*gh.Client` — lightweight GraphQL batch query fetching only `number + issueType { name }`, mutates issues in-place via index-based assignment.
2. **`HasTypeMatchers() bool`** on `*classify.Classifier` — scans parsed matchers for any `TypeMatcher` instance.
3. **Enrichment calls at cmd/ layer** — between `GatherData()` and `ProcessData()` for report pipelines; after search for standalone commands.

## Technical Approach

### Phase 1: Client method + Classifier gate

#### `EnrichIssueTypes` (`internal/github/enrich.go`)

```go
// EnrichIssueTypes batch-fetches IssueType for the given issues via GraphQL
// and sets it in-place. Issues that already have IssueType set are skipped.
// Batches in groups of 20 (matching FetchIssues pattern).
// Errors are non-fatal: failed batches leave IssueType empty (falls through
// to label/title matchers).
func (c *Client) EnrichIssueTypes(ctx context.Context, issues []model.Issue) error
```

- Query shape: `issue%d: issue(number: %d) { number issueType { name } }` — minimal fields
- Response: aliased map, same pattern as `fetchIssuesBatch` in `batch.go`
- Mutation: `issues[i].IssueType = typeName` (index-based, not range-copy)
- Skip issues where `IssueType != ""` (already enriched, e.g., from GraphQL)
- Cache: in-memory only via `DoJSON` with `CacheKey("enrich-issue-types", sortedNumbersCSV)` — different key space from `FetchIssues` (which doesn't use DoJSON)
- Error handling: log warning per failed batch, continue with best-effort enrichment
- Empty input: return nil immediately

#### `HasTypeMatchers` (`internal/classify/classify.go`)

```go
// HasTypeMatchers reports whether any category uses a type: matcher.
// Useful for gating expensive IssueType enrichment calls.
func (c *Classifier) HasTypeMatchers() bool {
    for _, matchers := range c.matchers {
        for _, m := range matchers {
            if _, ok := m.(TypeMatcher); ok {
                return true
            }
        }
    }
    return false
}
```

### Phase 2: Export `leadPipeline.issues` for enrichment access

The lead-time pipeline's `issues` field is unexported (`leadtime.BulkPipeline.issues`). The cmd/ layer needs access to enrich issues between `GatherData()` and `ProcessData()`.

**Option chosen: export the field** — rename `issues` to `Issues` in `BulkPipeline`. This is the simplest change and consistent with other exported fields like `Items`, `Stats`, `Warnings`. The field is already assigned in `GatherData()` and read in `ProcessData()`.

Same applies to `cycletime.BulkPipeline` if it has an unexported issues field (check during implementation — cycle-time doesn't classify but enriching there future-proofs).

### Phase 3: Wire enrichment into report pipeline

**Critical ordering:** enrichment MUST happen after `g.Wait()` (all GatherData complete) and BEFORE `ProcessData()` calls. `ProcessData()` copies `model.Issue` structs by value into `BulkItem` — enriching after `ProcessData()` is too late.

In `cmd/report.go`, after line 255 (`g.Wait()`), before line 265 (`leadPipeline.ProcessData()`):

```go
// Enrich REST-sourced issues with IssueType when config uses type: matchers.
if classifier != nil && classifier.HasTypeMatchers() {
    if leadOK {
        if err := client.EnrichIssueTypes(ctx, leadPipeline.Issues); err != nil {
            deps.Warn("issue type enrichment (lead time): %v", err)
        }
    }
    if throughputOK {
        if err := client.EnrichIssueTypes(ctx, throughputPipeline.OpenIssues); err != nil {
            deps.Warn("issue type enrichment (throughput open): %v", err)
        }
    }
}
```

Notes:
- Classifier must be constructed before this point — currently `computeQualityWithInsights` builds it at line 317. Move classifier construction earlier (after config is loaded).
- Cycle-time issues do NOT need enrichment (cycle-time doesn't classify).
- Velocity issues: velocity doesn't classify, but copies IssueType for display. Enrich if velocity pipeline has an accessible issues slice and the cost is minimal.
- Cache deduplication: if lead-time and cycle-time share the same search query (they often do), `SearchIssues` returns cached results — the same issue objects. Enriching once covers both.

### Phase 4: Wire enrichment into standalone commands

#### Standalone WIP (`cmd/wip.go` or wherever WIP is invoked standalone)

After `SearchIssues()`, before pipeline construction:
```go
if classifier.HasTypeMatchers() {
    _ = client.EnrichIssueTypes(ctx, issues)
}
```

WIP uses a narrow `searcher` interface internally, but the cmd layer has `*gh.Client`. Enrichment happens at cmd level before passing issues to the pipeline.

#### Config validate velocity (`cmd/config_validate_velocity.go`)

After `SearchIssues()` at line 156, before the matching loop at line 168:
```go
if hasTypeMatchers(categories) {
    _ = client.EnrichIssueTypes(ctx, issues)
}
```

## Acceptance Criteria

- [ ] `EnrichIssueTypes` batch-fetches only `number + issueType { name }` via GraphQL
- [ ] `HasTypeMatchers()` returns true when any category has a `type:` matcher, false otherwise
- [ ] Report quality correctly classifies issues by type when config has `type:Bug` matcher
- [ ] WIP correctly classifies issues by type in both standalone and report contexts
- [ ] No extra API calls when config has no `type:` matchers (verify with debug logging)
- [ ] Enrichment errors are non-fatal — classification falls through to label/title matchers
- [ ] `leadPipeline.Issues` is exported and enriched before `ProcessData()`
- [ ] `throughputPipeline.OpenIssues` is enriched before WIP pipeline construction
- [ ] Config validate velocity correctly tests `type:` matchers after enrichment
- [ ] Existing tests pass unchanged (no regression)

## Test Plan

### Unit tests

**`internal/github/enrich_test.go`:**
- `TestEnrichIssueTypes_SetsType` — mock GraphQL response, verify issues mutated in-place
- `TestEnrichIssueTypes_SkipsAlreadySet` — issues with IssueType populated are not overwritten
- `TestEnrichIssueTypes_EmptySlice` — returns nil, no API call
- `TestEnrichIssueTypes_PartialBatchFailure` — first batch succeeds, second fails, verify partial results
- `TestEnrichIssueTypes_BatchesCorrectly` — 25 issues → 2 batches (20 + 5)

**`internal/classify/classify_test.go`:**
- `TestHasTypeMatchers_True` — categories with `type:Bug` matcher
- `TestHasTypeMatchers_False` — categories with only `label:` and `title:` matchers
- `TestHasTypeMatchers_Mixed` — some categories have type matchers, some don't

### Integration-style tests

**`cmd/report_test.go` (or appropriate test file):**
- Test that report quality section correctly classifies issues when config has `type:Bug` and issues are REST-sourced (mock client that returns issues without IssueType from search, with IssueType from GraphQL enrichment)

## Scope Boundaries

- Preflight is not affected (already shipped in companion PR)
- Release command is not affected (already uses GraphQL `FetchIssues`)
- Not enriching PRs — `IssueType` is an issue-only field
- Not changing `SearchIssues()` itself — enrichment is opt-in at caller level
- Cycle-time pipeline does not need enrichment (doesn't classify)
- Disk caching of enrichment results is out of scope (in-memory per-process is sufficient)

## Dependencies & Risks

- **GraphQL schema**: `issueType` field must be available on the repository's GitHub instance. `ListIssueTypes` already handles graceful degradation for older instances — `EnrichIssueTypes` should follow the same pattern.
- **API cost**: 1 GraphQL call per 20 issues. For a typical report with 50 closed issues + 20 open issues, this is 4 extra GraphQL calls. Acceptable.
- **Breaking change**: Exporting `leadPipeline.issues` → `Issues` is a non-breaking change (unexported → exported).

## Sources

- **Origin document:** [docs/brainstorms/2026-03-21-issue-type-enrichment-requirements.md](docs/brainstorms/2026-03-21-issue-type-enrichment-requirements.md) — key decisions: lightweight query, classifier-aware gating, per-pipeline at cmd/ layer
- **Batch GraphQL pattern:** `internal/github/batch.go` — aliased query with `gqlIssueNode`
- **Cache infrastructure:** `internal/github/cache.go` — `DoJSON`, `CacheKey`
- **Pipeline architecture:** `docs/solutions/architecture-refactors/pipeline-per-metric-and-preflight-first-config.md`
- **Cache correctness learning:** `docs/solutions/architecture-patterns/label-based-lifecycle-for-cycle-time.md` — N+1 caching mandate

### Key File References

- `internal/github/batch.go:40-109` — existing batch pattern to follow
- `internal/github/cache.go` — `DoJSON`, `CacheKey` for caching enrichment
- `internal/github/search.go:16-42` — `searchItemToIssue` (the gap: no IssueType)
- `internal/classify/classify.go:29-50` — `Classifier` struct, `matchers` field
- `internal/pipeline/leadtime/leadtime.go:85-115` — `BulkPipeline` with unexported `issues`
- `internal/pipeline/throughput/throughput.go:41` — `OpenIssues` (already exported)
- `cmd/report.go:255-346` — `g.Wait()` through WIP construction (enrichment window)
- `cmd/report.go:656-674` — `computeQualityWithInsights` (classification site)
- `cmd/config_validate_velocity.go:156-170` — validation enrichment site
