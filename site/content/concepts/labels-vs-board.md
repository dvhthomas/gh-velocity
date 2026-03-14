---
title: "Labels vs. Project Board"
weight: 4
---

# Why Labels Over Project Board for Cycle Time

This page explains a fundamental limitation of the GitHub Projects v2 API and why `gh-velocity` recommends labels for cycle time measurement.

## The problem: project board timestamps are mutable

When you move an issue to "In Progress" on a Projects v2 board, GitHub records a `ProjectV2ItemFieldSingleSelectValue` with an `updatedAt` timestamp. This seems like a useful cycle time signal, but it has a critical flaw: **`updatedAt` reflects the last time the field was modified, not when the status was originally set**.

Here is a common scenario that produces wrong data:

1. **Monday**: You move issue #42 to "In Progress". The field's `updatedAt` is Monday.
2. **Wednesday**: You close issue #42.
3. **Thursday**: You tidy up the board and move the card to "Done". The field's `updatedAt` is now **Thursday**.

The tool computes cycle time as start minus end: Thursday (start signal) minus Wednesday (close date) equals **negative one day**. This is nonsensical.

This is not a bug in `gh-velocity`. It is a fundamental limitation of the GitHub Projects v2 API:

- **There is no field change history API.** You cannot query "when did this issue first move to In Progress?" -- only "what is the current status, and when was it last modified?"
- **The REST timeline API does not include project field changes.** Even per-issue timeline queries cannot retrieve project board transitions.
- **`updatedAt` on field values is the only timestamp available**, and it is overwritten on every field change.

The tool filters negative durations from aggregate statistics and warns you, but the root cause cannot be fixed at the application level.

## The solution: use labels

Label events have **immutable timestamps**. When you apply a label to an issue, GitHub creates a `LABELED_EVENT` with a `createdAt` timestamp that **never changes** -- not when you remove the label, not when you re-add it, not when you modify anything else. The first application of that label is permanently recorded.

This makes labels the only reliable source of "when did work start?" from the GitHub API.

To use labels for cycle time:

1. Create a label like `in-progress` or `wip` in your repo
2. Configure `lifecycle.in-progress.match` in `.gh-velocity.yml` to match it
3. Apply the label to issues when work starts

The label's immutable `createdAt` becomes the cycle time start. The issue's close date becomes the end. No timestamp can be retroactively changed.

## What project board status is still good for

Project board status remains valuable for **current-state queries** -- things that ask "what is the status right now?" rather than "when did the status change?"

| Use case | Signal | Reliable? |
|----------|--------|-----------|
| Cycle time start (when did work begin?) | Label `createdAt` | Yes -- immutable |
| Cycle time start (when did work begin?) | Board `updatedAt` | No -- mutable, can produce negative durations |
| WIP count (how many items are in progress now?) | Board current status | Yes -- current state is accurate |
| Backlog detection (is this issue still in backlog?) | Board current status | Yes -- current state is accurate |

The takeaway: configure `project_status` for WIP and backlog detection. Configure `match` for cycle time.

## Syncing project board status to labels

If your team uses a project board as the primary workflow tool and does not want to manually apply labels, you can automate the sync with a GitHub Actions workflow:

```yaml
# .github/workflows/project-label-sync.yml
name: Sync project status to labels

on:
  # Requires a GitHub App or classic PAT with 'project' scope.
  # GITHUB_TOKEN cannot receive projects_v2_item events.
  projects_v2_item:
    types: [edited]

jobs:
  sync:
    runs-on: ubuntu-latest
    if: github.event.changes.field_value.field_name == 'Status'
    steps:
      - name: Apply in-progress label
        if: github.event.changes.field_value.to.name == 'In progress'
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          # Get the issue/PR URL from the project item
          CONTENT_URL=$(gh api graphql -f query='
            query($itemId: ID!) {
              node(id: $itemId) {
                ... on ProjectV2Item {
                  content {
                    ... on Issue { url }
                    ... on PullRequest { url }
                  }
                }
              }
            }' -f itemId="${{ github.event.projects_v2_item.node_id }}" \
            --jq '.data.node.content.url')

          if [ -n "$CONTENT_URL" ]; then
            gh issue edit "$CONTENT_URL" --add-label "in-progress"
          fi
```

The `projects_v2_item` webhook event requires a **GitHub App** or a **classic PAT** with `project` scope. The default `GITHUB_TOKEN` in GitHub Actions **cannot** receive project board events. This is another GitHub platform limitation.

If setting up a GitHub App or PAT is not feasible, the simplest alternative is to manually apply the `in-progress` label when you start work. Applying a label is a single click in the GitHub issue sidebar.

## Configuration examples

**Labels only (simplest, most reliable):**

```yaml
lifecycle:
  in-progress:
    match: ["label:in-progress"]
```

**Labels + project board (recommended for board users):**

```yaml
project:
  url: "https://github.com/users/yourname/projects/1"
  status_field: "Status"

lifecycle:
  backlog:
    project_status: ["Backlog", "Triage"]
  in-progress:
    project_status: ["In progress"]          # WIP detection
    match: ["label:in-progress"]             # cycle time (immutable timestamp)
  done:
    project_status: ["Done", "Shipped"]
```

**Project board only (not recommended):**

```yaml
# WARNING: This config produces unreliable cycle times.
# The project board updatedAt timestamp can be wrong.
# Add lifecycle.in-progress.match for reliable cycle time.
project:
  url: "https://github.com/users/yourname/projects/1"
  status_field: "Status"

lifecycle:
  in-progress:
    project_status: ["In progress"]
```

Using this last configuration triggers a deprecation warning. The tool still computes cycle time from the board's `updatedAt`, but the results may include negative durations or inaccurate start times. Adding a `match` rule with labels eliminates these problems.
