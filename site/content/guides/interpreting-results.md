---
title: "Interpreting Results"
weight: 1
---

# Interpreting Results

gh-velocity produces output in three formats: pretty (default), JSON, and markdown. Each format contains the same data, structured for different consumers. This guide explains how to read the output, what healthy metrics look like, and what common patterns mean.

## Output formats at a glance

| Format | Best for | Flag |
|--------|----------|------|
| Pretty | Terminal reading, quick checks | `--format pretty` (default) |
| JSON | Agents, scripts, `jq` pipelines, CI | `--format json` |
| Markdown | Pasting into issues, PRs, Discussions | `--format markdown` |

All commands accept `--format` (or `-f`). If you omit it, you get pretty.

## Reading pretty output

Pretty output is designed for a terminal. Here is an example from `quality release`:

```
Release  v1.2.0  owner/repo
Previous tag: v1.1.0 (14d ago)
Cadence: 14 days
Hotfix: no

Composition (12 issues):
  bug: 3 (25%)  feature: 7 (58%)  chore: 1 (8%)  other: 1 (8%)
  Defect rate: 25%

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

Key things to notice:

- **Per-issue rows** show each issue with its individual metrics. Issues flagged as statistical outliers are marked with `OUTLIER`.
- **Aggregates** at the bottom summarize the distribution. Median is the primary number to watch; mean is shown for comparison.
- **Composition** shows category breakdown and defect rate.
- **Cadence** and **hotfix** describe the release rhythm.

## Reading JSON output

JSON is the richest format. Every field that appears in pretty output is present in JSON, plus additional fields for programmatic use.

```bash
gh velocity quality release v1.2.0 --format json | jq '.aggregates.lead_time'
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
gh velocity quality release v1.2.0 --format json | \
  jq '[.issues[] | select(.lead_time_outlier) | {number, title, lead_time_seconds}]'
```

When errors occur in JSON mode, they appear as structured `ErrorEnvelope` objects on stderr, not as plain text. See [Agent Integration]({{< relref "agent-integration" >}}) for details on parsing JSON errors.

## Reading markdown output

Markdown is designed for pasting into GitHub Issues, PRs, or Discussions:

```bash
gh velocity quality release v1.2.0 --format markdown
```

The output uses GitHub-flavored markdown tables and is ready to paste directly. Use it with `--post` to have gh-velocity post it for you, or pipe it into `gh issue comment`:

```bash
gh velocity quality release v1.2.0 --format markdown | \
  gh issue comment 100 --body-file -
```

## What "good" looks like

There are no universal benchmarks. What matters is your trend over time and whether the numbers match your team's experience. That said, here are patterns that indicate healthy delivery:

### Lead time

- **Median under 30 days** for most product teams. This means a typical issue goes from creation to close within a month.
- **P95 under 90 days**. If your P95 is over 90 days, old issues are being closed alongside new work, which is normal but worth understanding.
- **Low stddev relative to the mean**. If `stddev / mean > 1`, delivery times are highly variable. Consistent teams have lower relative standard deviation.

### Cycle time

- **Median under 7 days** means active work on a typical issue completes within a week.
- **Cycle time much shorter than lead time** is normal and expected. The gap represents backlog time (waiting before work starts).
- **N/A cycle times** mean the configured strategy has no signal for those issues. See [Cycle Time Setup]({{< relref "cycle-time-setup" >}}) to fix this.

### Release lag

- **Median under 7 days** means completed work reaches users within a week.
- **High release lag with low cadence** means work is done but sits waiting for a release. Consider releasing more frequently.

### Composition and defect rate

- **Defect rate under 30%** is typical for product-focused teams.
- **High "other" count** means your issues lack classification labels. Run `gh velocity config preflight` to generate category matchers, or label issues before releasing. See [Recipes: Check label coverage]({{< relref "recipes" >}}#check-label-coverage-before-a-release).

### Velocity

- **Stable velocity across iterations** indicates predictable delivery. Look at the trend column in the history table.
- **Completion rate above 80%** means the team commits to a realistic amount of work per iteration.
- **High standard deviation** in velocity across iterations suggests scope changes or inconsistent estimation.

### Throughput

- **Steady or growing item count** over time means consistent output. A sudden drop may indicate blockers or context switching.

## Common patterns and what they mean

### Large gap between mean and median lead time

Your data is right-skewed. A few old issues closed in this release are pulling the mean up. The median is the better measure of "typical." See [Understanding Statistics]({{< relref "/concepts/statistics" >}}) for why.

Example from a real repo:
- Mean lead time: 280 days
- Median lead time: 60 days

Two issues open for 4+ years were closed in the release. The median tells you the typical issue takes about 2 months.

### Many outliers in one release

Outliers are flagged using the IQR method. Multiple outliers in a single release often mean a backlog cleanup happened alongside normal work. Check the outlier issues to see if they are old issues finally closed or genuinely slow work.

### Cycle time is N/A for most issues

The configured strategy does not have a signal for those issues. Common causes:

- **Issue strategy**: No `lifecycle.in-progress.match` configured, or labels are not applied to issues. Fix by adding lifecycle labels.
- **PR strategy**: PRs do not reference issues with "Closes #N" or "Fixes #N". Fix by adding closing keywords to PR descriptions.

See [Troubleshooting]({{< relref "troubleshooting" >}}#cycle-time-shows-na-for-all-issues) for detailed resolution steps.

### High release lag despite fast cycle time

Work finishes quickly but waits before being released. This typically indicates a batch release process. The fix is organizational: release more often, or automate releases on merge.

### Defect rate of 0%

Either there are genuinely no bugs in this release, or your bug category matchers are not configured correctly. Check that your config has a category named `bug` with appropriate `match` rules. See [Troubleshooting]({{< relref "troubleshooting" >}}#defect-rate-shows-0).

### "Not assessed" items in velocity output

Items that do not match any effort query are excluded from velocity and committed totals and reported separately. A high not-assessed count means your effort matchers need tuning. Run `gh velocity config validate --velocity` to see which issues are unmatched.

## Comparing across releases

To spot trends, run the same command for multiple releases and compare:

```bash
gh velocity quality release v2.0.0 --format json > v2.json
gh velocity quality release v1.9.0 --format json > v1.json

echo "v1.9.0 median: $(jq -r '.aggregates.lead_time.median_seconds / 86400 | round | "\(.)d"' v1.json)"
echo "v2.0.0 median: $(jq -r '.aggregates.lead_time.median_seconds / 86400 | round | "\(.)d"' v2.json)"
```

For automated trend tracking, see the [CI Setup: Scheduled trend reports]({{< relref "/getting-started/ci-setup" >}}#scheduled-trend-reports) pattern.

## Next steps

- [Understanding Statistics]({{< relref "/concepts/statistics" >}}) -- deep dive on median, percentiles, IQR, and standard deviation
- [Recipes]({{< relref "recipes" >}}) -- common jq patterns for extracting specific data
- [Agent Integration]({{< relref "agent-integration" >}}) -- using JSON output with LLM agents and scripts
