---
title: "Velocity"
weight: 3
---

# Velocity

Velocity measures effort completed per iteration (sprint). It answers "how much work did we finish in this cycle?" and tracks the answer over time to reveal trends and consistency.

## Definition

```
velocity = sum(effort) for items completed in the iteration
completion_rate = velocity / committed_effort * 100
```

- **Velocity** is the total effort of items that are "done" within an iteration.
- **Committed effort** is the total effort of all items assigned to the iteration (done or not).
- **Completion rate** is the percentage of committed effort that was actually completed.

An item is "done" when:
- **Issues** (`velocity.unit: issues`): Closed with `stateReason: completed` (not "not planned").
- **PRs** (`velocity.unit: prs`): Merged.

## Effort strategies

Effort is how much weight each work item contributes to velocity. Three strategies are available.

### Count (`effort.strategy: count`)

Every completed item counts as 1. This is the default and requires no extra configuration.

```yaml
velocity:
  effort:
    strategy: count
```

Velocity becomes a simple count of completed items. All items are "assessed" (no items will appear as not-assessed).

### Attribute (`effort.strategy: attribute`)

Maps labels, issue types, or title patterns to effort values using [matcher syntax]({{< relref "../config#matcher-syntax" >}}). First match wins. Unmatched items are "not assessed" and contribute 0 effort.

```yaml
velocity:
  effort:
    strategy: attribute
    attribute:
      - query: "label:size/L"
        value: 5
      - query: "label:size/M"
        value: 3
      - query: "label:size/S"
        value: 1
      - query: "type:Bug"
        value: 2
```

Matcher types:
- `label:<name>` -- exact label match
- `type:<name>` -- GitHub Issue Type match
- `title:/<regex>/i` -- title regex, case-insensitive

Items without a matching label or type are counted as "not assessed." When a high percentage of items are not assessed, the tool emits an insight warning that velocity may be understated.

### Numeric (`effort.strategy: numeric`)

Reads effort from a Number field on a GitHub Projects v2 board. Each item's effort is the value in the specified project field.

```yaml
velocity:
  effort:
    strategy: numeric
    numeric:
      project_field: "Story Points"
```

Requires `project.url` to be set. Items without a value in the field (null) are not assessed. Negative values are treated as not assessed with a warning.

## Iteration strategies

Iterations define the time boundaries for grouping work. Two strategies are available.

### Project field (`iteration.strategy: project-field`)

Reads iteration boundaries from a ProjectV2 Iteration field on your GitHub project board. Each iteration has a name, start date, and duration defined in the board settings.

```yaml
velocity:
  iteration:
    strategy: project-field
    project_field: "Sprint"
    count: 6
```

Requires `project.url` to be set. The tool fetches both active and completed iterations from the board, then matches items to iterations by their iteration field assignment.

### Fixed (`iteration.strategy: fixed`)

Computes iteration boundaries using calendar math from an anchor date and a fixed length. No project board required.

```yaml
velocity:
  iteration:
    strategy: fixed
    fixed:
      length: "14d"    # 14-day sprints
      anchor: "2026-01-06"  # a known sprint start date
    count: 6
```

The tool generates iteration windows by stepping forward/backward from the anchor date in increments of `length`. Items are matched to iterations by their close/merge date falling within the window.

Supported length formats: `Nd` (days, e.g., `14d`) or `Nw` (weeks, e.g., `2w`).

#### How fixed iterations find items

For fixed iterations without a project board, the tool uses the GitHub Search API to find items closed/merged within each iteration window. This means:
- Issues are found with `is:issue is:closed reason:completed closed:<start>..<end>`
- PRs are found with `is:pr is:merged merged:<start>..<end>`
- The `scope.query` filter is applied to narrow results

## Not-assessed items

When using `attribute` or `numeric` effort strategies, items that do not match any rule or lack a field value are "not assessed." These items:

- Contribute 0 effort to velocity
- Are still counted in `items_total`
- Are reported separately in `not_assessed` count
- Are listed by number when `--verbose` is used

The tool generates insights when not-assessed items are prevalent:
- **100% not assessed**: "All N items lack effort estimates -- velocity will be 0 until estimates are added."
- **50%+ not assessed**: "X% of items (N/M) lack effort estimates -- velocity may be understated."

## Trend indicators

Each iteration in the history shows a trend compared to the previous iteration:
- **up arrow**: Velocity increased
- **down arrow**: Velocity decreased
- **dash**: Velocity unchanged

## Aggregate statistics

Across all historical iterations, the tool computes:
- **Average velocity**: Mean effort completed per iteration
- **Average completion**: Mean completion rate
- **Standard deviation**: Variability in velocity (requires 2+ iterations)
- **Coefficient of variation (CV)**: StdDev / Mean -- when CV > 0.5, the tool warns about inconsistent sprint commitments

## Configuration reference

| Config field | Type | Default | Description |
|---|---|---|---|
| `velocity.unit` | string | `"issues"` | Work item type: `"issues"` or `"prs"` |
| `velocity.effort.strategy` | string | `"count"` | How effort is assigned: `"count"`, `"attribute"`, or `"numeric"` |
| `velocity.effort.attribute` | list | -- | Matcher rules for attribute strategy (required when strategy is `attribute`) |
| `velocity.effort.attribute[].query` | string | -- | Matcher pattern (e.g., `"label:size/L"`) |
| `velocity.effort.attribute[].value` | number | -- | Effort value for matching items (must be >= 0) |
| `velocity.effort.numeric.project_field` | string | -- | Project board Number field name (required when strategy is `numeric`) |
| `velocity.iteration.strategy` | string | -- | How iterations are bounded: `"project-field"` or `"fixed"` |
| `velocity.iteration.project_field` | string | -- | Iteration field name on the board (required for `project-field`) |
| `velocity.iteration.fixed.length` | string | -- | Iteration length, e.g., `"14d"` or `"2w"` (required for `fixed`) |
| `velocity.iteration.fixed.anchor` | string | -- | A known iteration start date, `YYYY-MM-DD` (required for `fixed`) |
| `velocity.iteration.count` | int | `6` | Number of past iterations to display |

## Example output

### Pretty format

```
Velocity  dvhthomas/gh-velocity
  Unit: issues  Effort: count  Iteration: fixed (14d)

  Current: Mar 4 – Mar 17 (day 11/14)
    Velocity:   4 items (of 6 committed) — 66.7% complete

  History
    Iteration         Velocity  Committed  Done/Total  Completion  Trend
    Feb 18 – Mar 3    7 items   8 items    7/8         87.5%       ▲
    Feb 4 – Feb 17    5 items   7 items    5/7         71.4%       ▼
    Jan 21 – Feb 3    6 items   6 items    6/6         100.0%      ─

  Average velocity: 6.0 items/iteration (std dev: 1.0)
  Average completion: 86.3%
```

### JSON format

```json
{
  "repository": "dvhthomas/gh-velocity",
  "unit": "issues",
  "effort_unit": "items",
  "current": {
    "name": "Mar 4 – Mar 17",
    "start": "2026-03-04T00:00:00Z",
    "end": "2026-03-18T00:00:00Z",
    "velocity": 4,
    "committed": 6,
    "completion_pct": 66.7,
    "items_done": 4,
    "items_total": 6,
    "not_assessed": 0,
    "day_of_cycle": 11,
    "total_days": 14
  },
  "history": [ ... ],
  "avg_velocity": 6.0,
  "avg_completion": 86.3,
  "std_dev": 1.0
}
```

## Commands

- `gh velocity flow velocity` -- velocity report (default: current + last 6 iterations)
- `gh velocity flow velocity --current` -- current iteration only
- `gh velocity flow velocity --history` -- past iterations only
- `gh velocity flow velocity --iterations 3` -- override iteration count
- `gh velocity flow velocity --verbose` -- show not-assessed item numbers

## See also

- [Setting Up Velocity]({{< relref "/guides/velocity-setup" >}}) -- step-by-step guide to effort strategies, iteration strategies, and validation
- [Throughput]({{< relref "/reference/metrics/throughput" >}}) -- simpler unweighted count metric for teams without sprints
- [Interpreting Results]({{< relref "/guides/interpreting-results" >}}) -- what healthy velocity looks like
- [Configuration Reference: velocity]({{< relref "/reference/config" >}}#velocity) -- all config fields
