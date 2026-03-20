---
title: "Staleness"
weight: 7
---

# Staleness

Staleness measures how recently a work item had activity. It applies to open items in the WIP report.

## Signals

| Signal | Threshold | Meaning |
|--------|-----------|---------|
| **ACTIVE** | Updated < 3 days ago | Normal activity, no concern |
| **AGING** | Updated 3-7 days ago | May need attention |
| **STALE** | Updated > 7 days ago | Likely blocked or abandoned |

## Calculation

```
staleness = now - item.updated_at
```

- `updated_at` is GitHub's last activity timestamp (comments, commits, label changes, status changes)
- Thresholds are currently fixed (not configurable)
- Staleness is computed at query time, not cached

## WIP staleness ratio

```
stale_ratio = stale_items / total_wip_items
```

A high stale ratio (>30%) suggests systemic flow problems -- items are being started but not finished.

## What this tells you

Individual staleness signals help you find items that need attention right now. The aggregate stale ratio reveals whether the team is finishing what it starts.

Common causes of high stale ratios:

- **Too much WIP** -- the team is spreading attention across too many items
- **Blocked work** -- items waiting on external dependencies with no escalation path
- **Abandoned experiments** -- items that were started but never formally closed
- **Missing updates** -- work is happening but not being tracked (comments, commits, status changes)

## Interaction with WIP limits

When [WIP limits](/reference/config/#wip) are configured, staleness signals complement the limit warnings. An item that is both over-limit and stale is a stronger signal that the team needs to close work before starting new work.
