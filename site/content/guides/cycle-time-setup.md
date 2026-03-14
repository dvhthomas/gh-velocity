---
title: "Cycle Time Setup"
weight: 3
---

# Cycle Time Setup

Cycle time measures how long active work took on an issue. It excludes backlog time (unlike lead time, which measures the full elapsed duration from creation to close). The measurement depends on your configured strategy.

## Choosing a strategy

There are two cycle time strategies. Choose based on your workflow.

| Your workflow | Recommended strategy | Why |
|---|---|---|
| Issues with lifecycle labels | `issue` | Measures real work time (label applied to closed); immutable timestamps |
| Issues on a project board | `issue` + labels | Use labels for cycle time, board for WIP/backlog visibility |
| PRs close issues (most OSS repos) | `pr` | Measures PR review time (created to merged) |
| Issues only, no labels or PRs | `issue` | Lead time works immediately; add an `in-progress` label for cycle time |

**If you are unsure, start with `pr`.** It works immediately with no extra config.

## The PR strategy

The PR strategy uses the closing PR's creation date as the cycle start and its merge date as the end. No extra configuration is needed beyond setting the strategy:

```yaml
cycle_time:
  strategy: pr
```

Ensure your PRs reference issues with "Closes #N" or "Fixes #N" in the PR description, or use GitHub's sidebar "Development" section to link them. The PR does not need to be merged or even out of draft -- opening a draft PR that mentions an issue is enough for a cycle time signal.

Lead time is unaffected by strategy choice -- it always measures issue creation to close.

## The issue strategy

The issue strategy uses labels as the primary cycle time signal. When a matching label is applied to an issue, that timestamp becomes the cycle time start. The issue's close date is the cycle time end.

### Why labels are preferred

Label event timestamps (`LABELED_EVENT.createdAt`) are **immutable**. Once a label is applied, the timestamp never changes -- not when you remove the label, not when you re-add it, not when anything else changes. This makes labels the only reliable source of "when did work start?" from the GitHub API.

Project board timestamps, by contrast, are mutable. The `updatedAt` on a project field value reflects the **last** status change, not the original transition. This can produce negative cycle times when cards are moved after issue closure. For a full explanation of this limitation, see [Labels vs. Project Board]({{< relref "/concepts/labels-vs-board" >}}).

### Configuring lifecycle labels

Tell the tool which labels mark "work started":

```yaml
cycle_time:
  strategy: issue

lifecycle:
  in-progress:
    match: ["label:in-progress", "label:wip"]
```

To enable this:

1. Create a label like `in-progress` or `wip` in your repo
2. Add `lifecycle.in-progress.match` to your config with the label names
3. Apply the label to issues when work starts

Or run preflight to auto-detect existing labels:

```bash
gh velocity config preflight -R owner/repo --write
```

### Adding project board for WIP and backlog

If you also use a GitHub Projects v2 board, configure both signals. The board powers WIP counts and backlog detection. Labels power cycle time:

```yaml
project:
  url: "https://github.com/users/yourname/projects/1"
  status_field: "Status"

lifecycle:
  backlog:
    project_status: ["Backlog", "Triage"]
  in-progress:
    project_status: ["In progress"]          # WIP detection
    match: ["label:in-progress"]             # cycle time (preferred signal)
```

When both `match` and `project_status` are configured for a lifecycle stage, labels take priority for cycle time. Project board status is used as a fallback if no matching label is found.

If you configure only `project_status` without `match` for the in-progress stage, the tool emits a deprecation warning. Project board timestamps are unreliable for cycle time -- see [Labels vs. Project Board]({{< relref "/concepts/labels-vs-board" >}}).

Run `gh velocity config discover -R owner/repo` to find your project URL, status field name, and available status values.

## Workflow patterns

### Solo developer / OSS workflow (PR strategy)

Create an issue, open a PR with "Closes #N", merge, tag a release. Use `cycle_time.strategy: pr`. Works with no extra config.

```yaml
cycle_time:
  strategy: pr
```

### Team workflow with project board (issue strategy + labels)

Create an issue, triage into Backlog, move to In Progress and apply `in-progress` label, open a PR, review, merge, release. Use `cycle_time.strategy: issue` with `lifecycle.in-progress.match` for cycle time and `project_status` for WIP/backlog.

```yaml
cycle_time:
  strategy: issue

project:
  url: "https://github.com/users/yourname/projects/1"
  status_field: "Status"

lifecycle:
  backlog:
    project_status: ["Backlog", "Triage"]
  in-progress:
    project_status: ["In progress"]
    match: ["label:in-progress"]
```

To automate the label step when moving cards on the board, use a GitHub Actions workflow triggered by `projects_v2_item` events. See [Labels vs. Project Board: Syncing]({{< relref "/concepts/labels-vs-board" >}}) for a ready-to-use workflow.

### Team workflow without project board (PR strategy)

Create an issue, developer opens a PR with "Closes #N", review, merge, release. Use `cycle_time.strategy: pr`. The PR creation date is the cycle start.

```yaml
cycle_time:
  strategy: pr
```

## Connecting PRs to issues

The tool finds PR-to-issue connections through GitHub's timeline events. A PR becomes a cycle time signal when it references an issue in any of these ways:

- Write `Fixes #42`, `Closes #42`, or `Resolves #42` in a PR description
- Use GitHub's sidebar "Development" section to link a PR to an issue
- Mention `#42` anywhere in the PR (creates a cross-reference event)
- Any variation: `fix #42`, `close #42`, `resolve #42` (case-insensitive)

You do **not** need to:
- Add special labels or tags
- Use a specific branch naming convention
- Configure webhooks or integrations
- Follow any commit message format (unless you want commit-based enrichment)

## Running cycle time commands

Single issue:

```bash
gh velocity flow cycle-time 42
gh velocity flow cycle-time 42 -R cli/cli
```

Single PR (always uses PR created to merged, regardless of configured strategy):

```bash
gh velocity flow cycle-time --pr 99
```

Bulk (all issues closed in a window):

```bash
gh velocity flow cycle-time --since 30d
gh velocity flow cycle-time --since 2026-01-01 --until 2026-02-01 --format json
```

Cycle time does not require a local clone. It uses GitHub API signals (PR creation date, label events, project status). Running from inside a local checkout adds commit counts and a fallback signal from commit history.

## Troubleshooting cycle time

If cycle time shows N/A for all or most issues, see [Troubleshooting: Cycle time shows N/A]({{< relref "troubleshooting" >}}#cycle-time-shows-na-for-all-issues).

## Next steps

- [Labels vs. Project Board]({{< relref "/concepts/labels-vs-board" >}}) -- full explanation of the timestamp immutability issue
- [Velocity Setup]({{< relref "velocity-setup" >}}) -- configure effort-weighted iteration metrics
- [Configuration Reference]({{< relref "/reference/config" >}}) -- all cycle_time and lifecycle fields
