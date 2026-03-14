---
title: Label-based lifecycle for cycle time without project board
category: architecture-patterns
date: 2026-03-13
tags: [cycle-time, lifecycle, labels, timeline, graphql, label-event]
related: [IssueStrategy, GetLabelCycleStart, LifecycleStage]
---

# Label-based lifecycle for cycle time without project board

## Problem

Cycle time's issue strategy required a project board with status columns to detect "work started." Repos without project boards (or with label-based workflows) couldn't use the issue strategy — they had to fall back to the PR strategy, losing the issue-level signal.

## Solution

Added a second signal source to `IssueStrategy`: label timeline events via GitHub's `LabeledEvent` GraphQL query.

### Config

```yaml
lifecycle:
  in-progress:
    match:
      - "label:in-progress"
      - "label:wip"
```

The `match` field uses the same matcher syntax as quality categories (`label:`, `type:`, `title:`).

### Two-layer cost model

1. **Client-side label matching is free** — search results already include labels, so filtering by label name costs nothing.
2. **Timeline API is per-issue** — `GetLabelCycleStart()` queries `timelineItems(itemTypes: [LABELED_EVENT])` only for issues that need a cycle-time timestamp. Uses `singleflight` + cache to prevent duplicate queries.

### Signal priority

`IssueStrategy.Compute()` tries project board first (higher fidelity), falls back to label timeline:

```go
func (s *IssueStrategy) Compute(ctx context.Context, input CycleTimeInput) model.Metric {
    if s.ProjectID != "" { return s.computeFromProject(ctx, input) }
    if len(s.InProgressMatch) > 0 { return s.computeFromLabels(ctx, input) }
    return model.Metric{} // no signal
}
```

### GraphQL query

```graphql
query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    issue(number: $number) {
      timelineItems(first: 100, itemTypes: [LABELED_EVENT]) {
        nodes {
          ... on LabeledEvent { createdAt label { name } }
        }
      }
    }
  }
}
```

Returns the earliest `LabeledEvent.createdAt` matching any configured matcher as the cycle start.

### Preflight integration

Preflight now recommends label-based strategy when active labels are found but no project board status field exists:

```
project board → status labels → PR strategy → broken
```

## Prevention

- When adding new lifecycle signal sources, follow the same priority pattern: higher-fidelity sources first, cheaper fallbacks second
- Always cache timeline queries — they're N+1 by nature (one per issue)
- Warning messages about missing lifecycle config must mention both `project_status` and `match` options
