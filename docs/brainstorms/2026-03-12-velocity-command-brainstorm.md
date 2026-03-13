# Brainstorm: True Velocity Command

**Date:** 2026-03-12
**Status:** Complete
**Research:** [velocity-burndown-research.md](2026-03-12-velocity-burndown-research.md)

## What We're Building

A `flow velocity` command that measures **effort completed per iteration** — a number, not a ratio. Velocity answers "how much work did we ship this sprint?" expressed in effort units (points, hours, or item count).

Separately, **completion rate** (done / committed) is a related ratio that answers "how predictable are we?" Both are shown in output, but velocity is the headline number.

### How It Differs From Throughput

Throughput counts items in a sliding window. Velocity is effort-weighted and iteration-aligned:
- Throughput: "12 issues closed in the last 30 days"
- Velocity: "31 story points completed in Sprint 12"

### Effort Strategies

Three ways to determine how much "work" an item represents:

1. **Count** (baseline): Each completed item = 1 unit. Throughput repackaged into iterations.
2. **Attribute-based**: Scope-style GitHub search queries map items to effort values in config. If you can run the query in the GitHub Issues UI, it's a valid effort classifier.
3. **Numeric**: A ProjectV2 Number field (e.g., "Story Points") provides the value directly via GraphQL.

### Period Strategies

Two ways to define iteration boundaries:

1. **Project iteration field**: Read from a `ProjectV2IterationField`. GitHub stores `iterations` (active/upcoming) and `completedIterations` (past) with `startDate` and `duration`. End date = start + duration.
2. **Fixed periods**: Iteration length + anchor date in config. Simple calendar math, no project board needed.

### "Committed" Definition

Each period strategy implies its own committed definition:
- **Project iteration field**: An item is committed if it's assigned to that iteration on the board (explicit `ProjectV2ItemFieldIterationValue`).
- **Fixed periods**: An item is committed if it was open at iteration start OR opened during the iteration.

Carry-over: items committed but not completed roll forward. No cap — scope queries handle stale work naturally.

### "Done" Definition

- **Issues**: Closed with `reason:completed` (not `reason:"not planned"`)
- **PRs**: Merged (not closed-without-merge)
- Configurable via `velocity.unit: issues | prs`

### API Constraints (from research)

- **No field-value history**: GitHub API only exposes current state. Daily snapshots not feasible without cron.
- **No search API for iteration values**: `project:` qualifier checks membership only. Items in a specific iteration must be queried via GraphQL `fieldValueByName`.
- **Burnup is feasible**: `closedAt`/`mergedAt` timestamps give us cumulative completion over time within an iteration. No snapshots needed.

## Why This Approach

- **Scope-style queries for attribute matchers** — consistent with how `scope.query` works elsewhere. Testable in GitHub UI.
- **Three effort strategies** cover the spectrum: no sizing (count), t-shirt labels (attribute), story points on a board (numeric).
- **Two period strategies** cover GitHub Projects iterations and fixed sprints.
- **`flow velocity`** — velocity is a flow metric alongside lead-time, cycle-time, and throughput.
- **Velocity as a number, completion rate as a ratio** — distinct metrics, both shown, not conflated.

## Key Decisions

1. **Command location**: `flow velocity`
2. **Velocity is a number** (effort completed per iteration), not a ratio. Completion rate is separate.
3. **Effort strategies**: All three from the start (count, attribute, numeric).
4. **Period strategies**: Both from the start (project iteration field, fixed periods).
5. **Work unit**: Configurable `issues | prs`.
6. **Effort config uses scope-style queries**: `query: "label:size/S"` + `value: 1`.
7. **Default output**: Current iteration + historical table, with `--history` / `--current` flags.
8. **Posting**: Discussion posts for periodic summaries + issue comments on close.
9. **"Done" = `reason:completed` or merged**: Not "closed as not planned".
10. **Committed follows period strategy**: Board assignment for project-field, open-during for fixed.
11. **Unmatched items**: "Not assessed" — excluded from totals, reported with count + list.
12. **Multiple effort matches**: First-match wins (config order).
13. **Carry-over**: No cap, trust scope to filter stale work.
14. **Scope**: `--scope` AND'd with iteration boundaries, filters both committed and completed uniformly.
15. **Burnup only, no burndown**: Burnup is computable from `closedAt`/`mergedAt` timestamps. Burndown requires snapshots — out of scope entirely.
16. **`config validate --velocity`**: Runs effort matchers against real issues, reports match counts, overlaps, gaps, and distribution. Extensible to other config sections (`--cycle-time`, `--quality`).
17. **Preflight suggests velocity config**: When no velocity config exists, preflight scans labels, issue types, and project fields, then suggests a starting config block.
18. **Label heuristic patterns**: Prefixes (`size/*`, `effort:*`, `points-*`, `estimate-*`) and t-shirt sizes (XS/S/M/L/XL). No digit-only labels. Mapped to fibonacci-ish defaults.

## Config Shape

```yaml
velocity:
  unit: issues              # "issues" or "prs"
  effort:
    strategy: attribute      # "count", "attribute", or "numeric"
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
    strategy: project-field  # "project-field" or "fixed"
    project_field: "Sprint"
    fixed:
      length: 14d
      anchor: 2026-01-06
    count: 6                 # how many past iterations to show
```

## Output Concepts

### Default View (current + history)

```
Velocity  owner/repo

Current: Sprint 12 (Mar 4 – Mar 17)
  Velocity: 21 pts (8 items)
  Committed: 34 pts (14 items)
  Completion: 62%
  Carry-over: 6 items from prior sprints
  Not assessed: 3 items
  Projected: 29 pts (avg velocity: 31 pts/sprint)

History (last 6 sprints):
  Sprint   Period              Velocity   Committed   Rate    Items    Trend
  ──────   ──────              ────────   ─────────   ────    ─────    ─────
  11       Feb 18 – Mar 3      31 pts     35 pts      89%     12/13    ▲
  10       Feb 4 – Feb 17      28 pts     40 pts      70%     10/15    ▼
  9        Jan 21 – Feb 3      35 pts     38 pts      92%     14/16    ▲
  8        Jan 7 – Jan 20      30 pts     33 pts      91%     11/12    ─
  7        Dec 24 – Jan 6      15 pts     30 pts      50%      6/11    ▼
  6        Dec 10 – Dec 23     33 pts     36 pts      92%     13/14    ▲

  Avg velocity: 28.7 pts/sprint  |  Avg completion: 80.7%  |  Std Dev: 6.8
```

### Issue Close Comment

```
Closed in Sprint 12 (Mar 4 – Mar 17)
  This issue: 3 points (label:size/M)
  Sprint velocity so far: 24 pts (avg: 31 pts/sprint)
  Sprint completion: 24/34 pts (71%)
```

### Flags

- `--history` — historical iterations only, suppress current
- `--current` — current iteration only
- `--iterations N` — how many past iterations (default from config or 6)
- `--since` / `--until` — override iteration boundaries for ad-hoc queries
- `--format json|pretty|markdown` — standard output formats
- `--post` — post to configured target
- `--verbose` — include list of not-assessed items

## Zero/Null Effort Handling

- **Numeric strategy**: Items with 0 or null project field value → "not assessed". Excluded from velocity and committed totals.
- **Attribute strategy**: Items matching no query → "not assessed". Items explicitly configured with `value: 0` are valid (e.g., chores).
- **Stats**: Velocity averages, std dev, trends computed only over assessed items.
- **Output**: Each iteration shows assessed vs total item count. `--verbose` lists the specific not-assessed items.

## Config Validate

`config validate --velocity` runs effort matchers against real issues to surface problems before they affect velocity output.

```
$ gh velocity config validate --velocity

Effort matchers (attribute strategy):
  label:size/S (1pt)  — 23 issues matched
  label:size/M (3pt)  — 41 issues matched
  label:size/L (5pt)  — 12 issues matched
  type:epic (13pt)    —  4 issues matched

Overlaps (2 issues match multiple queries):
  #142 matches: label:size/M (3pt), type:epic (13pt) → using 13pt (first match)
  #198 matches: label:size/L (5pt), type:epic (13pt) → using 13pt (first match)

Unmatched: 18 of 98 closed issues have no effort assigned
  #12, #34, #56, #67, #78 ... (use --verbose for full list)

Distribution:
  1pt: 23 (23%)  |  3pt: 41 (42%)  |  5pt: 12 (12%)  |  13pt: 4 (4%)  |  unmatched: 18 (18%)
```

Key checks:
- **Overlap detection**: Which issues match multiple queries, with resolution shown (first-match order)
- **Gap detection**: How many closed issues have no effort match
- **Distribution**: Effort value spread — helps spot misconfigured matchers
- **Iteration field check** (if project-field strategy): Verify the iteration field exists and has iterations defined

Extensible: `--cycle-time`, `--quality` etc. can add their own validation sections later.

## Preflight Heuristics

When no `velocity` config section exists, `preflight` scans the repo and suggests a starting config.

```
$ gh velocity preflight

✓ Authentication valid
✓ Repository accessible
✓ Project board found

Suggested velocity config:
  Found labels: size/S, size/M, size/L, size/XL
  Found project field: "Story Points" (Number)
  Found project field: "Sprint" (Iteration)

  velocity:
    effort:
      strategy: numeric
      numeric:
        project_field: "Story Points"
    iteration:
      strategy: project-field
      project_field: "Sprint"

  Copy to .gh-velocity.yml? [y/N]
```

### Heuristic detection

**Labels** — scan for common sizing patterns:
- Prefixes: `size/*`, `effort:*`, `points-*`, `estimate-*`
- T-shirt sizes: standalone XS, S, M, L, XL labels
- No digit-only labels (too ambiguous)
- Default effort mapping: fibonacci-ish (1, 2, 3, 5, 8, 13)

**Project fields** — scan for:
- `ProjectV2Field` with type `NUMBER` → candidate for numeric effort
- `ProjectV2IterationField` → candidate for iteration strategy
- Heuristic ranking: fields named "points", "story points", "estimate", "effort", "size" ranked higher

**Strategy suggestion logic**:
- If Number field found → suggest `numeric` strategy
- Else if sizing labels found → suggest `attribute` strategy with mapped values
- Else → suggest `count` strategy with note about adding effort later
- If Iteration field found → suggest `project-field` iteration strategy
- Else → suggest `fixed` with 14-day default

## Resolved Questions

1. **Unmatched items**: Excluded, reported as "not assessed" with count + list.
2. **Multiple effort matches**: First-match (config order).
3. **Iteration assignment (fixed periods)**: Close/merge date.
4. **Scope interaction**: Filters both committed and completed uniformly.
5. **Carry-over cap**: No cap, trust scope.
6. **Zero/null effort**: Excluded from stats, reported separately.
7. **Committed definition**: Follows period strategy (board assignment vs open-during).
8. **Velocity vs completion rate**: Velocity = number (effort completed). Completion rate = ratio (done/committed). Both in output, distinct.
