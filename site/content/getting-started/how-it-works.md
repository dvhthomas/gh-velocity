---
title: "How It Works"
weight: 2
---

# How It Works

`gh-velocity` reads artifacts your team already produces -- issues, pull requests, labels, releases -- and turns them into metrics. No data warehouse, no tracking integration, no per-seat subscription. Everything comes from the GitHub API.

Commands are organized by the question they answer:

| Command group | Question it answers | Examples |
|---|---|---|
| `flow` | How fast is work flowing? | `flow lead-time`, `flow cycle-time`, `flow throughput`, `flow velocity` |
| `quality` | Is this code good? | `quality release` |
| `status` | What's happening right now? | `status wip`, `status my-week`, `status reviews` |
| `risk` | Where are structural risks? | `risk bus-factor` |
| `report` | Give me the full picture | Composite dashboard of flow + quality |
| `config` | How do I set this up? | `config preflight`, `config validate`, `config show` |

## The lifecycle of an issue

Every issue follows a **lifecycle** -- stages from creation to completion. Each transition produces a timestamp that maps to a metric:

```
1. You create an issue             -> lead time clock starts
2. Issue gets "in-progress" label  -> cycle time starts (issue strategy)
   OR a PR referencing the issue   -> cycle time starts (PR strategy)
   is created
3. The issue is closed             -> lead time + cycle time clocks stop
4. You publish a release that      -> release lag clock stops
   includes this work
```

Which signal is used depends on your configured **strategy** -- the data source gh-velocity uses for a given metric. Set `cycle_time.strategy` in your [config file]({{< relref "/getting-started/configuration" >}}). See the [strategy comparison table](#choosing-a-cycle-time-strategy) below.

## The metrics

**[Lead time]({{< relref "/reference/metrics/lead-time" >}})** is total elapsed time from issue creation to closure. It includes backlog time, review waits, dependency blocks, and neglect. A long lead time often signals slow prioritization, not slow development.

**[Cycle time]({{< relref "/reference/metrics/cycle-time" >}})** measures how long active work took. Two strategies are available:

- **Issue strategy** (`cycle_time.strategy: issue`): Starts when an in-progress label is applied (`lifecycle.in-progress.match`), ends when the issue closes. Label timestamps are immutable.
- **PR strategy** (`cycle_time.strategy: pr`): Starts when the closing PR is created, ends when it merges. Requires no extra config -- just link PRs to issues with "Closes #N".

**Release lag** is the time from issue closure to the release containing it. High release lag points to batch-and-release workflows where completed work sits waiting. See [Quality Metrics]({{< relref "/reference/metrics/quality" >}}) for the full definition.

**Cadence** is the time between consecutive releases. Combined with composition (bug ratio, feature ratio), it reveals whether you are shipping improvements or fighting fires.

**Hotfix** is a boolean flag. A release is marked as a hotfix when its cadence is shorter than the configured `hotfix_window_hours` (default: 72 hours).

**[Throughput]({{< relref "/reference/metrics/throughput" >}})** counts issues or PRs closed per time window (typically weekly). It answers "how much are we shipping?" without weighting by size. Declining throughput over weeks may signal blockers or context-switching overhead.

**[Velocity]({{< relref "/reference/metrics/velocity" >}})** measures **effort** delivered per iteration. Unlike throughput (which counts items), velocity weights each item by its effort -- using labels like `size:M`, a numeric project field, or a simple item count. Combined with iteration tracking (via a project board field or fixed-length sprints), it shows whether your team's capacity is stable, growing, or declining.

## Scope: which issues are included

Before computing any metric, gh-velocity applies a **scope** -- a filter that determines which issues and PRs are included. Define scope in `scope.query` using GitHub search syntax (e.g., `repo:myorg/myrepo label:team-backend`). The `--scope` flag narrows further at runtime, AND'd with the config scope.

## Start and end signals

| Your action | What the tool reads | Metric it enables |
| --- | --- | --- |
| Create an issue | `issue.created_at` | Lead time start |
| Apply "in-progress" label | `LABELED_EVENT.createdAt` (immutable) | Cycle time start (issue strategy) |
| Open a PR that closes the issue | `PullRequest.createdAt` | Cycle time start (PR strategy) |
| Close the issue | `issue.closed_at` | Lead time end, cycle time end (issue strategy) |
| Merge the closing PR | `PullRequest.mergedAt` | Cycle time end (PR strategy) |
| Publish a release | `release.created_at` | Release lag, cadence |
| Tag without a release | Tag commit date via git refs API | Release lag (less precise) |

> [!TIP]
> **Labels are the sole lifecycle signal.** Label event timestamps are immutable -- once applied, the `createdAt` timestamp never changes. This makes labels the only reliable "when did work start?" source from the GitHub API. Project boards are useful for velocity iteration/effort reads but are not used for lifecycle or cycle-time signals.

## What you need to do

Most of this is probably part of your workflow already.

**Minimum: close issues with PRs.** If your PRs include "Fixes #42" or "Closes #42" in the description -- or you use GitHub's sidebar to link a PR to an issue -- the tool computes lead time, cycle time (PR strategy), and release lag.

**Better: assign issues.** Assignment becomes a cycle time signal, useful for issues where a PR takes time to create.

**Even better: use labels for lifecycle tracking.** Add an `in-progress` label (or `wip`, `doing`, etc.) to issues when work starts. Configure `lifecycle.in-progress.match` in your config. Label timestamps are immutable, giving accurate cycle time measurements.

**Best: publish releases.** GitHub Releases (not just tags) give the tool precise dates for release lag and cadence.

## Choosing a cycle time strategy

The right strategy depends on your workflow:

| Your workflow | Strategy | Config | Trade-offs |
|---|---|---|---|
| Issues with lifecycle labels | `issue` | `lifecycle.in-progress.match: ["label:in-progress"]` | Most reliable timestamps (immutable). Requires label discipline. |
| PRs close issues (most OSS repos) | `pr` | `cycle_time.strategy: pr` | Zero config. Measures PR open-to-merge, not total work time. |

See [Cycle Time reference]({{< relref "/reference/metrics/cycle-time" >}}) for the full signal priority hierarchy.

**Setting up the issue strategy:**

1. Create a label like `in-progress` in your repo
2. Add `lifecycle.in-progress.match: ["label:in-progress"]` to your config
3. Apply the label to issues when work starts

If you use a project board, [gh-project-label-sync](https://github.com/dvhthomas/gh-project-label-sync) can automatically apply lifecycle labels when cards move between columns.

Run `config preflight --write` to auto-detect your setup and generate the right config. See [Cycle Time Setup]({{< relref "/guides/cycle-time-setup" >}}) for a detailed walkthrough.

## Connecting PRs to issues

gh-velocity finds PR-to-issue connections through GitHub's timeline events. A PR becomes a cycle time signal when it references an issue in any of these ways:

- Write `Fixes #42`, `Closes #42`, or `Resolves #42` in a PR description
- Use GitHub's sidebar "Development" section to link a PR to an issue
- Mention `#42` anywhere in the PR (creates a cross-reference event)
- Any variation: `fix #42`, `close #42`, `resolve #42` (case-insensitive)

The PR does **not** need to be merged or out of draft. Opening a draft PR that mentions an issue is enough.

## Solo developers vs. teams

{{< tabs "workflows" >}}
{{< tab "Solo / OSS" >}}
**Solo developer or OSS workflow** (PR strategy):

- Create an issue, open a PR with "Closes #N", merge, tag a release
- Use `cycle_time.strategy: pr` -- no extra config required

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
**Team workflow with labels** (issue strategy):

- Create an issue, apply `in-progress` label when work starts, open a PR, review, merge, release
- Use `cycle_time.strategy: issue` with `lifecycle.in-progress.match`

```yaml
# .gh-velocity.yml -- team config with labels
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

If you use a project board, [gh-project-label-sync](https://github.com/dvhthomas/gh-project-label-sync) automates the label step when cards move between columns.
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

`gh-velocity` is constrained to the GitHub API. Here is what that means in practice:

{{% details "What works well" %}}
- **Issue lifecycle**: Creation and closure dates are precise. Lead time is reliable.
- **PR merge timestamps**: The search API returns exact merge dates.
- **Closing references**: GitHub tracks which PRs close which issues via the `closingIssuesReferences` GraphQL field.
- **Release metadata**: Tags, release dates, and release bodies are all available via the REST API.
- **Labels**: Labels are the basis for classification. Consistent labeling gives accurate composition metrics.
{{% /details %}}

{{% details "What has limits" %}}
- **Cycle time depends on your strategy.** With no signal for a given issue, cycle time is N/A. The tool warns when this happens.
- **The PR search API caps at 1000 results.** Rare outside the largest monorepos.
- **Tag ordering is by API default, not semver.** Use `--since` to specify the previous tag if your tag history is non-linear.
- **"Closed" is not "merged."** Issues can be closed without a PR being merged. The tool treats closure as the end event regardless of cause.
{{% /details %}}

{{% details "What is not possible" %}}
- **Project board transition history.** No API exists for field change history. This is why labels are used for lifecycle.
- **Work-in-progress duration as separate phases.** Without transition history, you cannot measure time-in-review or time-in-backlog from the board alone. Labels partially address this.
- **Developer-level attribution.** The tool measures issue and release velocity, not individual performance. This is intentional.
- **Cross-repo tracking.** Each invocation targets a single repository.
{{% /details %}}

## Next steps

- [Configuration]({{< relref "/getting-started/configuration" >}}) -- set up your `.gh-velocity.yml`
- [CI Setup]({{< relref "/getting-started/ci-setup" >}}) -- automate reports with GitHub Actions
- [Interpreting Results]({{< relref "/guides/interpreting-results" >}}) -- understand what "good" looks like for each metric
- [Understanding Statistics]({{< relref "/concepts/statistics" >}}) -- median, percentiles, outlier detection explained
