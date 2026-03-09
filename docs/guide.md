# gh-velocity guide

## Why this exists

Engineering teams need to know how fast they ship and where the bottlenecks are. Most tools that answer these questions require a separate data warehouse, a tracking integration, or a per-seat subscription. `gh-velocity` takes a different approach: it computes metrics directly from the data GitHub already has.

Your issues have creation and closure dates. Your pull requests have merge timestamps and closing references. Your releases have tags and bodies. That is enough to measure lead time, cycle time, release lag, and composition — per issue, per release, and across releases.

The tradeoff is clear: you get zero-setup velocity metrics in exchange for being constrained to what the GitHub API can tell you. This guide explains exactly what that means and where the edges are.

## What the metrics mean

**Lead time** is the duration from when an issue is created to when it is closed. It measures the total elapsed time a piece of work existed, including time spent in backlog, waiting for review, blocked by dependencies, or simply forgotten. A long lead time does not necessarily mean slow development — it often means slow prioritization.

**Cycle time** is the duration from the first commit that references an issue to when that issue is closed. It approximates active development time. Cycle time requires a local git checkout because it searches commit history for issue references. When running against a remote repo (`-R`), cycle time is unavailable for single-issue queries.

**Release lag** is the duration from when an issue is closed to when the release containing it is published. It measures how long finished work waits before reaching users. High release lag often points to batch-and-release workflows where completed work sits in a staging branch.

**Cadence** is the time between consecutive releases. It is not a per-issue metric but a release-level observation. Cadence combined with composition (bug ratio, feature ratio) tells you whether you are shipping improvements or fighting fires.

**Hotfix** is a boolean flag. A release is marked as a hotfix when its cadence is shorter than the configured `hotfix_window_hours` (default: 72 hours). This lets you separate planned releases from emergency patches in your analysis.

## What GitHub can and cannot tell you

`gh-velocity` is constrained to the GitHub API. Here is what that means in practice.

### What works well

- **Issue lifecycle**: Creation and closure dates are precise. Lead time is reliable.
- **PR merge timestamps**: The search API returns exact merge dates. The `pr-link` strategy uses these to find PRs in a release window.
- **Closing references**: GitHub tracks which PRs close which issues. The GraphQL `closingIssuesReferences` field is the most reliable way to connect PRs to issues.
- **Release metadata**: Tags, release dates, and release bodies are all available via the REST API.
- **Labels**: Issue labels are the basis for bug/feature classification. If your team labels issues consistently, composition metrics are accurate.

### What has limits

- **Cycle time depends on local git with full history**. When you run `gh velocity cycle-time 42`, the tool searches your local git log for commits referencing issue #42. Against a remote repo (`-R owner/name`), this data is not available. In release reports, cycle time uses the commits discovered by the linking strategies — which may not include all relevant commits. **In GitHub Actions**, the default `actions/checkout` does a shallow clone (only the latest commit). You must set `fetch-depth: 0` for accurate commit-based metrics. The tool detects shallow clones and warns you.
- **The PR search API caps at 1000 results**. If a release window contains more than 1000 merged PRs, the `pr-link` strategy warns you and returns partial results. This is rare outside the largest monorepos.
- **Tag ordering is by API default, not semver**. Tags are returned in the order GitHub's API provides, which is usually creation date. The tool picks the tag immediately before your target tag in this list. If your tag history is non-linear, use `--since` to specify the previous tag explicitly.
- **"Closed" is not "merged"**. GitHub issues can be closed without a PR being merged — by a maintainer, a bot, or the author. `gh-velocity` treats closure as the end event regardless of cause. For most teams this is fine; for teams that close stale issues aggressively, it may inflate lead time counts.
- **Label-based classification is only as good as your labels**. If more than half the issues in a release lack bug/feature labels, the tool warns you. You can customize which labels map to which categories in the config file.

### What is not possible

- **Work-in-progress duration**. GitHub does not track when an issue moves between project board columns. Without a project management integration, there is no way to measure time-in-review or time-in-backlog as separate phases.
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
gh extension install dvhthomas/gh-velocity
```

Verify:

```bash
gh velocity version
```

### First queries

Start with a public repo to see what the output looks like:

```bash
# Release report for the GitHub CLI itself
gh velocity release v2.67.0 -R cli/cli
```

This takes 10-30 seconds depending on the number of issues. You will see:

- Release metadata (previous tag, cadence, hotfix status)
- Composition breakdown (bugs, features, other)
- Per-issue table with lead time, cycle time, release lag, and outlier flags
- Aggregate statistics with P90, P95, and outlier counts

Try the same report in JSON to see the full data:

```bash
gh velocity release v2.67.0 -R cli/cli -f json | jq '.aggregates.lead_time'
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
gh velocity scope v2.67.0 -R cli/cli
```

This shows what `pr-link`, `commit-ref`, and `changelog` each discovered, and the merged result. Use this to understand how well the strategies cover your workflow.

### Your own repo

Navigate to a local checkout and run:

```bash
cd your-repo
gh velocity release v1.0.0
```

When run from inside a repo, the tool uses local git for tag listing and commit history. This is faster and enables cycle-time computation. When you use `-R` to point at a remote repo, it falls back to the GitHub API with reduced functionality.

## Configuration reference

Create `.gh-velocity.yml` in your repository root. Every field is optional. The tool uses sensible defaults when no config file exists (which is the case when using `-R` against a remote repo).

### Minimal config

```yaml
quality:
  bug_labels: ["bug"]
  feature_labels: ["enhancement"]
```

This is equivalent to the defaults. You only need a config file if you want to change something.

### Full config

```yaml
# How your team works. "pr" means PRs close issues (most teams).
# "local" means direct commits to main (solo projects, scripts).
workflow: pr

# Issue classification labels
quality:
  bug_labels: ["bug", "defect", "regression"]
  feature_labels: ["enhancement", "feature", "new"]
  hotfix_window_hours: 48        # releases within 48h of previous = hotfix

# Commit message scanning
commit_ref:
  patterns: ["closes"]           # default: only closing keywords
  # patterns: ["closes", "refs"]   # also match bare #N references

# GitHub Projects v2 (for future project-level metrics)
project:
  id: "PVT_kwDOAbc123"
  status_field_id: "PVTSSF_kwDOAbc123"

# Status field value names (match your project board)
statuses:
  backlog: "Backlog"
  ready: "Ready"
  in_progress: "In progress"
  in_review: "In review"
  done: "Done"

# GitHub Discussions integration (for future posting)
discussions:
  category_id: "DIC_kwDOAbc123"
```

### Configuration details

**`quality.bug_labels`** and **`quality.feature_labels`**: Arrays of label names. Matching is exact and case-sensitive. An issue is classified as "bug" if any of its labels appear in `bug_labels`. If no labels match either list, the issue is classified as "other." When more than 50% of issues are "other," the tool warns about low label coverage.

**`quality.hotfix_window_hours`**: A release is flagged as a hotfix if it was published within this many hours of the previous release. Default is 72 (3 days). Maximum is 8760 (1 year). Set this lower if your team has a fast release cadence.

**`commit_ref.patterns`**: Controls how the `commit-ref` strategy scans commit messages.

- `["closes"]` (default): Only matches closing keywords — `fixes #N`, `closes #N`, `resolves #N` and their variations. This is conservative and avoids false positives from comments like "see #42" or "step #1".
- `["closes", "refs"]`: Also matches bare `#N` references. Use this if your team writes commits like "implement #42" without closing keywords. Be aware that this can match false positives like "update step #1."

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

The `release` and `scope` commands need to determine which issues belong to a release. This is harder than it sounds — different teams use different workflows, and no single method works everywhere. `gh-velocity` uses three strategies and merges the results.

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

The `scope` command shows this merge in action:

```bash
gh velocity scope v1.2.0
```

The output lists what each strategy found and marks items that appear in multiple strategies with "(also: commit-ref)" annotations.

## Use with an agent

Every command supports `-f json` for structured output. This makes `gh-velocity` composable with LLM agents, CI scripts, and data pipelines.

### Agent integration patterns

An agent that reviews releases:

```bash
# Get the full release report as JSON
REPORT=$(gh velocity release v1.2.0 -R owner/repo -f json)

# Feed to an agent for analysis
echo "$REPORT" | your-agent analyze-release
```

Extracting specific data with jq:

```bash
# Which issues are outliers?
gh velocity release v1.2.0 -f json | \
  jq '[.issues[] | select(.lead_time_outlier) | {number, title, lead_time_seconds}]'

# What percentage are bugs?
gh velocity release v1.2.0 -f json | \
  jq '.composition | "\(.bug_count)/\(.total_issues) bugs (\(.bug_ratio * 100 | round)%)"'

# P95 lead time in days
gh velocity release v1.2.0 -f json | \
  jq '.aggregates.lead_time.p95_seconds / 86400 | round | "\(.) days"'
```

### Posting to GitHub

The markdown format is designed for pasting into issues, PRs, or discussions:

```bash
# Generate a release summary and post it as an issue comment
gh velocity release v1.2.0 -f markdown | \
  gh issue comment 100 --body-file -

# Or create a new issue with the report
gh velocity release v1.2.0 -f markdown | \
  gh issue create --title "Release v1.2.0 metrics" --body-file -
```

### Claude Code / Copilot agent example

If you use an agent that can run shell commands, point it at your repo:

```
You have access to `gh velocity`. Use it to analyze our last 3 releases
and identify trends in lead time and bug ratio.

Commands available:
  gh velocity release <tag> -f json
  gh velocity scope <tag> -f json
  gh velocity lead-time <issue> -f json

Our recent tags: v2.5.0, v2.4.0, v2.3.0
```

The JSON output includes every field an agent needs: seconds-based durations, ratios as floats, boolean flags, and descriptive warnings.

## CI integration

### GitHub Actions: release metrics comment

Post a metrics report on every release:

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

      - name: Install gh-velocity
        run: gh extension install dvhthomas/gh-velocity
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Generate report
        run: |
          gh velocity release ${{ github.event.release.tag_name }} -f markdown > report.md
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Post to release discussion
        run: |
          gh issue create \
            --title "Metrics: ${{ github.event.release.tag_name }}" \
            --body-file report.md \
            --label "metrics"
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
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

      - name: Install gh-velocity
        run: gh extension install dvhthomas/gh-velocity
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract linked issue
        id: issue
        run: |
          # Parse PR body for "Fixes #N" or "Closes #N"
          ISSUE=$(echo "${{ github.event.pull_request.body }}" | \
            grep -oP '(?:fixes|closes|resolves)\s+#\K\d+' -i | head -1)
          echo "number=$ISSUE" >> "$GITHUB_OUTPUT"

      - name: Report lead time
        if: steps.issue.outputs.number != ''
        run: |
          gh velocity lead-time ${{ steps.issue.outputs.number }} -f markdown | \
            gh pr comment ${{ github.event.pull_request.number }} --body-file -
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Scheduled trend reports

Run weekly to track velocity over time:

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

      - name: Install gh-velocity
        run: gh extension install dvhthomas/gh-velocity
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Latest release metrics
        run: |
          TAG=$(git describe --tags --abbrev=0)
          gh velocity release "$TAG" -f json > metrics.json
          gh velocity release "$TAG" -f markdown > metrics.md
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: velocity-metrics
          path: metrics.json
```

## How-to recipes

### Compare two releases

```bash
gh velocity release v2.0.0 -f json > v2.json
gh velocity release v1.9.0 -f json > v1.json

# Compare lead times
echo "v1.9.0 median lead time: $(jq -r '.aggregates.lead_time.median_seconds / 86400 | round | "\(.)d"' v1.json)"
echo "v2.0.0 median lead time: $(jq -r '.aggregates.lead_time.median_seconds / 86400 | round | "\(.)d"' v2.json)"
```

### Find your slowest issues

```bash
gh velocity release v1.2.0 -f json | \
  jq -r '.issues | sort_by(-.lead_time_seconds) | .[0:5] | .[] |
    "#\(.number) \(.title[0:40]) — \(.lead_time_seconds / 86400 | round)d"'
```

### Check label coverage before a release

```bash
gh velocity release v1.2.0 -f json | \
  jq '"Bug: \(.composition.bug_count), Feature: \(.composition.feature_count), Unlabeled: \(.composition.other_count)"'
```

If `other_count` is high, label your issues before publishing the release for more useful metrics.

### Use --since to narrow scope

When the auto-detected previous tag is wrong (non-linear tag history, pre-releases), override it:

```bash
gh velocity release v2.0.0 --since v1.9.0
gh velocity scope v2.0.0 --since v1.9.0
```

### Analyze a repo you don't have locally

Every command works with `-R`:

```bash
gh velocity release v0.28.0 -R charmbracelet/bubbletea
gh velocity lead-time 500 -R charmbracelet/bubbletea
gh velocity scope v5.2.1 -R go-chi/chi
```

Note: cycle time for single issues requires a local checkout. In release reports, cycle time is computed from commits discovered by the linking strategies (which works via the API).

### Generate a report for every release

```bash
for tag in $(gh api repos/owner/repo/tags --jq '.[].name' | head -5); do
  echo "=== $tag ==="
  gh velocity release "$tag" -R owner/repo 2>/dev/null
  echo
done
```

### Export to CSV for spreadsheet analysis

```bash
gh velocity release v1.2.0 -f json | \
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

## Troubleshooting

### "not a git repository. Use --repo owner/name"

You are not inside a git checkout. Either `cd` into one or use `-R owner/name`.

### "no GitHub release for v1.0.0, using current time"

The tag exists but has no corresponding GitHub Release. The tool falls back to the current time for the release date, which makes release lag inaccurate. Create GitHub Releases for your tags, or the tool will resolve the tag's commit date from the API.

### "strategy pr-link: pr-link strategy requires tag dates"

Both the current and previous tags need dates for pr-link to search for merged PRs. This usually means the previous tag has no GitHub Release and the tag date could not be resolved. The other strategies (commit-ref, changelog) still run.

### "Low label coverage: N/M issues have no bug/feature labels"

More than half the issues lack the labels configured for bug/feature classification. Either label your issues or customize `quality.bug_labels` and `quality.feature_labels` in your config.

### "shallow clone detected; commit history is incomplete"

You are running in a git checkout that was cloned with limited history (common in CI). Fix this in GitHub Actions:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0    # fetch full history
```

Without full history, the tool cannot find commits between tags or search commit messages for issue references. Lead time (which only uses issue dates) is unaffected.

### Cycle time shows N/A for all issues

Cycle time requires commit-to-issue linking. When running against a remote repo (`-R`), single-issue `cycle-time` queries cannot search git history. In release reports, cycle time depends on the linking strategies finding commits — if `pr-link` finds issues through PRs (not commits), cycle time will be N/A.
