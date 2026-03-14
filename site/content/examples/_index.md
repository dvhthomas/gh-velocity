---
title: "Examples"
weight: 5
bookCollapseSection: true
---

# Examples

Real-world configuration examples for popular repositories, with annotations explaining what each section does and why.

All example configs are in [`docs/examples/`](https://github.com/dvhthomas/gh-velocity/tree/main/docs/examples) and are validated by the E2E test suite (`task e2e:configs`).

## Generating your own config

Use `preflight` to auto-detect a good starting config for any repo:

```bash
gh velocity config preflight -R owner/repo              # preview to stdout
gh velocity config preflight -R owner/repo --write       # save to .gh-velocity.yml

# With a project board
gh velocity config preflight -R owner/repo \
  --project-url https://github.com/users/me/projects/1
```

## Running examples

```bash
# Release quality report
gh velocity quality release v2.67.0 -R cli/cli \
  --config docs/examples/cli-cli.yml

# Lead time for recent work
gh velocity flow lead-time 1234 -R cli/cli \
  --config docs/examples/cli-cli.yml

# JSON output for scripting
gh velocity quality release v2.67.0 -R cli/cli \
  --config docs/examples/cli-cli.yml -f json
```

---

## Example 1: cli/cli -- Label-based issue workflow

**Repo**: [cli/cli](https://github.com/cli/cli) (GitHub CLI)
**Strategy**: Issue-based cycle time, label classification, no project board

```yaml
# Scope tells gh-velocity which repo to query.
# When running with -R cli/cli, this is redundant but makes
# the config self-contained for CI use.
scope:
  query: "repo:cli/cli"

# Four classification categories using labels.
# cli/cli uses standard GitHub labels: "bug", "enhancement", "tech-debt", "docs".
# First match wins -- an issue labeled both "bug" and "enhancement" is classified as "bug".
# Anything without a matching label becomes "other".
quality:
  categories:
    - name: bug
      match:
        - "label:bug"
    - name: feature
      match:
        - "label:enhancement"
    - name: chore
      match:
        - "label:tech-debt"
    - name: docs
      match:
        - "label:docs"
  # A release within 72 hours of the previous one is flagged as a hotfix.
  hotfix_window_hours: 72

# Scan commit messages for closing keywords (fixes #N, closes #N).
# The "closes" pattern is conservative -- it won't match "see #42" or "step #1".
commit_ref:
  patterns: ["closes"]

# Issue strategy: cycle time starts when an in-progress label is applied.
# cli/cli doesn't have a public project board, so labels are the only signal.
# If no in-progress label exists on an issue, cycle time is N/A.
cycle_time:
  strategy: issue

# Only the "done" stage is configured -- closed issues.
# No backlog or in-progress query because there's no project board.
lifecycle:
  done:
    query: "is:closed"
```

**Key takeaways**:
- This is a minimal, zero-board config. Works for any repo with consistent labeling.
- The `issue` cycle time strategy requires `lifecycle.in-progress.match` to be set for cycle time to work. Without it, cycle time is N/A. To add it: `lifecycle.in-progress.match: ["label:in-progress"]`.
- Classification relies entirely on labels. If cli/cli has low labeling coverage, many issues will be "other."

---

## Example 2: dvhthomas/gh-velocity -- Project board with lifecycle mapping

**Repo**: [dvhthomas/gh-velocity](https://github.com/dvhthomas/gh-velocity) (this project)
**Strategy**: Issue-based cycle time, project board columns, title-based classification

```yaml
scope:
  query: "repo:dvhthomas/gh-velocity"

# Title-based classification using regex matchers.
# This repo uses conventional commit prefixes in issue titles:
# "feat: ...", "refactor: ...", "docs: ...".
# No "bug" category because this repo doesn't use a "bug" label consistently.
quality:
  categories:
    - name: feature
      match:
        - "title:/^feat[\\(: ]/i"    # matches "feat: add X" or "feat(scope): ..."
    - name: chore
      match:
        - "title:/^refactor[\\(: ]/i"
    - name: docs
      match:
        - "title:/^docs?[\\(: ]/i"   # matches "doc: ..." or "docs: ..."
  hotfix_window_hours: 72

commit_ref:
  patterns: ["closes"]

cycle_time:
  strategy: issue

# Project board configuration.
# The URL points to a GitHub Projects v2 board.
# status_field is the name of the single-select field that tracks status.
project:
  url: "https://github.com/users/dvhthomas/projects/1"
  status_field: "Status"

# Full lifecycle mapping from board columns.
# Each stage maps to one or more project board status values.
# This enables:
#   - WIP command: shows items in "In progress" + "In review"
#   - Backlog detection: items in "Backlog" are excluded from cycle time
#   - Cycle time fallback: if no in-progress label, board status is used
lifecycle:
  backlog:
    project_status: ["Backlog"]
  in-progress:
    project_status: ["In progress"]
    # To add label-based cycle time (recommended):
    # match: ["label:in-progress"]
  in-review:
    project_status: ["In review"]
  done:
    project_status: ["Done"]
```

**Key takeaways**:
- Title regex matchers work well for repos that follow conventional commit naming.
- The project board enables the `wip` command and gives backlog visibility.
- For best cycle time accuracy, add `match: ["label:in-progress"]` to `lifecycle.in-progress` so label timestamps are used instead of board timestamps.
- The commented-out velocity section shows what sprint velocity config would look like with a `project-field` iteration strategy.

---

## Example 3: cli/cli with velocity -- Fixed iterations for sprint tracking

**Repo**: cli/cli with velocity tracking added
**Strategy**: PR-based cycle time, fixed 14-day iterations, count-based effort

```yaml
scope:
  query: "repo:cli/cli"

quality:
  categories:
    - name: bug
      match:
        - "label:bug"
    - name: feature
      match:
        - "label:enhancement"
  hotfix_window_hours: 72

commit_ref:
  patterns: ["closes"]

# PR strategy: no labels or board needed.
# Cycle time = PR created -> PR merged.
cycle_time:
  strategy: pr

# Velocity configuration for sprint tracking without a project board.
velocity:
  # Count closed issues as the work unit.
  unit: issues

  # Every closed issue = 1 point of effort.
  # Simple and requires no sizing labels.
  effort:
    strategy: count

  # Fixed 14-day iterations (2-week sprints).
  # Anchor is a known sprint start date -- the tool computes
  # all other iteration boundaries by stepping forward/backward.
  iteration:
    strategy: fixed
    fixed:
      length: "14d"
      anchor: "2026-01-06"

    # Show last 6 iterations in history.
    count: 6
```

**Key takeaways**:
- Fixed iterations work without a project board. The tool uses Search API to find items closed in each 14-day window.
- `effort.strategy: count` is the simplest option -- every closed issue counts equally.
- The anchor date should be a real sprint start date in your team's calendar. The tool extrapolates all other boundaries from it.
- To add effort weighting, switch to `effort.strategy: attribute` with label-based matchers.

---

## Feature matrix

| Config | Repo | Cycle time | Categories | Backlog | Project board | Velocity |
|--------|------|-----------|:---:|:---:|:---:|:---:|
| cli-cli.yml | cli/cli | issue | bug, feature, chore, docs | | | |
| cli-cli-velocity.yml | cli/cli | pr | bug, feature | | | fixed (14d) |
| dvhthomas-gh-velocity.yml | dvhthomas/gh-velocity | issue | feature, chore, docs | x | x | |
| facebook-react.yml | facebook/react | pr | bug, feature | x | | |
| kubernetes-kubernetes.yml | kubernetes/kubernetes | pr | bug, feature, chore, docs | x | | |
| hashicorp-terraform.yml | hashicorp/terraform | pr | bug, feature, chore, docs | | | |
| astral-sh-uv.yml | astral-sh/uv | pr | bug, feature, chore, docs | | | |

---

## CI workflow example

Post a weekly velocity report to GitHub Discussions using a GitHub Actions workflow:

```yaml
name: Velocity Report
on:
  schedule:
    - cron: '0 9 * * 1'  # Every Monday at 9am UTC
  workflow_dispatch:       # Allow manual trigger

permissions:
  contents: read
  pull-requests: read
  issues: write
  discussions: write

jobs:
  report:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install gh-velocity
        run: gh extension install dvhthomas/gh-velocity
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Post velocity report
        run: gh velocity report --since 30d --post -f markdown
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_POST_LIVE: 'true'
```

See [`docs/examples/velocity-report.yml`](https://github.com/dvhthomas/gh-velocity/blob/main/docs/examples/velocity-report.yml) for the full workflow file.
