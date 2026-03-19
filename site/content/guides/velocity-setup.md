---
title: "Setting Up Velocity"
weight: 2
---

# Setting Up Velocity

The `flow velocity` command measures effort completed per iteration -- a number, not a ratio. It answers "how much work did we ship this sprint?" expressed in effort units. A related metric, completion rate (done / committed), measures predictability. Both are shown in the output. For the metric definition and formula, see the [Velocity reference]({{< relref "/reference/metrics/velocity" >}}).

## How velocity differs from throughput

Throughput counts items in a sliding window. Velocity is effort-weighted and iteration-aligned:

- **[Throughput]({{< relref "/reference/metrics/throughput" >}})**: "12 issues closed in the last 30 days"
- **Velocity**: "31 story points completed in Sprint 12"

If you do not size your work items, velocity with the `count` strategy is effectively throughput sliced into iterations.

## Effort strategies

The `velocity.effort.strategy` field controls how the tool determines how much "work" an item represents. There are three strategies.

### Count (default)

Every completed item equals 1 unit. No sizing required. This is throughput repackaged into iterations.

```yaml
velocity:
  effort:
    strategy: count
```

Use count when your team does not estimate or size work items. It still gives you iteration-aligned completion data and a completion rate.

### Attribute

Map GitHub search queries to effort values. If you can run the query in the GitHub Issues UI, it works as an effort classifier. The tool evaluates queries in config order; first match wins.

```yaml
velocity:
  effort:
    strategy: attribute
    attribute:
      - query: "label:size/S"
        value: 1
      - query: "label:size/M"
        value: 3
      - query: "label:size/L"
        value: 5
      - query: "label:size/XL"
        value: 8
      - query: "type:epic"
        value: 13
```

Matchers use the same syntax as `scope.query` and `quality.categories[].match`. You can match on labels, issue types, title patterns, or any qualifier GitHub search supports.

Items matching no query are classified as "not assessed" and excluded from velocity and committed totals. Items explicitly configured with `value: 0` are valid (useful for chores or operational items that should count as done but carry no effort weight).

When an item matches multiple queries, the first match in config order wins. Run `gh velocity config validate --velocity` to detect overlaps.

### Numeric

Read effort directly from a GitHub Projects v2 Number field (e.g., "Story Points"). The field value is used as-is.

```yaml
velocity:
  effort:
    strategy: numeric
    numeric:
      project_field: "Story Points"
```

Items with a 0 or null field value are treated as "not assessed" and excluded from totals.

This strategy requires a `project:` section in your config and a token with `project` scope. See [CI Setup]({{< relref "/getting-started/ci-setup" >}}#setting-up-gh_velocity_token) for token configuration.

### Choosing a strategy

| Your workflow | Recommended strategy | Why |
|---|---|---|
| No estimation or sizing | `count` | Works immediately, no labels needed |
| T-shirt size labels (S/M/L/XL) | `attribute` | Maps labels to fibonacci-ish effort values |
| Story points on a project board | `numeric` | Reads the actual number field value |
| Mixed: some items labeled, some on a board | `attribute` | Covers items with labels; board-only items show as "not assessed" |

## Iteration strategies

The `velocity.iteration.strategy` field controls how the tool defines iteration boundaries. There are two strategies.

### Project field

Read iteration boundaries from a GitHub Projects v2 Iteration field. GitHub stores active/upcoming iterations and completed iterations with `startDate` and `duration`. The end date is computed as start + duration.

```yaml
velocity:
  iteration:
    strategy: project-field
    project_field: "Sprint"
```

An item is committed to an iteration if it is assigned to that iteration on the project board (via the `ProjectV2ItemFieldIterationValue`).

This strategy requires a `project:` section in your config and a token with `project` scope.

### Fixed

Define iteration boundaries with a length and anchor date. Simple calendar math, no project board needed.

```yaml
velocity:
  iteration:
    strategy: fixed
    fixed:
      length: 14d
      anchor: 2026-01-06
```

Iterations are computed forward and backward from the anchor date. An item is committed if it was open at iteration start or opened during the iteration. Items are assigned to the iteration in which they were closed or merged.

Use fixed when you do not have a project board or prefer calendar-based sprints.

### Choosing an iteration strategy

| Your workflow | Recommended strategy | Why |
|---|---|---|
| GitHub Projects with Iteration field | `project-field` | Matches your board's sprint definitions exactly |
| Fixed-length sprints, no project board | `fixed` | Calendar math, no API dependency |
| No sprints | `fixed` with `length: 30d` | Monthly iterations give useful velocity trends |

## Controlling the output

### History count

The `velocity.iteration.count` field sets how many past iterations to display. Override per-run with `--iterations`:

```yaml
velocity:
  iteration:
    count: 6    # default
```

```bash
gh velocity flow velocity --iterations 3
```

### Flags

| Flag | Effect |
|------|--------|
| `--current` | Show only the current (in-progress) iteration |
| `--history` | Show only past iterations, suppress current |
| `--iterations N` | Override iteration count |
| `--since DATE` | Show iterations overlapping this start date |
| `--until DATE` | Show iterations overlapping this end date |
| `--verbose` | Include not-assessed item numbers in output |
| `--results json` | JSON output for scripts and agents |

### Example output

```
Velocity  owner/repo

Current: Sprint 12 (Mar 4 - Mar 17)
  Velocity: 21 pts (8 items)
  Committed: 34 pts (14 items)
  Completion: 62%
  Carry-over: 6 items from prior sprints
  Not assessed: 3 items
  Projected: 29 pts (avg velocity: 31 pts/sprint)

History (last 6 sprints):
  Sprint   Period              Velocity   Committed   Rate    Items    Trend
  11       Feb 18 - Mar 3      31 pts     35 pts      89%     12/13    ^
  10       Feb 4 - Feb 17      28 pts     40 pts      70%     10/15    v
  9        Jan 21 - Feb 3      35 pts     38 pts      92%     14/16    ^
  8        Jan 7 - Jan 20      30 pts     33 pts      91%     11/12    -
  7        Dec 24 - Jan 6      15 pts     30 pts      50%      6/11    v
  6        Dec 10 - Dec 23     33 pts     36 pts      92%     13/14    ^

  Avg velocity: 28.7 pts/sprint  |  Avg completion: 80.7%  |  Std Dev: 6.8
```

## What "done" means

- **Issues**: Closed with `reason:completed` (not `reason:"not planned"`)
- **PRs**: Merged (not closed without merge)

Configure which work unit to track with `velocity.unit`:

```yaml
velocity:
  unit: issues    # "issues" (default) or "prs"
```

## Carry-over and scope

Items committed but not completed in an iteration roll forward into the next one with no cap. The `--scope` flag and `scope.query` in config filter both committed and completed items uniformly, so stale work outside your scope is naturally excluded.

## Preflight suggestions

When no `velocity` section exists in your config, `preflight` scans the repo and suggests a starting configuration.

```bash
gh velocity config preflight -R owner/repo
```

Preflight detects:

- **Labels** with common sizing patterns: prefixes like `size/*`, `effort:*`, `points-*`, `estimate-*`; standalone t-shirt sizes (XS, S, M, L, XL). Digit-only labels are skipped as too ambiguous. Detected labels are mapped to fibonacci-ish defaults (1, 2, 3, 5, 8, 13).
- **Project Number fields** (candidates for numeric effort): fields named "points", "story points", "estimate", "effort", or "size" rank higher.
- **Project Iteration fields** (candidates for iteration strategy).

The suggestion logic:
1. If a Number field is found, suggest `numeric` strategy
2. Else if sizing labels are found, suggest `attribute` strategy with mapped values
3. Else suggest `count` strategy with a note about adding effort later
4. If an Iteration field is found, suggest `project-field` iteration strategy
5. Else suggest `fixed` with a 14-day default

Use `--write` to write the suggested config directly:

```bash
gh velocity config preflight -R owner/repo --write
```

## Validating effort matchers

After configuring effort matchers, validate them against real issues:

```bash
gh velocity config validate --velocity
```

This runs your effort queries against closed issues and reports:

- **Match counts** per query
- **Overlap detection**: which issues match multiple queries, with resolution shown (first-match order)
- **Gap detection**: how many closed issues have no effort match
- **Distribution**: effort value spread to help spot misconfigured matchers

## Full config example

```yaml
velocity:
  unit: issues
  effort:
    strategy: attribute
    attribute:
      - query: "label:size/S"
        value: 1
      - query: "label:size/M"
        value: 3
      - query: "label:size/L"
        value: 5
      - query: "type:epic"
        value: 13
    numeric:
      project_field: "Story Points"
  iteration:
    strategy: project-field
    project_field: "Sprint"
    fixed:
      length: 14d
      anchor: 2026-01-06
    count: 6
```

Only the fields for your chosen strategies need to be present. If you use `strategy: attribute`, you do not need the `numeric` section, and vice versa.

## Next steps

- [Cycle Time Setup]({{< relref "cycle-time-setup" >}}) -- configure cycle time measurement
- [Interpreting Results]({{< relref "interpreting-results" >}}) -- understand the velocity output
- [Configuration Reference]({{< relref "/reference/config" >}}) -- full schema for all velocity fields
