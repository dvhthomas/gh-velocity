---
title: "Interpreting Results"
weight: 1
---

# Interpreting Results

gh-velocity produces output in four formats: pretty (default), JSON, markdown, and HTML. Each format contains the same data, structured for different consumers. This guide explains how to read the output, what healthy metrics look like, and what common patterns mean.

## Reading your output

### Output formats at a glance

| Format | Best for | Flag |
|--------|----------|------|
| Pretty | Terminal reading, quick checks | `--results pretty` (default) |
| JSON | Agents, scripts, `jq` pipelines, CI | `--results json` |
| Markdown | Pasting into issues, PRs, Discussions | `--results markdown` |
| HTML | Self-contained report files | `--results html` |

All commands accept `--results` (or `-r`). If you omit it, you get pretty.

### Three metric states

Every timing metric (lead time, cycle time) can be in one of three states:

| State | Pretty output | JSON | Meaning |
|-------|--------------|------|---------|
| **Completed** | `2d 4h` | `"duration_seconds": 187200` | Work started and finished |
| **In progress** | `in progress` | `"started_at": "...", "duration_seconds": null` | Work started, clock still running |
| **N/A** | `N/A` | `"started_at": null, "duration_seconds": null` | No start signal found |

N/A usually means your cycle time strategy has no signal for that issue. Possible causes:

- **No `lifecycle.in-progress.match` configured** — no label to look for, so every issue is N/A
- **Configured but no matching label event found** — the label exists in config but was never applied to this issue
- **Issue is in backlog** — matches a backlog label, excluded from cycle time
- **Negative cycle time was filtered** — start after close, discarded as invalid

See [Cycle Time Setup]({{< relref "cycle-time-setup" >}}) to fix this.

### Output layers

Every command produces up to four layers of output:

| Layer | What it contains | Example |
|-------|-----------------|---------|
| **Stats** | Aggregate numbers (mean, median, P90, count) | Lead time median: 5d |
| **Detail** | Per-item breakdowns | Issue #42: 2d cycle time |
| **Insights** | Human-readable observations with severity | "Low classification coverage: 60% are unclassified" |
| **Provenance** | How the output was produced (command, config, scope) | Reproducibility metadata |

The `report` command shows stats only (one line per metric). Standalone commands like `flow cycle-time` and `quality release` show all four layers. In JSON, each layer is a top-level key.

### Reading pretty output

Pretty output is designed for a terminal. Here is an example from `quality release`:

```
Release  v1.2.0  owner/repo
Previous tag: v1.1.0 (14d ago)
Cadence: 14 days
Hotfix: no

Composition (12 issues):
  bug: 3 (25%)  feature: 7 (58%)  chore: 1 (8%)  other: 1 (8%)
  Bug ratio: 25%

  #   Title                          Lead     Cycle    Lag   Category
  ─── ────────────────────────────── ──────── ──────── ───── ────────
  42  Fix auth timeout               2d       1d       0d    bug
  38  Add dark mode                  45d      5d       3d    feature
  51  Upgrade deps                   120d     1d       0d    chore      OUTLIER
  ...

Aggregates:
  Lead time   — median: 8d, mean: 22d, P90: 45d, P95: 120d, stddev: 35d
  Cycle time  — median: 2d, mean: 3d, P90: 5d, P95: 5d
  Release lag — median: 1d, mean: 2d
  Outliers: 1 (threshold: 98d)
```

Key elements:

- **Per-issue rows** -- individual metrics per issue. Statistical outliers are marked `OUTLIER`.
- **Aggregates** -- distribution summary. Median is the primary number; mean is shown for comparison.
- **Composition** -- category breakdown and bug ratio.
- **Cadence** and **hotfix** -- release rhythm.

### Reading JSON output

JSON is the richest format -- every pretty-output field is present, plus additional fields for programmatic use.

```bash
gh velocity quality release v1.2.0 --results json | jq '.aggregates.lead_time'
```

```json
{
  "count": 12,
  "mean_seconds": 1900800,
  "median_seconds": 691200,
  "stddev_seconds": 3024000,
  "p90_seconds": 3888000,
  "p95_seconds": 10368000,
  "outlier_cutoff_seconds": 8467200,
  "outlier_count": 1
}
```

Durations are always in seconds. Divide by 86400 to get days. Ratios are floats between 0 and 1. Booleans flag outlier status per issue:

```bash
gh velocity quality release v1.2.0 --results json | \
  jq '[.issues[] | select(.lead_time_outlier) | {number, title, lead_time_seconds}]'
```

In JSON mode, errors appear as structured `ErrorEnvelope` objects on stderr. See [Agent Integration]({{< relref "agent-integration" >}}) for parsing details.

### Reading markdown output

Markdown output is ready to paste into GitHub Issues, PRs, or Discussions:

```bash
gh velocity quality release v1.2.0 --results markdown
```

Use with `--post` to have gh-velocity post it automatically, or pipe into `gh issue comment`:

```bash
gh velocity quality release v1.2.0 --results markdown | \
  gh issue comment 100 --body-file -
```

## What healthy metrics look like

There are no universal benchmarks -- what matters is your trend over time and whether the numbers match your team's experience. These patterns indicate healthy delivery:

### [Lead time]({{< relref "/reference/metrics/lead-time" >}})

- **Median under 30 days** for most product teams -- a typical issue goes from creation to close within a month.
- **P95 under 90 days**. A P95 over 90 days means old issues are being closed alongside new work -- normal, but worth understanding.
- **Predictability label is not "low"**. Derived from the coefficient of variation (CV = stddev / mean). "Low" means individual item durations are hard to predict. See [Understanding Statistics]({{< relref "/concepts/statistics#standard-deviation-and-predictability-cv" >}}).

### [Cycle time]({{< relref "/reference/metrics/cycle-time" >}})

- **Median under 7 days** means active work on a typical issue completes within a week.
- **Cycle time much shorter than lead time** is normal. The gap is backlog time (waiting before work starts).
- **N/A cycle times** mean the configured strategy has no signal for those issues. See [Cycle Time Setup]({{< relref "cycle-time-setup" >}}).

### [Release lag]({{< relref "/reference/metrics/quality" >}})

- **Median under 7 days** means completed work reaches users within a week.
- **High release lag with low cadence** means completed work sits waiting for a release. Consider releasing more frequently.

### [Composition and bug ratio]({{< relref "/reference/metrics/quality" >}})

- **Bug ratio under 30%** is typical for product-focused teams.
- **High "other" count** means issues lack classification labels. Run `gh velocity config preflight` to generate category matchers, or label issues before releasing.

### [Velocity]({{< relref "/reference/metrics/velocity" >}})

- **Stable velocity across iterations** indicates predictable delivery. Look at the trend column in the history table.
- **Completion rate above 80%** means the team commits to a realistic amount of work per iteration.
- **High standard deviation** in velocity across iterations suggests scope changes or inconsistent estimation.

### [Throughput]({{< relref "/reference/metrics/throughput" >}})

- **Steady or growing item count** means consistent output. A sudden drop may indicate blockers or context switching.

## Common patterns and what they mean

### Large gap between mean and median lead time

Your data is right-skewed -- a few old issues are pulling the mean up. The median is the better measure of "typical." See [Understanding Statistics]({{< relref "/concepts/statistics" >}}).

Example from a real repo:
- Mean lead time: 280 days
- Median lead time: 60 days

Two issues open for 4+ years were closed in the release. The median tells you the typical issue takes about 2 months.

### Many outliers in one release

Outliers are flagged using the IQR method. Multiple outliers in a single release often indicate a backlog cleanup alongside normal work. Check whether they are old issues finally closed or genuinely slow work.

### Cycle time is N/A for most issues

> [!TIP]
> N/A means "no start signal found" — not "zero." Check your strategy configuration.

Common causes:

- **Issue strategy**: No `lifecycle.in-progress.match` configured, or labels are not applied to issues. Fix by adding lifecycle labels.
- **PR strategy**: PRs do not reference issues with "Closes #N" or "Fixes #N". Fix by adding closing keywords to PR descriptions.

See [Troubleshooting]({{< relref "troubleshooting" >}}#cycle-time-shows-na-for-all-issues) for detailed resolution steps.

### High release lag despite fast cycle time

Work finishes quickly but waits before release. This typically indicates a batch release process. Release more often, or automate releases on merge.

### Bug ratio of 0%

Either there are genuinely no bugs, or your bug category matchers are misconfigured. Check that your config has a category named `bug` with appropriate `match` rules. See [Troubleshooting]({{< relref "troubleshooting" >}}#defect-rate-shows-0).

### "Not assessed" items in velocity output

Items matching no effort query are excluded from velocity and committed totals, reported separately. A high not-assessed count means your effort matchers need tuning. Run `gh velocity config validate --velocity` to see unmatched issues.

## Why noise exclusion matters

Repos with spam, duplicate, or invalid issues produce misleading metrics. The `preflight` command detects these labels and adds `-label:` exclusions to the scope query automatically. Example from **cli/cli** (30-day window):

| Metric | Before (no exclusions) | After (noise excluded) |
|--------|----------------------|----------------------|
| Issues closed | 112 | 57 |
| Lead time median | 8 minutes | 35 days |
| Bug ratio | 25% | 35% |
| Bug fix speed | "slower than other work" | "faster than other work" |
| Hotfix count | 72 | 17 |
| Predictability (CV) | 2.7 | 1.8 |

Half the closed issues were spam or duplicates closed in under 60 seconds. These instant closures dragged the median lead time to 8 minutes, made bug fixes look slower than "other work" (which was mostly spam), and inflated the hotfix count by counting every instant closure as a "hotfix."

After excluding noise labels, every metric became meaningful: lead time reflects actual delivery time, bug ratio reflects real workload composition, and the insight about bug fix speed reversed — bugs are actually resolved faster than features.

### Checking your scope for noise

Run `preflight` to regenerate your config and inspect the scope:

```bash
gh velocity config preflight -R owner/repo --write
```

If noise labels are detected, the generated config will include exclusions:

```yaml
scope:
  query: "repo:cli/cli -label:duplicate -label:invalid -label:suspected-spam"
  # Excluded 3 noise label(s) detected in this repo: duplicate, invalid, suspected-spam
```

Detected patterns: `spam`, `duplicate`, and `invalid` (matched at word boundaries). For different noise labels, add manual exclusions to the scope query.

## Comparing across releases

To spot trends, run the same command for multiple releases and compare:

```bash
gh velocity quality release v2.0.0 --results json > v2.json
gh velocity quality release v1.9.0 --results json > v1.json

echo "v1.9.0 median: $(jq -r '.aggregates.lead_time.median_seconds / 86400 | round | "\(.)d"' v1.json)"
echo "v2.0.0 median: $(jq -r '.aggregates.lead_time.median_seconds / 86400 | round | "\(.)d"' v2.json)"
```

For automated trend tracking, see the [CI Setup: Scheduled trend reports]({{< relref "/getting-started/ci-setup" >}}#scheduled-trend-reports) pattern.

## Next steps

- [Understanding Statistics]({{< relref "/concepts/statistics" >}}) -- deep dive on median, percentiles, IQR, and standard deviation
- [Ad Hoc Queries]({{< relref "ad-hoc-queries" >}}) -- common jq patterns for extracting specific data
- [Agent Integration]({{< relref "agent-integration" >}}) -- using JSON output with LLM agents and scripts
