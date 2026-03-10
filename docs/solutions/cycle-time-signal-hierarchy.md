---
title: "Cycle Time Signal Hierarchy and Backlog Suppression"
category: architecture-decisions
tags: [cycle-time, signals, projects-v2, labels, graphql, timeline-api]
module: internal/github/cyclestart.go
symptom: "Cycle time was only available with local git clone and commit-based detection"
root_cause: "Original design used only commit message parsing for cycle time start signal"
date: 2026-03-09
---

# Cycle Time Signal Hierarchy and Backlog Suppression

## Problem

Cycle time originally required a local git clone because it relied solely on commit messages referencing issue numbers (`fixes #42`). This was too constraining — many teams don't follow that convention, and requiring a local clone made the tool unusable in remote/API-only scenarios.

## Solution

Implemented a 5-level signal hierarchy that detects cycle time start from multiple GitHub data sources, with most signals available via API (no local clone needed):

### Signal Priority (highest to lowest)

1. **Status change** (Projects v2) — Issue moved out of backlog on a project board. Requires `project.id`/`status_field_id` config. Queries `projectItems → fieldValues` on the issue.

2. **Label** — An "active" label (e.g., "in-progress") was added. Requires `statuses.active_labels` config. Queries `LABELED_EVENT` from issue timeline.

3. **PR created** — Any PR referencing the issue was opened, including drafts and open PRs. Queries both `CLOSED_EVENT` (for closing PR) and `CROSS_REFERENCED_EVENT` (for any referencing PR) from timeline. Earliest PR creation date wins.

4. **First assigned** — Earliest `ASSIGNED_EVENT` on the issue timeline.

5. **First commit** — Earliest commit referencing the issue in git log. Requires local clone with full history.

### Backlog Suppression

Critical insight: if an issue is currently in backlog, cycle time should be **N/A regardless of other signals**. Someone may have been assigned, started work, then the issue was deprioritized back to backlog.

Two mechanisms for detecting backlog state:
- **Projects v2**: Current status field value matches `statuses.backlog` (default: "Backlog")
- **Labels**: Issue currently has a label in `statuses.backlog_labels` (e.g., "backlog", "icebox")

When backlog is detected, all lower-priority signals are skipped. Lead time is unaffected.

### PR Signal: Cross-References vs Closing References

The PR signal uses **two** timeline event types:
- `ClosedEvent.closer` — The PR that actually closed the issue (highest confidence)
- `CrossReferencedEvent.source` — Any PR that mentions the issue (catches drafts, open PRs)

The earliest PR creation date from either source is used. This means a draft PR opened 2 weeks before it closes the issue will correctly mark cycle time start at draft creation, not at merge.

## Key Design Decisions

- **Priority-based, not earliest-signal.** We pick the highest-priority signal, not the earliest timestamp. This prevents misleading cycle times from stale assignments.
- **Two status tracking mechanisms.** Projects v2 (board-based) and labels (issue-based) serve different workflows. Both can be active simultaneously — Projects v2 is checked first.
- **Backlog overrides everything.** This is intentional — a backlog status is a stronger signal than any other because it represents a team decision that work is not happening.
- **End signal is always `issue.closed_at`.** Both lead time and cycle time end when the issue closes. Future extension may add "in a release" as an optional end signal.

## GraphQL Query Pattern

The timeline query fetches 4 event types in a single call:
```
timelineItems(first: 100, itemTypes: [CLOSED_EVENT, ASSIGNED_EVENT, CROSS_REFERENCED_EVENT, LABELED_EVENT])
```

The project status uses a separate query to `projectItems → fieldValues` because it requires project-specific configuration and adds query complexity.

## Files

- `internal/github/cyclestart.go` — `GetCycleStart()` (timeline signals), `GetProjectStatus()` (project board signal)
- `cmd/cycletime.go` — Orchestrates signal priority and backlog suppression
- `internal/config/config.go` — `StatusConfig.ActiveLabels`, `StatusConfig.BacklogLabels`

## Gotchas

- `ProjectV2ItemFieldSingleSelectValue.updatedAt` gives when the status was LAST changed, not when it first left backlog. If an issue moves Backlog → In Progress → In Review, `updatedAt` reflects the In Review transition, not the original departure from Backlog.
- `CrossReferencedEvent` includes cross-references from issues too, not just PRs. Must check `source.__typename == "PullRequest"`.
- Labels are case-sensitive in the config. "In-Progress" won't match "in-progress".
