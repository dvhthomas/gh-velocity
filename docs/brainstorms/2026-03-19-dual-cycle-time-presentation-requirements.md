---
date: 2026-03-19
topic: dual-cycle-time-presentation
---

# Dual Cycle Time: Issue vs PR in Aggregate Reporting

## Problem Frame

Users want both issue cycle time ("how long did this work item take") and PR cycle time ("how long did this code change take to land"). The data model already supports both — the `issue` command shows both dimensions together. But the aggregate `report` command currently presents one cycle-time number, forcing a strategy choice (issue or pr). Users shouldn't have to choose; they want both, clearly labeled.

The tricky cases:
- Issue with multiple PRs — which PR CT represents the issue?
- PR with no linked issue — missing the "why" context
- Issue with no PRs — closed via commit or manually
- PR linked to multiple issues — many-to-many

## Status

Parked for later. The single-item `issue` command already shows both dimensions correctly. This brainstorm is about how the aggregate report presents both without confusion.

## Key Insight

Issue CT is the manager/team lead metric (full lifecycle). PR CT is the engineering efficiency metric (code review cycle). They measure different things and both are valuable. The architecture supports both — this is a presentation/UX problem, not a data model problem.

## Next Steps

→ Resume `/ce:brainstorm` when tackling the report command's cycle-time section.
