---
date: 2026-03-20
topic: wip-detail-report
---

# WIP Detail Report

## Problem Frame

The WIP command (`gh velocity status wip`) exists but is incomplete: it only fetches issues, makes its own API calls, and produces a flat item list without summary analytics. It answers "what's in progress" but not "how much is in progress" or "who is working on what." It's also disconnected from the report dashboard, which has a `TODO` placeholder at `report.go:304`.

Teams need WIP to answer two fundamental questions:
1. **How many people are working on things** (assigned capacity)
2. **How many things are in progress** (work volume by stage and health)

Additionally, effort configuration currently lives under `velocity.effort`, but effort is a cross-cutting concept needed by both velocity and WIP (for effort-weighted limits). It should be promoted to top-level config.

## Requirements

### Data Layer

- R1. **Throughput as shared data layer** — Throughput pipeline retains full item data (not just counts) for both issues and PRs, including assignee information. WIP consumes throughput's items rather than re-fetching. The Issue and PR models gain `Assignees []string` fields populated from the Search API response.

- R2. **Throughput fetches open items** — Throughput broadens its scope to also fetch open issues and open PRs (in addition to closed/merged). Open items are available for WIP consumption. Throughput's own counts (issues closed, PRs merged) remain unchanged.

### Config Restructure

- R3. **Promote effort to top-level config** — Move `velocity.effort` to top-level `effort` in `.gh-velocity.yml`. The effort config (strategy, attribute matchers, numeric field) becomes a shared concept consumed by both velocity and WIP. `velocity.effort` becomes a deprecated alias that is read if top-level `effort` is absent, with a warning directing users to migrate.

### WIP Core

- R4. **WIP includes issues and PRs** — WIP operates on the open items from throughput's dataset. Both issues and PRs are classified into lifecycle stages using the existing `lifecycle.in-progress.match` and `lifecycle.in-review.match` config matchers. PRs without matching labels use PR-native signals as fallback: draft = In Progress, non-draft open = In Review.

- R5. **Lifecycle stage counts with matcher granularity** — WIP shows counts grouped by lifecycle stage (In Progress, In Review), with per-matcher sub-counts within each stage (e.g., "In Progress: 5 — Design: 3, Architecture Review: 2 | In Review: 4"). Matchers may be labels, issue types, title patterns, or field values.

- R6. **Top assignees table** — WIP includes a "top N assignees" section showing users ranked by assigned open items, with a per-user breakdown by lifecycle stage. This serves as a capacity signal (who has the most work in flight).

- R7. **Staleness breakdown** — WIP shows counts by staleness level (ACTIVE / AGING / STALE) using existing thresholds (3d / 7d). This surfaces stuck work at the aggregate level.

### WIP Warnings

- R8. **Team WIP warning** — When total WIP (weighted by effort strategy) exceeds a configured `wip.team_limit`, generate an insight/warning (e.g., "Team WIP is 62 effort-points, exceeding limit of 50"). Uses the shared top-level `effort` config to weight items. When effort strategy is `count`, limit is simply item count. No warning generated when `wip.team_limit` is not configured.

- R9. **Individual WIP warning** — When any assignee's total WIP (weighted by effort strategy) exceeds a configured `wip.person_limit`, flag that person in the assignees table and generate an insight (e.g., "Alice has 12 effort-points assigned, exceeding per-person limit of 8"). No warning when `wip.person_limit` is not configured.

### Insights

- R10. **Auto-generated insights** — WIP generates human-readable observations following the existing `model.Insight` pattern. Includes: assignee load ("Alice has 8 items, highest in team"), staleness ("40% of WIP is stale"), capacity ("12 items in progress across 4 people"), stage health ("No items in Design stage are stale"), and WIP limit warnings (R8, R9).

### Report Integration

- R11. **Report summary row** — In the report dashboard summary table, WIP shows: total open item count + stale count (e.g., "WIP: 14 items (3 stale)"). Detail goes in the collapsible section.

- R12. **Report detail section** — WIP appears as a collapsible detail section in the report (like lead time, cycle time, etc.) containing the per-item table, lifecycle stage counts, top assignees, staleness breakdown, WIP limit warnings, and insights.

### Output

- R13. **Standalone command updated** — `gh velocity status wip` gains the same enrichments (assignee table, stage counts, staleness breakdown, insights, WIP warnings) but fetches its own data since it runs outside the report context.

- R14. **All three output formats** — JSON, Markdown, and Pretty output for all WIP content, following existing format conventions. JSON is self-contained (no stderr for errors/warnings when `--format json`).

## Success Criteria

- `gh velocity report` includes a WIP section with items, stage counts, assignee table, staleness, WIP warnings, and insights
- `gh velocity status wip` shows the same enriched output standalone
- No additional API calls for WIP in the report beyond what throughput already makes (WIP reuses throughput's retained items)
- Effort config works from top-level `effort` key; `velocity.effort` still works with a deprecation warning
- WIP warnings fire correctly when team or individual limits are exceeded (effort-weighted)
- `config preflight --write`, `config show`, examples, and `defaultConfigTemplate` reflect the new top-level `effort` and `wip` config sections

## Scope Boundaries

- **Not included:** Backlog or Done stage items — WIP only shows items determined to be "in progress" (in-progress + in-review lifecycle stages). Open issues with no matching lifecycle labels are excluded from WIP entirely.
- **Not included:** Cross-repo WIP aggregation — single repo scope only
- **Not included:** Configurable staleness thresholds — uses existing hardcoded 3d/7d
- **Not included:** Per-stage WIP limits (only team-level and person-level)

## Key Decisions

- **Throughput as data layer:** Throughput retains items and adds open-item queries so WIP doesn't re-fetch. This is a refactor of throughput's internal storage, not a new pipeline.
- **Effort promoted to top-level:** Effort is a shared concept, not velocity-specific. Top-level `effort` config consumed by both velocity and WIP. Deprecated alias from `velocity.effort` for backwards compatibility.
- **WIP limits use effort weighting:** Limits are expressed in effort units (whatever the effort strategy defines). When strategy is `count`, this naturally equals item count. When strategy is `attribute` or `numeric`, limits are effort-weighted.
- **Warnings, not hard blocks:** Exceeding WIP limits generates insights/warnings, not errors. The tool informs, it doesn't enforce.
- **Matcher granularity in stage counts:** Individual matchers within a lifecycle stage are shown separately (e.g., "Design: 3") rather than collapsed into "In Progress: 3". Gives visibility into workflow sub-stages.
- **PR fallback to native signals:** When PRs lack lifecycle labels, draft status and open status serve as implicit stage classification. Label config takes precedence when present.
- **Report summary is minimal:** "14 items (3 stale)" — detail lives in the collapsible section.

## Dependencies / Assumptions

- Issue and PR models need `Assignees []string` field added, populated from Search API response (the `assignees` array is already in the REST response, just not parsed)
- Throughput pipeline's `GatherData` will make 2 additional queries (open issues, open PRs) in the report context — acceptable API cost since the client cache and rate limiting are shared
- PR model needs a `Draft bool` field for native signal classification (already available in Search API response)
- The effort strategy evaluation logic in the velocity pipeline needs to be extractable as a shared function

## Outstanding Questions

### Deferred to Planning

- [Affects R2][Needs research] **How open items are bounded** — Open items have no date window (unlike `closed:>date`). Must decide: label-filtered queries (like current wip command, one query per lifecycle label), broad fetch with client-side filter (bounded by scope/updated date), or hybrid. Consider Search API's 1000-result cap, typical repo sizes, and API cost. This is the highest-priority planning question.
- [Affects R1][Technical] How to structure throughput's retained items — flat slice vs. partitioned by state (open/closed) for efficient WIP slicing
- [Affects R2][Needs research] Whether the Search API's `is:open` query can share pagination/caching with the existing `is:closed` query or needs separate cache keys
- [Affects R3][Technical] Migration path for `velocity.effort` → top-level `effort` in config parsing — likely read both locations, prefer top-level, warn on old location
- [Affects R6][Technical] Whether "top N" should be configurable or hardcoded to 10
- [Affects R8-R9][Needs research] How the existing effort evaluation logic (attribute matchers, numeric field lookup) is currently structured and how to extract it as a shared function for WIP to call. Note: `numeric` strategy requires project board field values, which may require additional API calls for open items — this could conflict with the "no additional API calls" goal and may need special handling (e.g., fall back to count-based for WIP when numeric data isn't already cached)
- [Affects R13][Technical] How standalone WIP command reuses the same computation logic without the throughput pipeline — likely a shared function that accepts items

## Next Steps

→ `/ce:plan` for structured implementation planning
