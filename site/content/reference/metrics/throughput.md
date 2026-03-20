---
title: "Throughput"
weight: 4
---

# Throughput

Throughput counts issues closed and PRs merged within a time window -- the simplest measure of delivery rate.

## Definition

```
throughput_issues = count(issues closed in window)
throughput_prs    = count(PRs merged in window)
```

Unweighted: every closed issue and every merged PR counts as 1, regardless of size, complexity, or effort.

## Throughput vs. velocity

Throughput and [velocity]({{< relref "velocity" >}}) both measure output, but they differ in two ways:

| Dimension | Throughput | Velocity |
|-----------|-----------|----------|
| **Weighting** | Every item = 1 | Items weighted by effort strategy (count, attribute, or numeric) |
| **Time boundary** | Arbitrary date range (`--since` / `--until`) | Iteration-aligned (sprint boundaries) |

Throughput is useful without sprints or effort estimates. Velocity is useful for tracking effort-weighted output across consistent iteration boundaries.

When velocity uses `effort.strategy: count`, velocity for a completed iteration equals throughput for the same window. They diverge when effort weighting or iteration alignment matters.

## What it tells you

- **Delivery rate**: How many items your team finishes per week/month.
- **Trend direction**: Is output increasing, decreasing, or stable over time?
- **Bottleneck signal**: A drop in throughput with stable team size suggests process friction.

Throughput does not distinguish a one-line typo fix from a multi-week refactor. Use alongside lead time and cycle time for a complete picture.

## Signals used

| Signal | Source | Description |
|--------|--------|-------------|
| Issue closed | `issue.closed_at` | Issue closure timestamp within the time window |
| PR merged | `pr.merged_at` | PR merge timestamp within the time window |

Both come from the GitHub REST Search API.

## Configuration that affects throughput

| Config field | Effect |
|---|---|
| `scope.query` | Filters which issues/PRs are counted |
| `exclude_users` | Excludes items authored by specified users (e.g., bots) |
| `lifecycle.done.query` | Search qualifiers for finding closed issues |

Throughput does not use effort strategies, iteration boundaries, or classification categories.

## Example output

Throughput appears in the `report` command output:

```
Activity (last 30 days)
  Issues closed: 12
  PRs merged: 18
  Releases: 2
```

### JSON

```json
{
  "issues_closed": 12,
  "prs_merged": 18,
  "releases": 2
}
```

## Commands that report throughput

- `gh velocity report --since 30d` -- activity summary including throughput counts
- `gh velocity flow lead-time --since 30d` -- closed issue count as context

## Insights

| Insight | When it fires | What it means |
|---------|--------------|---------------|
| **Zero activity** | 0 issues closed and 0 PRs merged | No work completed in this window. Check your date range and scope. |
| **Issue/PR mismatch** | PRs merged but 0 issues closed, or PR:issue ratio > 3:1 | PRs may not be linked to issues. This affects cycle time measurement and issue-based metrics. |

## See also

- [Velocity]({{< relref "/reference/metrics/velocity" >}}) -- effort-weighted, iteration-aligned output metric
- [Configuration Reference]({{< relref "/reference/config" >}}) -- scope and exclude_users fields that affect throughput
