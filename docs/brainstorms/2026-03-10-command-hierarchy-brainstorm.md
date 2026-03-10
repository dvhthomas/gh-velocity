---
title: Command Hierarchy — Single, Bulk, and Aggregate Commands
date: 2026-03-10
status: complete
---

# Command Hierarchy — Single, Bulk, and Aggregate Commands

## What We're Building

A redesigned command tree for gh-velocity that supports:
- **Single-item commands**: one issue, one PR — detailed view
- **Bulk/aggregate commands**: time-filtered queries returning per-item rows + aggregate stats
- **WIP command**: what's in progress right now (Projects v2 + label fallback)
- **Stats command**: trailing-window dashboard composing all metrics
- **Quality command**: release-scoped quality metrics (renamed from `release`)

The key insight: same command, different args. `lead-time 42` gives one issue. `lead-time --since 2026-01-01` gives bulk. The command auto-switches output between single-item detail and aggregate table based on input.

## Why This Approach

- **Scope: repo-per-team.** This tool serves small teams with lightweight needs — not org-wide rollups. Single repo is the primary scope. Projects v2 adds cross-repo board visibility for WIP, but release/quality metrics stay single-repo.
- **Same command, different args** avoids subcommand sprawl. Users learn one command name per metric. The `--since`/`--until` flags are the universal bulk trigger.
- **`stats` as composition, not duplication.** The stats command calls the same metric functions as the individual commands, then formats a one-screen summary. No separate data path.
- **Quality is release-scoped.** Defect rate, hotfix detection, and change failure rate are inherently release-bounded. Time-based quality doesn't make sense — `stats` handles the time-based aggregate view.

## Key Decisions

### 1. Command Tree

```
gh velocity
  lead-time <issue>                    # single issue
  lead-time --since DATE [--until DATE] # bulk: all closed issues in window

  cycle-time <issue>                   # single issue (configured strategy)
  cycle-time --pr <number>             # single PR
  cycle-time --since DATE [--until DATE] # bulk: all closed issues/PRs in window

  quality release <tag> --since <tag>  # release-scoped quality metrics

  wip                                  # current work in progress snapshot

  stats                                # trailing 30d dashboard (default)
  stats --since DATE [--until DATE]    # custom window

  scope <tag> --since <tag>            # diagnostic: what's in a release

  config show | validate | create | discover
  version
```

### 2. Single vs Bulk Auto-Switch

When a positional issue/PR number is given, output is single-item detail. When `--since` is given without a positional arg, output switches to bulk mode:
- Per-item rows (table in pretty/markdown, array in JSON)
- Aggregate stats (mean, median, P90, count)

This mirrors how `gh issue list` works — same command, filters change the scope.

### 3. Stats Command — 30-Day Dashboard

Default: trailing 30 days. Same `--since`/`--until` date flags as all other bulk commands.

```
gh velocity stats                                # trailing 30d
gh velocity stats --since 2026-01-01             # since Jan 1
gh velocity stats --since 2026-01-01 --until 2026-02-01  # Jan only
```

Output sections:
- **Lead Time**: median, P90, count
- **Cycle Time**: median, P90, count
- **WIP**: items in progress (count + optional list)
- **Quality**: defect count / total from most recent release
- **Throughput**: issues closed, PRs merged

Internally, `stats` calls the same metric functions as individual commands and composes the output. No duplicate data paths.

### 4. Quality = Release-Scoped Only

`quality release <tag> --since <tag>` replaces the current `release` command. Quality metrics (defect rate, hotfix detection, bug ratio, category composition) are inherently release-bounded. The `stats` command handles time-based quality by pulling from the most recent release.

No `quality --since DATE` mode — that would be a different (weaker) metric. Issue #9 tracks this rename.

### 5. WIP Command — Board + Label Fallback

Primary source: Projects v2 board status. WIP = items where status is NOT Backlog and NOT Done.

Fallback (no project board): open issues with `active_labels` minus issues with `backlog_labels`. Works for repos that use labels for status tracking.

The `wip` command supports `-R owner/repo` to filter board items to a specific repo. Since Projects v2 boards are inherently cross-repo, this lets teams see WIP for just one repo on a shared board. Without `-R`, all board items are shown.

Output: list of in-progress items with their age (time since entering WIP state or time since created if no board signal).

### 6. Time Filtering Convention

All bulk commands use the same flags:
- `--since DATE` — start of window (required for bulk mode)
- `--until DATE` — end of window (optional, defaults to now)

DATE format: `YYYY-MM-DD` or `YYYY-MM-DDThh:mm:ssZ`. Also accept relative: `30d`, `7d`, `90d`.

The `quality release` command keeps its existing `--since <tag>` semantics (tag-based, not date-based).

### 7. All Times in UTC

All timestamps in input, output, and computation are explicitly UTC. No local timezone ambiguity.

- Date-only input (`2026-01-15`) is interpreted as `2026-01-15T00:00:00Z`
- JSON output uses RFC 3339 with `Z` suffix
- Pretty/markdown output shows UTC explicitly (e.g., `2026-01-15 14:30 UTC`)
- Duration metrics (lead time, cycle time) are timezone-agnostic by nature
- GitHub API returns UTC timestamps, so no conversion is needed — just enforce it on the output side

### 8. Velocity vs Throughput (Future: Size Weighting)

Currently all metrics treat issues as equal-weight units — "14 issues closed" is throughput, not velocity. True velocity requires a size signal so you can say "we completed 42 points of work" or "3 epics and 11 tasks."

Teams express size differently:
- **Labels**: `epic`, `task`, `spike`, or t-shirt sizes `size/S`, `size/M`, `size/L`
- **Project fields**: a custom Projects v2 field like "Size" or "Story Points" (single-select or number)
- **Issue type**: GitHub's native Issue Types (e.g., Epic vs Task)

**Not implementing now**, but the design should accommodate a future `size` config block:

```yaml
# Future — not implemented yet
size:
  source: label | field | type    # where to read size from
  field_name: "Size"              # Projects v2 field name (when source: field)
  weights:                        # map size values to numeric weights
    epic: 8
    task: 1
    spike: 2
    # or: S: 1, M: 3, L: 5, XL: 8
```

This would enable:
- Weighted throughput in `stats` (points completed, not just count)
- Size distribution in `quality release` (are we shipping lots of small fixes or a few big features?)
- Velocity trend over time (points per period)

The `classify.Input` struct already has extensible fields (`Labels`, `IssueType`, `Title`). Adding `Size string` or `SizeWeight int` later is straightforward. The `stats` command's throughput section is where this would surface first.

**For now**: throughput = issue count. The name "gh-velocity" is aspirational — size weighting turns throughput into true velocity.

### 9. Cross-Repo Strategy

Single repo (`--repo`) is the primary scope for all commands. Projects v2 queries are inherently cross-repo but are only used for WIP and board status signals — not as the primary data source for lead-time, cycle-time, or quality.

No `--repos` flag, no multi-repo config, no org-wide queries. The model types don't need a repo field. This keeps the architecture simple and matches the "small team, lightweight needs" target.

If a team works across repos, they run the tool per-repo and compare. Or they use the project board (via `wip`) which naturally spans repos.

## Resolved Questions

- **Cross-repo scope?** — Repo-per-team. Projects v2 for WIP (cross-repo by nature), everything else single-repo. Not building for org-wide rollups.
- **Command nesting?** — Same command, different args. No noun-first or scope-first nesting. Flat commands with `--since` as the bulk trigger.
- **Stats default window?** — 30 days trailing, same `--since`/`--until` flags as all bulk commands. Dashboard style, not release-anchored.
- **WIP signal source?** — Projects v2 board status primary, label-based fallback for repos without boards. `-R` flag filters board items to one repo.
- **Quality scope?** — Release-only. `quality release <tag> --since <tag>`. No time-based quality mode.
- **Timezone handling?** — All times explicitly UTC. Date-only input = midnight UTC. Output always shows UTC. No local timezone conversion.
