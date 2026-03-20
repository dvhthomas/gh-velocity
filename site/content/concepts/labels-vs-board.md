---
title: "Labels as Lifecycle Signal"
weight: 4
---

# Labels Are the Lifecycle Signal

Labels are the sole source of truth for lifecycle and cycle-time signals in gh-velocity. Project boards remain useful for visibility and velocity reads (iteration tracking, effort fields), but they are not used for lifecycle or cycle-time measurement.

## Why labels won

Label event timestamps (`LABELED_EVENT.createdAt`) are **immutable**. Once a label is applied, the timestamp never changes -- not when you remove the label, not when you re-add it, not when anything else changes. The first application of that label is permanently recorded.

Project board timestamps, by contrast, are mutable. The GitHub Projects v2 API only exposes `updatedAt` on field values -- the timestamp of the **last** status change, not the original transition to "In Progress." If someone moves a card after the issue is closed, `updatedAt` reflects that post-closure move, producing negative cycle times (`start > end`). There is no field change history API to retrieve the original transition date.

This is a fundamental GitHub API limitation that cannot be worked around at the application level. Labels are the only reliable answer to "when did work start?"

## What labels do

| Signal | Source | Used for |
|--------|--------|----------|
| `in-progress` label applied | `LABELED_EVENT.createdAt` (immutable) | Cycle time start |
| `in-review` label applied | `LABELED_EVENT.createdAt` (immutable) | Lifecycle stage grouping |
| `done` label applied | `LABELED_EVENT.createdAt` (immutable) | Lifecycle stage grouping |
| Issue closed | `issue.closed_at` | Cycle time end, lead time end |

## What project boards do

Project boards remain valuable for velocity and visibility -- they are just not lifecycle signals:

| Use case | How it works |
|----------|-------------|
| Iteration tracking | `velocity.iteration.strategy: project-field` reads an Iteration field from the board |
| Effort classification | `field:Size/M` matchers read SingleSelect fields from the board |
| Team visibility | Board columns give a visual overview of work status |

## What project boards ARE used for

Project boards serve two concrete roles in gh-velocity:

1. **Iteration tracking** — the `velocity.iteration.strategy: project-field` setting reads an Iteration field from the board to group issues into sprints for velocity measurement.
2. **Effort fields** — the `numeric` effort strategy reads a Number field from the board (e.g., story points), and `field:Size/M` matchers read SingleSelect fields for the `attribute` effort strategy.

Both are read-only data sources. The board is never used as a lifecycle or cycle-time signal.

## Suggested labels

- **`in-progress`** (required for cycle time) -- apply when work starts on an issue
- **`in-review`** (optional) -- apply when a PR is opened for code review
- **`done`** (optional) -- apply when work is complete

## Configuration

```yaml
lifecycle:
  in-progress:
    match: ["label:in-progress"]
  in-review:
    match: ["label:in-review"]
  done:
    query: "is:closed"
    match: ["label:done"]
```

## If you use a project board

If your team uses a project board as the primary workflow tool and does not want to manually apply labels, use a label-sync GitHub Action to keep labels in sync with board column changes. This way the board drives your workflow while labels provide the immutable timestamps gh-velocity needs.

Use [gh-project-label-sync](https://github.com/dvhthomas/gh-project-label-sync) to automatically apply lifecycle labels when cards move on the board.

The `projects_v2_item` webhook event requires a **GitHub App** or a **classic PAT** with `project` scope. The default `GITHUB_TOKEN` in GitHub Actions **cannot** receive project board events. This is a GitHub platform limitation.

If setting up a GitHub App or PAT is not feasible, the simplest alternative is to manually apply the `in-progress` label when you start work. Applying a label is a single click in the GitHub issue sidebar.

## Project board with velocity reads

You can use a project board for velocity without using it for lifecycle. This is the recommended pattern for teams that use boards:

```yaml
# Project board for velocity iteration/effort reads
project:
  url: "https://github.com/users/yourname/projects/1"
  status_field: "Status"

# Labels for lifecycle (sole source of truth)
lifecycle:
  in-progress:
    match: ["label:in-progress"]

# Velocity reads iteration and effort from the board
velocity:
  effort:
    strategy: attribute
    attribute:
      - query: "field:Size/S"
        value: 2
      - query: "field:Size/M"
        value: 3
      - query: "field:Size/L"
        value: 5
  iteration:
    strategy: project-field
    project_field: "Sprint"
```

## See also

- [Cycle Time Setup]({{< relref "/guides/cycle-time-setup" >}}) -- step-by-step guide to configuring cycle time with labels
- [Cycle Time Reference]({{< relref "/reference/metrics/cycle-time" >}}) -- metric definition, signals, and strategies
- [Configuration Reference: lifecycle]({{< relref "/reference/config" >}}#lifecycle) -- full schema for lifecycle stages
- [GitHub's Capabilities & Limits]({{< relref "/concepts/github-capabilities" >}}) -- broader context on what the API can and cannot do
