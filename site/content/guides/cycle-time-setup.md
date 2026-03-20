---
title: "Cycle Time Setup"
weight: 3
---

# Cycle Time Setup

Cycle time measures how long active work took on an issue, excluding backlog time (unlike [lead time]({{< relref "/reference/metrics/lead-time" >}}), which measures full elapsed duration from creation to close). The measurement depends on your configured strategy. For the metric definition and formula, see the [Cycle Time reference]({{< relref "/reference/metrics/cycle-time" >}}).

## Choosing a strategy

There are two cycle time strategies. Choose based on your workflow.

| Your workflow | Recommended strategy | Why |
|---|---|---|
| Issues with lifecycle labels | `issue` | Measures real work time (label applied to closed); immutable timestamps |
| PRs close issues (most OSS repos) | `pr` | Measures PR review time (created to merged) |
| Issues only, no labels or PRs | `issue` | Lead time works immediately; add an `in-progress` label for cycle time |

The PR strategy requires zero setup; the issue strategy gives richer data but requires label discipline.

> [!TIP]
> No labels yet? The PR strategy works without any setup -- it uses PR merge times. Switch to the issue strategy later when you add lifecycle labels.

## The PR strategy

The PR strategy uses the closing PR's creation date as the cycle start and its merge date as the end. No extra configuration needed:

```yaml
cycle_time:
  strategy: pr
```

PRs must reference issues with "Closes #N" or "Fixes #N" in the description, or use GitHub's sidebar "Development" section. The PR does not need to be merged or out of draft -- opening a draft PR that mentions an issue is enough for a cycle time signal.

Lead time is unaffected by strategy choice -- it always measures issue creation to close.

## The issue strategy

The issue strategy uses labels as the cycle time signal. When a matching label is applied, that timestamp becomes the cycle start. The issue's close date is the cycle end.

### Why labels

Label event timestamps (`LABELED_EVENT.createdAt`) are **immutable**. Once applied, the timestamp never changes -- not on removal, not on re-application, not on any other event. This makes labels the only reliable "when did work start?" signal from the GitHub API.

### Configuring lifecycle labels

Tell the tool which labels mark "work started":

```yaml
cycle_time:
  strategy: issue

lifecycle:
  in-progress:
    match: ["label:in-progress"]
```

Suggested labels:

- **`in-progress`** (required for cycle time) -- apply when work starts
- **`in-review`** (optional) -- apply when a PR is opened for review
- **`done`** (optional) -- apply when work is complete

To enable this:

1. Create a label like `in-progress` in your repo
2. Add `lifecycle.in-progress.match` to your config with the label name
3. Apply the label to issues when work starts

Or run preflight to auto-detect existing labels:

```bash
gh velocity config preflight -R owner/repo --write
```

### If you use a project board

If your team uses a GitHub Projects v2 board, use [gh-project-label-sync](https://github.com/dvhthomas/gh-project-label-sync) to apply lifecycle labels automatically when cards move. The board drives your workflow; labels provide the immutable timestamps gh-velocity needs.

The project board remains useful for velocity iteration/effort reads (via `velocity.iteration.strategy: project-field` and `field:` matchers), but is not used as a lifecycle or cycle-time signal.

## Workflow patterns

### Solo developer / OSS workflow (PR strategy)

Create an issue, open a PR with "Closes #N", merge, tag a release. Use `cycle_time.strategy: pr`. Works with no extra config.

```yaml
cycle_time:
  strategy: pr
```

### Team workflow with labels (issue strategy)

Create an issue, apply `in-progress` label when work starts, open a PR, review, merge, release. Use `cycle_time.strategy: issue` with `lifecycle.in-progress.match`.

```yaml
cycle_time:
  strategy: issue

lifecycle:
  in-progress:
    match: ["label:in-progress"]
  in-review:
    match: ["label:in-review"]
  done:
    query: "is:closed"
    match: ["label:done"]
```

If you also use a project board for visibility, use [gh-project-label-sync](https://github.com/dvhthomas/gh-project-label-sync) to automate the label step when moving cards on the board.

### Team workflow without labels (PR strategy)

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

None of the following are required:
- Special labels or tags
- A specific branch naming convention
- Webhooks or integrations
- A commit message format (unless you want commit-based enrichment)

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
gh velocity flow cycle-time --since 2026-01-01 --until 2026-02-01 --results json
```

Cycle time does not require a local clone -- it uses GitHub API signals (PR creation date, label events). Running from a local checkout adds commit counts and a fallback signal from commit history.

## Troubleshooting cycle time

If cycle time shows N/A for all or most issues, see [Troubleshooting: Cycle time shows N/A]({{< relref "troubleshooting" >}}#cycle-time-shows-na-for-all-issues).

## Next steps

- [Labels as Lifecycle Signal]({{< relref "/concepts/labels-vs-board" >}}) -- why labels are the sole lifecycle signal
- [Setting Up Flow Velocity]({{< relref "velocity-setup" >}}) -- configure effort-weighted iteration metrics
- [Configuration Reference]({{< relref "/reference/config" >}}) -- all cycle_time and lifecycle fields
