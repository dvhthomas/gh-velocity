---
title: "Insights for All Report Sections"
date: 2026-03-17
status: active
type: brainstorm
origin: https://github.com/dvhthomas/gh-velocity/issues/82
---

# Brainstorm: Insights for All Report Sections

## What We're Building

Every report section and every standalone command should lead with human-readable insights — the *interpretation* of the metrics, not just the numbers. Today only the velocity section generates `[]model.Insight`. Lead time, cycle time, throughput, and quality sections show raw stats and per-item tables with no commentary.

The current grafana/k6 showcase output ([Discussion #83](https://github.com/dvhthomas/gh-velocity/discussions/83#discussioncomment-16171967)) illustrates the problem: "median 43d 20h, mean 203d 3h, P90 446d 20h (n=21, 4 outliers)" tells you numbers but not *what they mean*. An insight like "4 outliers >447d dominate the mean — consider excluding ancient browser bugs" is actionable.

### Goals

- Insights are the headline of every section; per-item tables are supporting evidence
- Every pipeline generates its own insights using the data it already has
- Report-level "Key Findings" section collects all insights, grouped by metric
- Individual commands (`flow lead-time`, `flow cycle-time`, etc.) also show insights
- Comprehensive insight rules from day one — outliers, skew, thresholds, trends, category breakdowns

## Why This Approach

### Insights-first layout

The report summary currently leads with a metrics table. The new layout puts a **Key Findings** section first — a grouped bullet list of all insights across sections. The metrics table follows as the quantitative summary. Detail sections (inside `<details>` blocks) also lead with section-specific insights before the per-item table.

This mirrors how a human analyst would present findings: "here's what matters" → "here are the numbers" → "here's the evidence."

### Per-pipeline generation

Each pipeline generates its own `[]model.Insight` during `ProcessData()`, following the pattern velocity already established. This avoids a central engine that would need to understand every metric type. Each pipeline has the richest context about its own data — outlier counts, percentile distributions, category breakdowns — and can generate insights directly.

The report command collects insights from all pipelines and groups them by section for the Key Findings block. Standalone commands render their pipeline's insights directly.

### Comprehensive rules

Rather than starting with 2-3 rules per section and iterating, build the full insight rule set from the start. The rules are simple conditionals on data already computed (stats, outliers, counts, categories). The cost of adding a rule is low (a few lines each), and sparse insights ("no cycle time data") are as important as rich ones.

## Key Decisions

1. **Insights first, then metrics table** — Key Findings section leads the report. Detail sections also lead with insights.
2. **Per-pipeline architecture** — Each pipeline owns its insight generation. Report collects and groups. No central engine.
3. **All commands, not just report** — Standalone commands show their pipeline's insights too.
4. **All insights shown, grouped by section** — No curation/ranking at report level. Every insight from every section appears, grouped by metric name.
5. **Comprehensive rule set** — Build all rules in one pass rather than iterating.

## Insight Rules by Section

### Lead Time
- **Outlier detection**: N outliers above P90 threshold — name the threshold, suggest action
- **Skew warning**: When mean >> median (e.g., >3x), flag heavy right skew and explain what's causing it
- **Percentile distribution**: "80% of issues resolved in <Xd" when P80 is dramatically lower than max
- **Fastest/slowest callout**: Name the fastest and slowest items with context (title, time)
- **Trend vs prior period**: If prior-period data available, compare median/mean shift
- **Per-category breakdown**: When categories are configured, compare lead time across bug/feature/chore

### Cycle Time
- **All lead-time rules apply** (outliers, skew, percentiles, fastest/slowest, trend, categories)
- **Strategy-specific guidance**: "No cycle time data — configure lifecycle.in-progress" or "PR cycle time median 2h — fast review turnaround"
- **Bottleneck detection**: When cycle time >> lead time difference is notable, flag the gap

### Throughput
- **Issue/PR mismatch**: "57 PRs merged but 0 issues closed — PRs may not be linked to issues"
- **Zero activity warning**: No items in the window
- **Trend vs prior period**: Throughput up/down percentage
- **Per-category breakdown**: Distribution across bug/feature/chore/docs/other

### Velocity
- Already implemented: not-assessed items, high completion, zero velocity, high variability
- No changes needed

### Quality
- **Defect rate threshold**: Flag when above typical 15-20% range
- **Bug fix speed comparison**: "Bug fix lead time (median 8m) much faster than features (median 3d)"
- **Category distribution**: Breakdown of what's being shipped (e.g., "60% features, 30% bugs, 10% chore")
- **Hotfix detection**: Items resolved within `hotfix_window_hours` of creation

## Report Layout (Markdown)

```markdown
## Report: owner/repo (date range)

**Key Findings:**

*Lead Time:*
- Median 44d but 4 outliers >447d dominate the mean
- 80% resolved in <50d; long tail of ancient browser bugs

*Cycle Time:*
- No cycle time data — configure lifecycle.in-progress

*Throughput:*
- 23 PRs merged but only 21 issues closed — check PR-issue linking

*Velocity:*
- 87% completion, 13 items/sprint

*Quality:*
- 43% defect rate — well above typical 15-20% threshold

| Metric | Value |
| --- | --- |
| Lead Time | median 43d 20h, mean 203d 3h, P90 446d 20h (n=21, 4 outliers) |
| Cycle Time | no data |
| Throughput | 21 issues closed, 23 PRs merged |
| Velocity | 13.0 items/sprint avg, 87% completion (n=2) |
| Quality | 9 bugs / 21 issues (43% defect rate) |

<details>
<summary>Lead Time (21 issues)</summary>

**Insights:**
- Median 44d but mean 203d — heavy right skew from 4 outliers
- 80% of issues resolved in <50d; long tail of stale browser bugs
- Fastest: gRPC reflection fix #5712 (1h 11m)

| Issue | Title | Labels | Created | Closed | Lead Time |
| --- | --- | --- | --- | --- | --- |
| ... | ... | ... | ... | ... | ... |

**Summary:** median 43d 20h, mean 203d 3h, P90 446d 20h (n=21, 4 outliers)
</details>

<!-- other detail sections follow same pattern -->
```

## Standalone Command Layout (Markdown)

```markdown
## Lead Time: owner/repo (date range)

**Insights:**
- Median 44d but 4 outliers >447d dominate the mean
- 80% resolved in <50d; long tail of ancient browser bugs
- Fastest: gRPC reflection fix #5712 (1h 11m)

| Issue | Title | Labels | Created | Closed | Lead Time |
| --- | --- | --- | --- | --- | --- |
| ... | ... | ... | ... | ... | ... |

**Summary:** median 43d 20h, mean 203d 3h ...
```

## Data Requirements

All insight rules use data that pipelines already compute:
- `model.Stats` has Median, Mean, P90, StdDev, Count, Outliers, OutlierThreshold
- `model.StatsThroughput` has IssuesClosed, PRsMerged
- `model.StatsQuality` has BugCount, TotalCount, DefectRate
- `model.VelocityResult` already has Insights
- Category classification is already applied to items

New data needed:
- **Per-category stats**: Lead time / cycle time broken down by category. Requires grouping items by their classification before computing stats.
- **Trend comparison**: Requires fetching data for a prior period of equal length. This is expensive (doubles API calls) — may want to defer or make opt-in.

## Design Constraints

**No AI at runtime.** All insight rules are deterministic heuristics — simple conditionals on computed stats (e.g., "mean > 3x median → flag skew", "defect rate > 20% → flag high"). No LLM calls, no ML models. Users who want deeper analysis can export JSON/markdown and feed it to an agent themselves.

## Resolved Questions

1. **Trend comparison cost**: Opt-in via `--compare-prior` flag. Only fetches prior period data when explicitly requested. Off by default to avoid doubling API calls.
2. **JSON format**: Per-section — each section has its own `insights` array (e.g., `lead_time.insights: [...]`). Matches the per-pipeline architecture.
3. **Pretty-text format**: Use the existing `→` bullet format from velocity's `WriteInsightsPretty`.
