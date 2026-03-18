# gh-velocity guide

## Why this exists

Engineering teams need to know how fast they ship and where the bottlenecks are. Most tools that answer these questions require a separate data warehouse, a tracking integration, or a per-seat subscription. `gh-velocity` takes a different approach: it computes metrics directly from the data GitHub already has.

Your issues have creation and closure dates. Your pull requests have merge timestamps and closing references. Your releases have tags and bodies. That is enough to measure lead time, cycle time, release lag, and composition — per issue, per release, and across releases.

The tradeoff is clear: you get zero-setup velocity metrics in exchange for being constrained to what the GitHub API can tell you. This guide explains exactly what that means and where the edges are.

## What the metrics mean

**Lead time** is the duration from when an issue is created to when it is closed. It measures the total elapsed time a piece of work existed, including time spent in backlog, waiting for review, blocked by dependencies, or simply forgotten. A long lead time does not necessarily mean slow development — it often means slow prioritization.

**Cycle time** measures how long active work took. There are two strategies:

- **Issue strategy** (`cycle_time.strategy: issue`): Starts when the issue is labeled with an in-progress label (`lifecycle.in-progress.match`), ends when the issue is closed. Can optionally fall back to project board status, but labels are strongly recommended (see [Why labels over project board](#why-labels-over-project-board-for-cycle-time) below).
- **PR strategy** (`cycle_time.strategy: pr`): Starts when the closing PR is created, ends when it is merged. Works with no extra config — just link PRs to issues with "Closes #N".

The strategy is configured in `.gh-velocity.yml`. If unsure, start with `pr` — it works immediately.

**Release lag** is the duration from when an issue is closed to when the release containing it is published. It measures how long finished work waits before reaching users. High release lag often points to batch-and-release workflows where completed work sits in a staging branch.

**Cadence** is the time between consecutive releases. It is not a per-issue metric but a release-level observation. Cadence combined with composition (bug ratio, feature ratio) tells you whether you are shipping improvements or fighting fires.

**Hotfix** is a boolean flag. A release is marked as a hotfix when its cadence is shorter than the configured `hotfix_window_hours` (default: 72 hours). This lets you separate planned releases from emergency patches in your analysis.

## How your GitHub workflow becomes metrics

`gh-velocity` reads the artifacts your team already produces — issues, pull requests, assignments, releases — and turns them into metrics. Here is the typical workflow and what the tool extracts at each step.

### The lifecycle of an issue

```
1. You create an issue           → lead time clock starts
2. Issue gets "in-progress"      → cycle time starts (issue strategy)
   label applied
   OR a PR referencing the issue → cycle time starts (PR strategy)
   is created
3. The issue is closed           → lead time + cycle time clocks stop
4. You publish a release that    → release lag clock stops
   includes this work
```

Which cycle time signal is used depends on your configured strategy (`cycle_time.strategy` in `.gh-velocity.yml`).

### Start and end signals

**Lead time** always starts when the issue is created and ends when the issue is closed.

**Cycle time** depends on the configured strategy:
- **Issue strategy**: Starts when an in-progress label is applied to the issue (via `lifecycle.in-progress.match`), ends when closed. Falls back to project board status if no matching label is found, but label timestamps are far more reliable (see [why](#why-labels-over-project-board-for-cycle-time)).
- **PR strategy**: Starts when the closing PR is created, ends when the PR is merged.

### What you need to do (and what you probably already do)

**Minimum requirement: close issues with PRs.** If your PRs include "Fixes #42" or "Closes #42" in the description — or you use GitHub's sidebar to link a PR to an issue — the tool can compute lead time, cycle time, and release lag. This is the most common GitHub workflow and requires no extra effort. The PR does not need to be merged or even finished — a draft PR referencing an issue is enough for a cycle time signal.

**Better: assign issues.** When someone is assigned to an issue, that becomes a cycle time signal. This is useful for issues where a PR takes time to create — the assignment marks when work actually began.

**Even better: use labels for lifecycle tracking.** Add an `in-progress` label (or `wip`, `doing`, etc.) to issues when work starts. Configure `lifecycle.in-progress.match` in `.gh-velocity.yml` to match these labels. Label timestamps are **immutable** — once applied, the `createdAt` timestamp never changes, giving you accurate cycle time measurements.

**Also valuable: use a Projects v2 board for workflow visibility.** Project boards are excellent for tracking current status (WIP counts, backlog size) and filtering. However, **do not rely on project board status alone for cycle time** — the GitHub API only exposes the field's last-modified timestamp, not when the status was originally set. See [Why labels over project board](#why-labels-over-project-board-for-cycle-time) for details. When you use a board, configure both `project_status` (for WIP/backlog) and `match` (for cycle time).

**Best: use releases.** Publishing GitHub Releases (not just tags) gives the tool precise dates for computing release lag and cadence. If you only push tags, the tool resolves dates from the tag's commit — which works but is less precise.

### What the tool reads at each level

| Your action | What the tool reads | Metric it enables |
| --- | --- | --- |
| Create an issue | `issue.created_at` | Lead time start |
| Apply "in-progress" label | `LABELED_EVENT.createdAt` (immutable) | Cycle time start (issue strategy, preferred) |
| Move issue to "In progress" on project board | `ProjectV2ItemFieldSingleSelectValue.updatedAt` (mutable — see [caveat](#why-labels-over-project-board-for-cycle-time)) | Cycle time start (issue strategy, fallback) |
| Open a PR that closes the issue | `PullRequest.createdAt` | Cycle time start (PR strategy) |
| Close the issue | `issue.closed_at` | Lead time end, cycle time end (issue strategy) |
| Merge the closing PR | `PullRequest.mergedAt` | Cycle time end (PR strategy) |
| Publish a release | `release.created_at` | Release lag, cadence |
| Tag without a release | Tag commit date via git refs API | Release lag (less precise) |

### Choosing a cycle time strategy

| Your workflow | Recommended strategy | Why |
|---------------|---------------------|-----|
| Issues with lifecycle labels | `issue` | Measures real work time (label applied → closed); immutable timestamps |
| Issues on a project board | `issue` + labels | Use labels for cycle time, board for WIP/backlog visibility |
| PRs close issues (most OSS repos) | `pr` | Measures PR review time (created → merged) |
| Issues only, no labels or PRs | `issue` | Lead time works; add an `in-progress` label for cycle time |

If you're unsure, start with `pr` — it works immediately with no extra config.

To enable `issue` strategy cycle time with labels:

1. Create a label like `in-progress` or `wip` in your repo
2. Add `lifecycle.in-progress.match: ["label:in-progress"]` to your config
3. Apply the label to issues when work starts

Or run preflight to auto-detect:

```bash
gh velocity config preflight -R owner/repo --write
```

### Configuring the issue strategy

The issue strategy uses labels as the primary cycle time signal. Configure which labels mark "work started":

```yaml
# .gh-velocity.yml — recommended: labels for cycle time
lifecycle:
  in-progress:
    match: ["label:in-progress", "label:wip"]
```

When any matching label is applied to an issue, that timestamp becomes the cycle time start. Label event timestamps (`LABELED_EVENT.createdAt`) are **immutable** — they never change once the label is applied, so you get accurate cycle time measurements.

**If you also use a project board**, configure both signals. The board gives you WIP counts and backlog filtering; labels give you reliable cycle time:

```yaml
# .gh-velocity.yml — recommended: labels + project board
project:
  url: "https://github.com/users/yourname/projects/1"
  status_field: "Status"

lifecycle:
  backlog:
    project_status: ["Backlog", "Triage"]   # used for backlog detection and WIP
  in-progress:
    project_status: ["In progress"]          # used for WIP detection
    match: ["label:in-progress"]             # used for cycle time (preferred)
```

When both `match` and `project_status` are configured, labels take priority for cycle time. Project board status is used as a fallback if no matching label is found, and it continues to power WIP and backlog detection.

> **Warning**: If you configure only `project_status` without `match`, the tool emits a deprecation warning. Project board timestamps are unreliable for cycle time — see [Why labels over project board](#why-labels-over-project-board-for-cycle-time).

Run `gh velocity config discover -R owner/repo` to find your project URL, status field name, and available status values. Run `gh velocity config preflight -R owner/repo --write` to auto-detect labels and generate a complete config.

### Configuring the PR strategy

The PR strategy requires no extra config. It uses the closing PR's creation date as the cycle start and its merge date as the end:

```yaml
# .gh-velocity.yml
cycle_time:
  strategy: pr
```

Ensure your PRs reference issues with "Closes #N" or "Fixes #N" so the tool can link them.

Lead time is unaffected by strategy choice — it always measures the full elapsed time from issue creation to close.

### Solo developers vs. teams

**Solo developer / OSS workflow** (PR strategy):
- Create an issue → open a PR with "Closes #N" → merge → tag a release
- Use `cycle_time.strategy: pr`. Works with no extra config.

**Team workflow with project board** (issue strategy + labels):
- Create an issue → triage into Backlog → move to In Progress and apply `in-progress` label → open a PR → review → merge → release
- Use `cycle_time.strategy: issue` with `lifecycle.in-progress.match` for cycle time and `project_status` for WIP/backlog. The label application is the cycle start.
- To automate the label step, use a GitHub Actions workflow triggered by `projects_v2_item` events (see [Syncing project board status to labels](#syncing-project-board-status-to-labels)).

**Team workflow without project board** (PR strategy):
- Create an issue → developer opens a PR with "Closes #N" → review → merge → release
- Use `cycle_time.strategy: pr`. The PR creation date is the cycle start.

### Connecting PRs to issues

The tool finds PR-to-issue connections through GitHub's timeline events. A PR becomes a cycle time signal when it references an issue in any of these ways:

- Write `Fixes #42`, `Closes #42`, or `Resolves #42` in a PR description
- Use GitHub's sidebar "Development" section to link a PR to an issue
- Mention `#42` anywhere in the PR (creates a cross-reference event)
- Any variation: `fix #42`, `close #42`, `resolve #42` (case-insensitive)

The PR does **not** need to be merged, closed, or even out of draft. Opening a draft PR that mentions an issue is enough.

You do **not** need to:
- Add special labels or tags
- Use a specific branch naming convention
- Configure webhooks or integrations
- Follow any commit message format (unless you want commit-based enrichment)

## What GitHub can and cannot tell you

`gh-velocity` is constrained to the GitHub API. Here is what that means in practice.

### What works well

- **Issue lifecycle**: Creation and closure dates are precise. Lead time is reliable.
- **PR merge timestamps**: The search API returns exact merge dates. The `pr-link` strategy uses these to find PRs in a release window.
- **Closing references**: GitHub tracks which PRs close which issues. The GraphQL `closingIssuesReferences` field is the most reliable way to connect PRs to issues.
- **Release metadata**: Tags, release dates, and release bodies are all available via the REST API.
- **Labels**: Issue labels are the basis for bug/feature classification. If your team labels issues consistently, composition metrics are accurate.

### What has limits

- **Cycle time depends on your configured strategy**. With `pr` strategy, the tool uses the closing PR's creation → merge dates. With `issue` strategy, it prefers label events (`lifecycle.in-progress.match`) and falls back to project board status. If neither strategy has a signal for a given issue, cycle time is N/A. The tool warns you when this happens.
- **Project board timestamps are unreliable for cycle time**. The GitHub Projects v2 API exposes only `updatedAt` on field values — the timestamp of the **last** status change, not the original transition. If someone moves a card to "Done" after an issue is closed, `updatedAt` reflects that post-closure move, producing `start > end` and **negative cycle times**. This is a fundamental GitHub API limitation — there is no field change history API. The tool filters negative durations from aggregate statistics and warns you, but the root cause cannot be fixed without label-based timestamps. See [Why labels over project board](#why-labels-over-project-board-for-cycle-time).
- **The PR search API caps at 1000 results**. If a release window contains more than 1000 merged PRs, the `pr-link` strategy warns you and returns partial results. This is rare outside the largest monorepos.
- **Tag ordering is by API default, not semver**. Tags are returned in the order GitHub's API provides, which is usually creation date. The tool picks the tag immediately before your target tag in this list. If your tag history is non-linear, use `--since` to specify the previous tag explicitly.
- **"Closed" is not "merged"**. GitHub issues can be closed without a PR being merged — by a maintainer, a bot, or the author. `gh-velocity` treats closure as the end event regardless of cause. For most teams this is fine; for teams that close stale issues aggressively, it may inflate lead time counts.
- **Label-based classification is only as good as your labels**. If more than half the issues in a release lack bug/feature labels, the tool warns you. You can customize which labels map to which categories in the config file.

### What is not possible

- **Project board transition history**. GitHub Projects v2 has no API for field change history. You cannot query "when did this issue move to In Progress?" — only "what is the current status, and when was it last modified?" This is why label events are the recommended cycle time signal: `LABELED_EVENT.createdAt` is immutable and records the exact moment a label was applied.
- **Work-in-progress duration as separate phases**. Without transition history, there is no way to measure time-in-review or time-in-backlog as separate phases using project board data alone. Labels partially address this — you could use separate labels for each phase (`in-review`, `blocked`) and measure durations between label events.
- **Developer-level attribution**. The tool measures issue and release velocity, not individual performance. This is intentional.
- **Cross-repo tracking**. Each invocation targets a single repository. Multi-repo releases require separate runs.

## Getting started

### Prerequisites

Install the [GitHub CLI](https://cli.github.com/):

```bash
# macOS
brew install gh

# Linux
sudo apt install gh    # Debian/Ubuntu
sudo dnf install gh    # Fedora

# Windows
winget install GitHub.cli
```

Authenticate:

```bash
gh auth login
```

You need at least `repo` scope for private repositories. For public repos, no special scopes are required.

### Install the extension

```bash
# Latest stable release
gh extension install dvhthomas/gh-velocity

# Or pin a specific version
gh extension install dvhthomas/gh-velocity --pin v0.0.2
```

Verify:

```bash
gh velocity version
```

To upgrade later:

```bash
gh extension upgrade velocity
```

`gh extension upgrade` installs the latest stable release. Pre-releases (e.g., `v0.0.2-rc.1`) must be pinned explicitly with `--pin`.

### First queries

All metric commands require a config file. When targeting a remote repo with `-R`, use `--config` to point at an example config (see `docs/examples/`). From inside your own repo with `.gh-velocity.yml` present, the config is loaded automatically.

Start with a public repo to see what the output looks like:

```bash
# Release report for the GitHub CLI itself
gh velocity quality release v2.67.0 -R cli/cli
```

This takes 10-30 seconds depending on the number of issues. You will see:

- Release metadata (previous tag, cadence, hotfix status)
- Composition breakdown (bugs, features, other)
- Per-issue table with lead time, cycle time, release lag, and outlier flags
- Aggregate statistics with P90, P95, and outlier counts

Try the same report in JSON to see the full data:

```bash
gh velocity quality release v2.67.0 -R cli/cli -f json | jq '.aggregates.lead_time'
```

```json
{
  "count": 17,
  "mean_seconds": 24271200,
  "median_seconds": 5248800,
  "stddev_seconds": 43981056,
  "p90_seconds": 134236800,
  "p95_seconds": 138499200,
  "outlier_cutoff_seconds": 119448000,
  "outlier_count": 2
}
```

See which strategy found each issue:

```bash
gh velocity quality release v2.67.0 -R cli/cli --discover
```

This shows what `pr-link`, `commit-ref`, and `changelog` each discovered, and the merged result. Use this to understand how well the strategies cover your workflow.

### Your own repo

From inside a local checkout, you can omit `-R`:

```bash
cd your-repo
gh velocity quality release v1.0.0
```

When run from inside a repo, the tool uses local git for tag listing and commit history. This is faster and enables cycle-time computation.

### Cycle time works remotely

`cycle-time` does not require a local clone. It detects when work started using GitHub API signals:

```bash
# Works without cloning — uses PR creation date, assignment, or project status
gh velocity flow cycle-time 42 -R cli/cli
```

If you run from inside a local checkout, the tool also counts commits referencing the issue and can use the earliest commit as a fallback start signal.

```bash
# From inside a clone — enriched with commit count and fallback signal
cd cli
gh velocity flow cycle-time 42
```

To enable label-based cycle time (issue strategy), add lifecycle configuration:

```yaml
# .gh-velocity.yml — labels for reliable cycle time
lifecycle:
  in-progress:
    match: ["label:in-progress", "label:wip"]
```

With this config and `cycle_time.strategy: issue`, the tool checks if the issue has an in-progress label and uses the label's immutable `createdAt` timestamp as the cycle start.

If you also use a project board, add both signals:

```yaml
# .gh-velocity.yml — labels + board (recommended for board users)
project:
  url: "https://github.com/users/yourname/projects/1"
  status_field: "Status"

lifecycle:
  backlog:
    project_status: ["Backlog", "Triage"]
  in-progress:
    project_status: ["In progress"]          # WIP/backlog detection
    match: ["label:in-progress"]             # cycle time (preferred)
```

Run `gh velocity config preflight -R owner/repo --write` to generate this config automatically.

In **GitHub Actions**, set `fetch-depth: 0` if you want commit enrichment:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0    # enables commit count and fallback cycle-time signal
```

The tool detects shallow clones and warns you.

## Configuration reference

A `.gh-velocity.yml` file is required for all metric commands. Create one in your repository root, or use `--config` to point to an alternate path. Every field within the config is optional — the tool uses sensible defaults for anything you omit.

### Minimal config

```yaml
quality:
  categories:
    - name: bug
      match:
        - "label:bug"
    - name: feature
      match:
        - "label:enhancement"
```

This is equivalent to the defaults. You only need a config file if you want to change something. Run `gh velocity config preflight -R owner/repo` to generate a config tailored to your repo.

### Full config

```yaml
# How your team works. "pr" means PRs close issues (most teams).
# "local" means direct commits to main (solo projects, scripts).
workflow: pr

# Scope: which issues/PRs to analyze (GitHub search query syntax).
scope:
  query: "repo:myorg/myrepo"

# Issue/PR classification — first matching category wins; unmatched = "other".
# Matchers: label:<name>, type:<name>, title:/<regex>/i
# Note: bug ratio in reports counts issues classified as "bug".
# If you name your bug category differently, use "bug" as the name.
quality:
  categories:
    - name: bug
      match:
        - "label:bug"
        - "label:defect"
        - "type:Bug"
    - name: feature
      match:
        - "label:enhancement"
        - "type:Feature"
    - name: chore
      match:
        - "label:tech-debt"
        - "title:/^chore[\\(: ]/i"
    - name: docs
      match:
        - "label:documentation"
        - "label:docs"
  hotfix_window_hours: 48        # releases within 48h of previous = hotfix

# Commit message scanning
commit_ref:
  patterns: ["closes"]           # default: only closing keywords
  # patterns: ["closes", "refs"]   # also match bare #N references

# Cycle time strategy: "issue" (default) or "pr"
# Issue strategy uses lifecycle.in-progress.match (labels) to detect "work started."
# Falls back to project board status if no label match — but labels are recommended.
# PR strategy uses PR created → merged (works with no extra config).
cycle_time:
  strategy: pr

# GitHub Projects v2 — enables WIP command and backlog detection.
# Note: project board status is NOT recommended as the sole cycle time signal.
# Use lifecycle.in-progress.match (labels) for reliable cycle time timestamps.
project:
  url: "https://github.com/users/yourname/projects/1"
  status_field: "Status"

# Lifecycle stages: map labels and/or project board columns to workflow stages.
# match: recommended for cycle time — label timestamps are immutable and reliable.
# project_status: used for WIP detection and backlog filtering (board column names).
# Run: gh velocity config preflight -R owner/repo --write
lifecycle:
  backlog:
    project_status: ["Backlog", "Triage"]
  in-progress:
    project_status: ["In progress"]            # WIP detection
    match: ["label:in-progress", "label:wip"]  # cycle time (preferred signal)

# Exclude bot accounts from metrics.
exclude_users:
  - "dependabot[bot]"
  - "renovate[bot]"

# GitHub Discussions integration (for --post on bulk commands)
discussions:
  category: General
```

### Configuration details

**`quality.categories`**: Ordered list of classification categories. Each category has a `name` and a list of `match` rules. The first matching category wins; unmatched issues are classified as "other." When more than 50% of issues are "other," the tool warns about low classification coverage. Matcher types: `label:<name>` (exact label match), `type:<name>` (GitHub Issue Type), `title:/<regex>/i` (title regex, case-insensitive).

**`quality.hotfix_window_hours`**: A release is flagged as a hotfix if it was published within this many hours of the previous release. Default is 72 (3 days). Maximum is 8760 (1 year). Set this lower if your team has a fast release cadence.

**`commit_ref.patterns`**: Controls how the `commit-ref` strategy scans commit messages.

- `["closes"]` (default): Only matches closing keywords — `fixes #N`, `closes #N`, `resolves #N` and their variations. This is conservative and avoids false positives from comments like "see #42" or "step #1".
- `["closes", "refs"]`: Also matches bare `#N` references. Use this if your team writes commits like "implement #42" without closing keywords. Be aware that this can match false positives like "update step #1."

**`project.url`**: GitHub Projects v2 URL (e.g., `https://github.com/users/yourname/projects/1`). Required when lifecycle stages use `project_status`. Enables the `wip` command and backlog detection. For cycle time, prefer `lifecycle.in-progress.match` (labels) over `project_status` — see [Why labels over project board](#why-labels-over-project-board-for-cycle-time). Find your project URL by navigating to your project board in GitHub.

**`project.status_field`**: The visible name of the status field on your project board (usually "Status"). Required when lifecycle stages use `project_status`. Run `gh velocity config discover` to find available fields and options.

**Unknown keys** in the config file produce warnings to stderr but do not cause errors. This lets you add comments or future fields without breaking the tool.

### Validating your config

```bash
gh velocity config validate
```

This checks all fields for correct types, valid ranges, and proper GraphQL ID formats. It does not make any API calls.

To see the resolved configuration with all defaults applied:

```bash
gh velocity config show
gh velocity config show -f json
```

## Linking strategies in depth

The `quality release` and `quality release --discover` commands need to determine which issues belong to a release. This is harder than it sounds — different teams use different workflows, and no single method works everywhere. `gh-velocity` uses three strategies and merges the results.

### pr-link

The highest-fidelity strategy. It:

1. Searches for PRs merged between the previous tag date and the target tag date
2. Queries each PR's `closingIssuesReferences` via GraphQL
3. Returns issues with full metadata (title, labels, dates)

This works well for teams that use "Fixes #N" in PR descriptions or GitHub's sidebar linking. It requires that your tags correspond to GitHub Releases (or at least that the tag's commit has a date).

**Limitation**: The GitHub search API returns at most 1000 results per query. If your release window contains more than 1000 merged PRs, results are partial. The tool warns when this happens.

### commit-ref

Scans the commit messages between two tags for issue references. By default, it only matches closing keywords:

```
fixes #42
Closes #10
RESOLVED #99
```

With `patterns: ["closes", "refs"]` in your config, it also matches bare references:

```
implement #42
update #7
```

Commits are grouped by issue number. If three commits all reference `#42`, the tool returns one item with three associated commits.

### changelog

Parses the GitHub Release body (the release notes text) for `#N` references. This catches issues that are mentioned in release notes but not linked via PRs or commit messages.

This strategy is low-fidelity — it only finds the issue number, not the full issue data. The tool fetches issue details separately.

### How merge works

Results from all three strategies are combined using priority-based deduplication:

1. **pr-link** has highest priority (most data, highest confidence)
2. **commit-ref** is next
3. **changelog** is lowest

When the same issue number appears in multiple strategies, the highest-priority version wins. This means pr-link's rich data (PR reference, full issue metadata) is preferred over commit-ref's issue-number-only data.

The `--discover` flag shows this merge in action:

```bash
gh velocity quality release v1.2.0 --discover
```

The output lists what each strategy found and marks items that appear in multiple strategies with "(also: commit-ref)" annotations.

## Use with an agent

Every command supports `-f json` for structured output. This makes `gh-velocity` composable with LLM agents, CI scripts, and data pipelines.

### Agent integration patterns

An agent that reviews releases:

```bash
# Get the full release report as JSON
REPORT=$(gh velocity quality release v1.2.0 -R owner/repo -f json)

# Feed to an agent for analysis
echo "$REPORT" | your-agent analyze-release
```

Extracting specific data with jq:

```bash
# Which issues are outliers?
gh velocity quality release v1.2.0 -f json | \
  jq '[.issues[] | select(.lead_time_outlier) | {number, title, lead_time_seconds}]'

# What percentage are bugs?
gh velocity quality release v1.2.0 -f json | \
  jq '.composition | "\(.bug_count)/\(.total_issues) bugs (\(.bug_ratio * 100 | round)%)"'

# P95 lead time in days
gh velocity quality release v1.2.0 -f json | \
  jq '.aggregates.lead_time.p95_seconds / 86400 | round | "\(.) days"'
```

### Posting to GitHub

The markdown format is designed for pasting into issues, PRs, or discussions:

```bash
# Generate a release summary and post it as an issue comment
gh velocity quality release v1.2.0 -f markdown | \
  gh issue comment 100 --body-file -

# Or create a new issue with the report
gh velocity quality release v1.2.0 -f markdown | \
  gh issue create --title "Release v1.2.0 metrics" --body-file -
```

### Claude Code / Copilot agent example

If you use an agent that can run shell commands, point it at your repo:

```
You have access to `gh velocity`. Use it to analyze our last 3 releases
and identify trends in lead time and bug ratio.

Commands available:
  gh velocity quality release<tag> -f json
  gh velocity quality release <tag> --discover -f json
  gh velocity flow lead-time<issue> -f json

Our recent tags: v2.5.0, v2.4.0, v2.3.0
```

The JSON output includes every field an agent needs: seconds-based durations, ratios as floats, boolean flags, and descriptive warnings.

## CI integration

### How authentication works

gh-velocity uses the `GH_TOKEN` environment variable for all GitHub API calls — the same variable that powers the `gh` CLI. Locally, `gh auth login` handles this automatically. In CI, you set `GH_TOKEN` in your workflow.

### Token permissions

The default `GITHUB_TOKEN` provided by GitHub Actions works for most gh-velocity commands. However, **`GITHUB_TOKEN` cannot access Projects v2 boards** — this is a GitHub platform limitation, not a gh-velocity limitation.

gh-velocity handles this with the `GH_VELOCITY_TOKEN` environment variable. When set, the binary automatically uses it instead of `GH_TOKEN` for all API calls — no workflow fallback logic needed. Just pass both:

```yaml
env:
  GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
```

Here's what each token can do:

| Capability | `GITHUB_TOKEN` only | + `GH_VELOCITY_TOKEN` |
| --- | --- | --- |
| Lead time, throughput | Yes | Yes |
| Release quality metrics | Yes | Yes |
| Bus factor | Yes | Yes |
| `--post` to issues/PRs | Yes | Yes |
| `--post` to Discussions | Yes | Yes |
| **Cycle time (issue strategy)** | **No** — requires project board | **Yes** |
| **WIP** | **No** — requires project board | **Yes** |
| **Reviews** | Yes | Yes |

**If your config has no `project:` section**, `GITHUB_TOKEN` is all you need.

**If your config has a `project:` section**, commands that need the board (cycle time with issue strategy, WIP) will warn and skip that data — the rest of the report still works. To get full metrics, set up `GH_VELOCITY_TOKEN`.

### Setting up GH_VELOCITY_TOKEN for CI

1. **Create a classic PAT** with `project` (read-only) scope:

   [Create token](https://github.com/settings/tokens/new?scopes=project&description=gh-velocity) — this link pre-fills the scope and description.

   > Fine-grained PATs do not currently support user-owned projects. Use a classic PAT for user projects, or a GitHub App for organization projects.

2. **Add it as a repository secret** named `GH_VELOCITY_TOKEN`:

   Go to your repo → Settings → Secrets and variables → Actions → New repository secret.

3. **Pass it in your workflow** alongside `GH_TOKEN`:

   ```yaml
   env:
     GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
     GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
   ```

   The binary prefers `GH_VELOCITY_TOKEN` when set. If it's empty or missing, it falls back to `GH_TOKEN` transparently. No workflow expressions needed — the binary handles the logic.

### Workflow permissions

Your workflow needs explicit `GITHUB_TOKEN` permissions for `--post` to write back to GitHub:

```yaml
permissions:
  contents: read          # read repo and config
  issues: write           # --post comments on issues/PRs
  discussions: write      # --post bulk reports as Discussions
```

These are `GITHUB_TOKEN` permissions (set in the workflow file). `GH_VELOCITY_TOKEN` only needs the `project` scope — it inherits read access to public repos automatically.

### GitHub Actions: weekly report

Post a velocity report to Discussions every week:

```yaml
name: Velocity Report

on:
  schedule:
    - cron: '0 9 * * 1'  # Monday 9am UTC
  workflow_dispatch:      # allow manual runs

permissions:
  contents: read
  issues: write
  discussions: write

jobs:
  report:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - run: gh extension install dvhthomas/gh-velocity

      - name: Post velocity report
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
          GH_VELOCITY_POST_LIVE: 'true'
        run: gh velocity report --since 7d --post -f markdown
```

### GitHub Actions: release metrics comment

Post a quality report on every release:

```yaml
name: Release Metrics

on:
  release:
    types: [published]

permissions:
  contents: read
  issues: write

jobs:
  metrics:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0    # full history for accurate commit analysis

      - run: gh extension install dvhthomas/gh-velocity

      - name: Post release metrics
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
        run: |
          gh velocity quality release ${{ github.event.release.tag_name }} -f markdown > report.md
          gh issue create \
            --title "Metrics: ${{ github.event.release.tag_name }}" \
            --body-file report.md \
            --label "metrics"
```

### GitHub Actions: PR lead-time check

Add lead-time context to PRs that close issues:

```yaml
name: Lead Time Context

on:
  pull_request:
    types: [opened, edited]

permissions:
  contents: read
  pull-requests: write

jobs:
  lead-time:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - run: gh extension install dvhthomas/gh-velocity

      - name: Extract linked issue
        id: issue
        run: |
          # Parse PR body for "Fixes #N" or "Closes #N"
          ISSUE=$(echo "${{ github.event.pull_request.body }}" | \
            grep -oiE '(fixes|closes|resolves)\s+#[0-9]+' | \
            grep -oE '[0-9]+' | head -1)
          echo "number=$ISSUE" >> "$GITHUB_OUTPUT"

      - name: Report lead time
        if: steps.issue.outputs.number != ''
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
        run: |
          gh velocity flow lead-time ${{ steps.issue.outputs.number }} -f markdown | \
            gh pr comment ${{ github.event.pull_request.number }} --body-file -
```

### Scheduled trend reports

Capture metrics as build artifacts for trend analysis:

```yaml
name: Weekly Velocity

on:
  schedule:
    - cron: '0 9 * * 1'   # Monday 9am UTC

jobs:
  report:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - run: gh extension install dvhthomas/gh-velocity

      - name: Latest release metrics
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
        run: |
          TAG=$(git describe --tags --abbrev=0)
          gh velocity quality release "$TAG" -f json > metrics.json
          gh velocity quality release "$TAG" -f markdown > metrics.md

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: velocity-metrics
          path: metrics.json
```

## How-to recipes

### Compare two releases

```bash
gh velocity quality release v2.0.0 -f json > v2.json
gh velocity quality release v1.9.0 -f json > v1.json

# Compare lead times
echo "v1.9.0 median lead time: $(jq -r '.aggregates.lead_time.median_seconds / 86400 | round | "\(.)d"' v1.json)"
echo "v2.0.0 median lead time: $(jq -r '.aggregates.lead_time.median_seconds / 86400 | round | "\(.)d"' v2.json)"
```

### Find your slowest issues

```bash
gh velocity quality release v1.2.0 -f json | \
  jq -r '.issues | sort_by(-.lead_time_seconds) | .[0:5] | .[] |
    "#\(.number) \(.title[0:40]) — \(.lead_time_seconds / 86400 | round)d"'
```

### Check label coverage before a release

```bash
gh velocity quality release v1.2.0 -f json | \
  jq '"Bug: \(.composition.bug_count), Feature: \(.composition.feature_count), Unlabeled: \(.composition.other_count)"'
```

If `other_count` is high, label your issues before publishing the release for more useful metrics.

### Use --since to narrow scope

When the auto-detected previous tag is wrong (non-linear tag history, pre-releases), override it:

```bash
gh velocity quality release v2.0.0 --since v1.9.0
gh velocity quality release v2.0.0 --since v1.9.0 --discover
```

### Analyze a repo you don't have locally

Every command works with `-R`:

```bash
gh velocity quality release v0.28.0 -R charmbracelet/bubbletea
gh velocity flow lead-time 500 -R charmbracelet/bubbletea
gh velocity quality release v5.2.1 -R go-chi/chi --discover
```

All commands work remotely. Cycle time uses API-based signals (PR creation date, first assignment). A local checkout adds commit counts and a fallback signal from commit history.

### Generate a report for every release

```bash
for tag in $(gh api repos/owner/repo/tags --jq '.[].name' | head -5); do
  echo "=== $tag ==="
  gh velocity quality release "$tag" -R owner/repo 2>/dev/null
  echo
done
```

### Export to CSV for spreadsheet analysis

```bash
gh velocity quality release v1.2.0 -f json | \
  jq -r '["number","title","lead_time_days","cycle_time_days","outlier"],
    (.issues[] | [
      .number,
      .title,
      ((.lead_time_seconds // 0) / 86400 | round),
      ((.cycle_time_seconds // 0) / 86400 | round),
      .lead_time_outlier
    ]) | @csv' > release-metrics.csv
```

## Understanding the statistics

### Why median over mean

Lead times are almost always right-skewed: most issues close in days, but a few ancient issues get closed during a release and inflate the mean. The median is a better measure of "typical" for your team.

Example from cli/cli v2.67.0:
- Mean lead time: 280 days
- Median lead time: 60 days

The mean is 4.6x the median because two issues open for 4+ years were closed in this release. The median tells you the typical issue takes about 2 months.

### P90 and P95

"95% of issues in this release shipped within P95 days." These are useful for setting expectations or SLAs. A P95 lead time of 30 days means only 1 in 20 issues takes longer than a month.

Percentiles require at least 5 data points. Below that threshold, the values are omitted rather than computed from too little data.

### Outlier detection

The tool uses the interquartile range (IQR) method:

1. Compute Q1 (25th percentile) and Q3 (75th percentile)
2. IQR = Q3 - Q1
3. Outlier threshold = Q3 + 1.5 * IQR
4. Any value above the threshold is flagged

This is the same method used in box plots. It adapts to your data — a team with consistently long lead times will have a higher threshold than a team that ships fast.

Outlier detection requires at least 4 data points. Individual issues are flagged with `OUTLIER` in pretty and markdown output, and `lead_time_outlier: true` in JSON.

### Standard deviation

Sample standard deviation (N-1 denominator) measures spread. It is most useful as a ratio with the mean: if `stddev / mean` is greater than 1, your delivery times are highly variable. Consistent teams have low relative standard deviation.

Standard deviation requires at least 2 data points.

## Why labels over project board for cycle time

This section explains a fundamental limitation of the GitHub Projects v2 API and why `gh-velocity` recommends labels for cycle time measurement.

### The problem: project board timestamps are mutable

When you move an issue to "In Progress" on a Projects v2 board, GitHub records a `ProjectV2ItemFieldSingleSelectValue` with an `updatedAt` timestamp. This seems like a useful cycle time signal — but it has a critical flaw: **`updatedAt` reflects the last time the field was modified, not when the status was originally set**.

Common scenario that produces wrong data:

1. Monday: You move issue #42 to "In Progress" → `updatedAt` = Monday
2. Wednesday: You close issue #42
3. Thursday: You move the card to "Done" on the board → `updatedAt` = **Thursday**

Now `updatedAt` (Thursday) is **after** the issue close date (Wednesday). The tool computes `start - end = Thursday - Wednesday = -1 day`. This produces **negative cycle times**, which are nonsensical.

This is not a bug in `gh-velocity`. It is a fundamental limitation of the GitHub Projects v2 API:

- **There is no field change history API.** You cannot query "when did this issue first move to In Progress?"
- **The REST timeline API does not include project field changes.** Even per-issue timeline queries cannot retrieve project board transitions.
- **The `updatedAt` on field values is the only timestamp available**, and it is overwritten on every field change.

### The solution: use labels

Label events have **immutable timestamps**. When you apply a label to an issue, GitHub creates a `LABELED_EVENT` with a `createdAt` timestamp that **never changes** — not when you remove the label, not when you re-add it, not when you modify anything else. The first application of that label is permanently recorded.

This makes labels the only reliable source of "when did work start?" from the GitHub API.

### What project board status is still good for

Project board status remains valuable for **current-state queries** — things that ask "what is the status right now?" rather than "when did the status change?":

| Use case | Signal | Works well? |
|----------|--------|-------------|
| **Cycle time start** (when did work begin?) | Label `createdAt` | Yes — immutable |
| **Cycle time start** (when did work begin?) | Board `updatedAt` | **No** — mutable, can be wrong |
| **WIP count** (how many items are in progress now?) | Board current status | Yes — current state |
| **Backlog detection** (is this issue still in backlog?) | Board current status | Yes — current state |

Configure `project_status` for WIP and backlog. Configure `match` for cycle time.

### Syncing project board status to labels

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

> **Important**: The `projects_v2_item` webhook event requires a **GitHub App** or a **classic PAT** with `project` scope. The default `GITHUB_TOKEN` in GitHub Actions **cannot** receive project board events. This is another GitHub platform limitation.

If setting up a GitHub App or PAT is not feasible, the simplest alternative is to manually apply the `in-progress` label when you start work. Many teams find this natural — applying a label when picking up an issue is a single click in the GitHub UI.

### Configuration examples

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

**Project board only (not recommended — you will see a deprecation warning):**
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

## Troubleshooting

### "not a git repository. Use --repo owner/name"

You are not inside a git checkout. Either `cd` into one or use `-R owner/name`.

### "no GitHub release for v1.0.0, using current time"

The tag exists but has no corresponding GitHub Release. The tool falls back to the current time for the release date, which makes release lag inaccurate. Create GitHub Releases for your tags, or the tool will resolve the tag's commit date from the API.

### "strategy pr-link: pr-link strategy requires tag dates"

Both the current and previous tags need dates for pr-link to search for merged PRs. This usually means the previous tag has no GitHub Release and the tag date could not be resolved. The other strategies (commit-ref, changelog) still run.

### "Low label coverage: N/M issues have no bug/feature labels"

More than half the issues lack the labels configured for classification. Either label your issues or customize `quality.categories` in your config. Run `gh velocity config preflight` to discover available labels and generate matching categories.

### "shallow clone detected; commit history is incomplete"

You are running in a git checkout that was cloned with limited history (common in CI). Fix this in GitHub Actions:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0    # fetch full history
```

Without full history, the tool cannot find commits between tags or search commit messages for issue references. Lead time (which only uses issue dates) is unaffected.

### Cycle time shows N/A for all issues

This is the most common first-run issue. Causes by strategy:

**Issue strategy** (`cycle_time.strategy: issue`):
- Missing `lifecycle.in-progress.match` in config — the tool has no label-based signal to detect when work started. Fix: add labels like `in-progress` to your repo and configure `lifecycle.in-progress.match: ["label:in-progress"]`.
- Falling back to project board but missing `project.url` or `project.status_field` — run `gh velocity config discover -R owner/repo` to find the right values.
- Token missing `project` scope — run `gh auth status` to check (only needed if using project board features).
- Negative cycle times in output — your project board timestamps are unreliable. Add `lifecycle.in-progress.match` with label matchers. See [Why labels over project board](#why-labels-over-project-board-for-cycle-time).

**PR strategy** (`cycle_time.strategy: pr`):
- No closing PRs found — ensure PRs reference issues with "Closes #N" or "Fixes #N" in the PR description.
- Issues were closed without PRs — the PR strategy requires merged PRs linked to issues.

**Quick fix**: Switch to `strategy: pr` if you don't use a project board. It works immediately when PRs reference issues.

### Cycle time shows N/A for a single issue

Cycle time is N/A when the configured strategy has no signal for that issue:

- **Issue strategy**: The issue has no matching in-progress label, and either it was never tracked on the configured project board or it never moved into an in-progress status.
- **PR strategy**: No merged PR references this issue with a closing keyword.

### No results / empty output

1. **Check your date range**: `--since 30d` looks at the last 30 days. Try a wider range.
2. **Check your scope**: Run with `--debug` to see the GitHub search query and verify URL.
3. **Check the verify URL**: Bulk commands show a "Verify:" link — open it in GitHub to see what the search returns.
4. **Check for activity**: A repo with no closed issues or merged PRs in the window will show empty results. That's correct.

### Bug ratio shows 0%

The report's bug ratio counts issues classified as "bug". If you name your bug category differently (e.g., "defect", "incident"), rename it to "bug" in your config:

```yaml
quality:
  categories:
    - name: bug        # must be "bug" for bug ratio
      match:
        - "label:defect"
        - "label:incident"
```
