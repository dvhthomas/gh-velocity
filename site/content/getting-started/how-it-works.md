---
title: "How It Works"
weight: 2
---

# How It Works

`gh-velocity` reads the artifacts your team already produces -- issues, pull requests, labels, releases -- and turns them into metrics. No separate data warehouse, no tracking integration, no per-seat subscription. Everything comes from the GitHub API.

## The lifecycle of an issue

Every issue follows a lifecycle, and each transition produces a timestamp that maps to a metric:

```
1. You create an issue             -> lead time clock starts
2. Issue gets "in-progress" label  -> cycle time starts (issue strategy)
   OR a PR referencing the issue   -> cycle time starts (PR strategy)
   is created
3. The issue is closed             -> lead time + cycle time clocks stop
4. You publish a release that      -> release lag clock stops
   includes this work
```

Which cycle time signal is used depends on your configured strategy (`cycle_time.strategy` in `.gh-velocity.yml`). If you are unsure, start with `pr` -- it works immediately with no extra configuration.

## The metrics

**Lead time** is the total elapsed time from issue creation to closure. It includes time in backlog, waiting for review, blocked by dependencies, or simply forgotten. A long lead time does not necessarily mean slow development -- it often means slow prioritization.

**Cycle time** measures how long active work took. Two strategies are available:

- **Issue strategy** (`cycle_time.strategy: issue`): Starts when an in-progress label is applied, ends when the issue is closed. Label timestamps are immutable and reliable.
- **PR strategy** (`cycle_time.strategy: pr`): Starts when the closing PR is created, ends when it is merged. Works with no extra config -- just link PRs to issues with "Closes #N".

**Release lag** is the time from when an issue is closed to when the release containing it is published. High release lag points to batch-and-release workflows where completed work sits waiting.

**Cadence** is the time between consecutive releases. Combined with composition (bug ratio, feature ratio), it tells you whether you are shipping improvements or fighting fires.

**Hotfix** is a boolean flag. A release is marked as a hotfix when its cadence is shorter than the configured `hotfix_window_hours` (default: 72 hours).

## Start and end signals

| Your action | What the tool reads | Metric it enables |
| --- | --- | --- |
| Create an issue | `issue.created_at` | Lead time start |
| Apply "in-progress" label | `LABELED_EVENT.createdAt` (immutable) | Cycle time start (issue strategy, preferred) |
| Move issue to "In progress" on project board | `ProjectV2ItemFieldSingleSelectValue.updatedAt` (mutable -- see note below) | Cycle time start (issue strategy, fallback) |
| Open a PR that closes the issue | `PullRequest.createdAt` | Cycle time start (PR strategy) |
| Close the issue | `issue.closed_at` | Lead time end, cycle time end (issue strategy) |
| Merge the closing PR | `PullRequest.mergedAt` | Cycle time end (PR strategy) |
| Publish a release | `release.created_at` | Release lag, cadence |
| Tag without a release | Tag commit date via git refs API | Release lag (less precise) |

{{< hint warning >}}
**Project board timestamps are mutable.** The GitHub Projects v2 API only exposes `updatedAt` on field values -- the timestamp of the *last* status change, not the original transition. If someone moves a card to "Done" after an issue is already closed, the timestamp reflects that post-closure move, producing negative cycle times. This is a fundamental GitHub API limitation. Use labels for cycle time; use the project board for WIP counts and backlog visibility.
{{< /hint >}}

## What you need to do

Most of this is probably part of your workflow already.

**Minimum: close issues with PRs.** If your PRs include "Fixes #42" or "Closes #42" in the description -- or you use GitHub's sidebar to link a PR to an issue -- the tool can compute lead time, cycle time (PR strategy), and release lag. No extra effort required.

**Better: assign issues.** When someone is assigned to an issue, that becomes a cycle time signal. Useful for issues where a PR takes time to create.

**Even better: use labels for lifecycle tracking.** Add an `in-progress` label (or `wip`, `doing`, etc.) to issues when work starts. Configure `lifecycle.in-progress.match` in your config. Label timestamps are immutable, giving you accurate cycle time measurements.

**Best: publish releases.** Publishing GitHub Releases (not just tags) gives the tool precise dates for computing release lag and cadence.

## Choosing a cycle time strategy

| Your workflow | Recommended strategy | Why |
|---------------|---------------------|-----|
| Issues with lifecycle labels | `issue` | Measures real work time (label applied to closed); immutable timestamps |
| Issues on a project board | `issue` + labels | Use labels for cycle time, board for WIP/backlog visibility |
| PRs close issues (most OSS repos) | `pr` | Measures PR review time (created to merged) |
| Issues only, no labels or PRs | `issue` | Lead time works; add an `in-progress` label for cycle time |

To enable `issue` strategy cycle time with labels:

1. Create a label like `in-progress` or `wip` in your repo
2. Add `lifecycle.in-progress.match: ["label:in-progress"]` to your config
3. Apply the label to issues when work starts

Or run preflight to auto-detect:

```bash
gh velocity config preflight -R owner/repo --write
```

## Connecting PRs to issues

The tool finds PR-to-issue connections through GitHub's timeline events. A PR becomes a cycle time signal when it references an issue in any of these ways:

- Write `Fixes #42`, `Closes #42`, or `Resolves #42` in a PR description
- Use GitHub's sidebar "Development" section to link a PR to an issue
- Mention `#42` anywhere in the PR (creates a cross-reference event)
- Any variation: `fix #42`, `close #42`, `resolve #42` (case-insensitive)

The PR does **not** need to be merged, closed, or even out of draft. Opening a draft PR that mentions an issue is enough.

## Solo developers vs. teams

{{< tabs "workflows" >}}
{{< tab "Solo / OSS" >}}
**Solo developer or OSS workflow** (PR strategy):

- Create an issue, open a PR with "Closes #N", merge, tag a release
- Use `cycle_time.strategy: pr` -- works with no extra config

```yaml
# .gh-velocity.yml -- minimal config
cycle_time:
  strategy: pr
quality:
  categories:
    - name: bug
      match: ["label:bug"]
    - name: feature
      match: ["label:enhancement"]
```
{{< /tab >}}
{{< tab "Team with board" >}}
**Team workflow with project board** (issue strategy + labels):

- Create an issue, triage into Backlog, move to In Progress and apply `in-progress` label, open a PR, review, merge, release
- Use `cycle_time.strategy: issue` with `lifecycle.in-progress.match` for cycle time and `project_status` for WIP/backlog

```yaml
# .gh-velocity.yml -- team config with board
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

To automate the label step when someone moves a card on the board, see the project-label-sync workflow in the guide.
{{< /tab >}}
{{< tab "Team without board" >}}
**Team workflow without a project board** (PR strategy):

- Create an issue, developer opens a PR with "Closes #N", review, merge, release
- Use `cycle_time.strategy: pr` -- the PR creation date is the cycle start

```yaml
# .gh-velocity.yml
cycle_time:
  strategy: pr
quality:
  categories:
    - name: bug
      match: ["label:bug"]
    - name: feature
      match: ["label:enhancement"]
```
{{< /tab >}}
{{< /tabs >}}

## What GitHub can and cannot tell you

`gh-velocity` is constrained to the GitHub API. Here is what that means in practice.

{{< details "What works well" >}}
- **Issue lifecycle**: Creation and closure dates are precise. Lead time is reliable.
- **PR merge timestamps**: The search API returns exact merge dates.
- **Closing references**: GitHub tracks which PRs close which issues via the `closingIssuesReferences` GraphQL field.
- **Release metadata**: Tags, release dates, and release bodies are all available via the REST API.
- **Labels**: Issue labels are the basis for classification. Consistent labeling gives you accurate composition metrics.
{{< /details >}}

{{< details "What has limits" >}}
- **Cycle time depends on your strategy.** With no signal for a given issue, cycle time is N/A. The tool warns you when this happens.
- **Project board timestamps are unreliable for cycle time.** See the warning above.
- **The PR search API caps at 1000 results.** Rare outside the largest monorepos.
- **Tag ordering is by API default, not semver.** Use `--since` to specify the previous tag if your tag history is non-linear.
- **"Closed" is not "merged."** Issues can be closed without a PR being merged. The tool treats closure as the end event regardless of cause.
{{< /details >}}

{{< details "What is not possible" >}}
- **Project board transition history.** There is no API for field change history. This is why labels are recommended.
- **Work-in-progress duration as separate phases.** Without transition history, you cannot measure time-in-review or time-in-backlog from the board alone.
- **Developer-level attribution.** The tool measures issue and release velocity, not individual performance. This is intentional.
- **Cross-repo tracking.** Each invocation targets a single repository.
{{< /details >}}

## Next steps

- [Configuration](../configuration/) -- set up your `.gh-velocity.yml`
- [CI Setup](../ci-setup/) -- automate reports with GitHub Actions
