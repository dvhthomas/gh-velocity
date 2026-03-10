---
title: Command Hierarchy v2 — Group by Question Answered
date: 2026-03-10
status: active
---

# Command Hierarchy v2: Group by Question Answered

## What We're Building

Reorganize `gh velocity` subcommands from a flat grab-bag into three thematic groups based on the question each answers, plus a top-level composite report:

| Group | Question | Commands |
|-------|----------|----------|
| `flow` | How fast are we? | `lead-time`, `cycle-time`, `throughput` (future) |
| `quality` | How good is our output? | `release`, `dora` (future) |
| `status` | What's happening right now? | `wip`, `aging` (future) |
| `report` | Give me everything | Composite of all groups |

### Target Command Tree

```
gh velocity
├── flow
│   ├── lead-time <issue>               # single issue
│   ├── lead-time --since 30d           # bulk
│   ├── cycle-time <issue>              # single issue
│   ├── cycle-time --pr <number>        # single PR override
│   ├── cycle-time --since 30d          # bulk
│   └── throughput --since 30d          # (future)
│
├── quality
│   ├── release <tag> [--since <tag>] [--scope]
│   └── dora --since 30d               # (future) DORA four keys
│
├── status
│   ├── wip [-R owner/repo]
│   └── aging                           # (future) in-progress aging
│
├── report [--since 30d] [--until DATE] # composite dashboard
│
├── config
│   ├── show
│   ├── validate
│   ├── create
│   └── discover
│
└── version
```

## Why This Approach

1. **Discoverability**: `gh velocity flow --help` immediately shows all speed-related commands. New users can navigate by intent ("I want to know how fast we are") rather than memorizing command names.

2. **Extensibility**: DORA metrics (deployment frequency, change failure rate, MTTR) land naturally under `quality`. Throughput goes under `flow`. Aging goes under `status`. No awkward top-level proliferation.

3. **"scope" absorbed**: The standalone `scope` command becomes `quality release <tag> --scope`, which is clearer — it's always been about "what went into this release?"

4. **Naming**: "flow" (value-stream language) avoids repeating "velocity" and naturally encompasses lead-time, cycle-time, and throughput. "report" is clearer than "stats" or "dashboard" for a composite view.

## Key Decisions

- **Keep `velocity` as the top-level name.** It's established, and subcommand grouping clarifies the rest.
- **Three thematic groups: `flow`, `quality`, `status`.** Grouped by the question answered, not by metric type.
- **`report` replaces `stats`.** Clean break, no deprecated alias (pre-1.0).
- **`scope` becomes `--scope` flag on `quality release`.** Not a standalone command.
- **`gh` aliases solve typing length.** e.g., `gh alias set vlt 'velocity flow lead-time'`. No need to compromise naming for brevity.
- **Single and bulk modes stay the same.** Positional arg = single, `--since` = bulk. This pattern is already established and works well.

## Resolved Questions

- **Should the top-level name change?** No — keep `velocity`. Subcommand groups provide the clarity.
- **What happens to `scope`?** Becomes `--scope` flag on `quality release`.
- **How to handle the `stats` → `report` rename?** Clean break, no alias. Pre-1.0.
- **`speed` vs `flow` vs `pace`?** `flow` — value-stream language, no repetition with `velocity`.
- **Should `report` delegate to group logic or stay monolithic?** Delegate to each group's computation for consistency (single source of truth), but run group computations concurrently with goroutines to keep it fast.
- **Does `throughput` need a standalone command?** Yes — `gh velocity flow throughput --since 30d`. Useful for CI/automation and scripting. It's a first-class flow metric alongside lead-time and cycle-time.
