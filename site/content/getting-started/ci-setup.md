---
title: "CI Setup"
weight: 4
---

# CI / GitHub Actions Setup

Run gh-velocity in CI to post automated reports on releases, PRs, and schedules.

## How authentication works

gh-velocity uses the `GH_TOKEN` environment variable for all GitHub API calls -- the same variable that powers the `gh` CLI. Locally, `gh auth login` handles this. In CI, set `GH_TOKEN` in your workflow. See [Token permissions]({{< relref "/getting-started/configuration" >}}#token-permissions) for what each token can access.

## Token setup

The default `GITHUB_TOKEN` works for most commands. However, **`GITHUB_TOKEN` cannot access Projects v2 boards** -- a GitHub platform limitation.

gh-velocity handles this with `GH_VELOCITY_TOKEN`. When set, the binary uses it instead of `GH_TOKEN` for all API calls. Pass both:

```yaml
env:
  GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
```

### What each token can do

`GITHUB_TOKEN` handles everything except Projects v2 board access. Add `GH_VELOCITY_TOKEN` only if your config has a `project:` section.

### Setting up GH_VELOCITY_TOKEN

1. **Create a classic PAT** with the following scopes:

   - `project` (read-only) -- required for Projects v2 board access
   - `public_repo` -- required if your workflow creates Discussions via GraphQL (e.g., the showcase workflow or `--post` to Discussions)
   - `write:discussion` -- required for posting comments to Discussions

   [Create token](https://github.com/settings/tokens/new?scopes=project,public_repo,write:discussion&description=gh-velocity) -- this link pre-fills the scopes and description.

   > [!NOTE]
   > Fine-grained PATs do not support user-owned projects or the `createDiscussion` GraphQL mutation. Use a classic PAT.

2. **Add it as a repository secret** named `GH_VELOCITY_TOKEN`:

   Go to your repo Settings, then Secrets and variables, then Actions, then New repository secret.

3. **Pass it in your workflow** alongside `GH_TOKEN`:

   ```yaml
   env:
     GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
     GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
   ```

   `GH_VELOCITY_TOKEN` takes precedence when set. If empty or missing, the binary falls back to `GH_TOKEN`.

## Workflow permissions

Your workflow needs explicit `GITHUB_TOKEN` permissions for `--post` to write back to GitHub:

```yaml
permissions:
  contents: read          # read repo and config
  issues: write           # --post comments on issues/PRs
  discussions: write      # --post bulk reports as Discussions
```

`GH_VELOCITY_TOKEN` (a classic PAT) needs `project`, `public_repo`, and `write:discussion` scopes -- these are set when creating the token, not in the workflow file.

## Which workflow should you use?

| Your goal | Workflow | Trigger |
|---|---|---|
| Regular team dashboard | [Weekly velocity report](#weekly-velocity-report) | `schedule` (cron) |
| Track release quality over time | [Release metrics comment](#release-metrics-comment) | `release: [published]` |
| PR-level feedback | [PR lead-time check](#pr-lead-time-check) | `pull_request` |
| Long-term trend data | [Scheduled trend reports](#scheduled-trend-reports) | `schedule` (cron) |

Start with the **weekly velocity report** -- it covers the most ground with the least setup.

## Example workflows

### Weekly velocity report

Post a velocity report to GitHub Discussions every week:

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
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Post velocity report
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
          GH_VELOCITY_POST_LIVE: 'true'
        run: gh velocity report --since 30d --post -r markdown
```

> [!NOTE]
> `GH_VELOCITY_POST_LIVE` is required for `--post` to write to GitHub. Without it, `--post` runs in dry-run mode -- a safety net against accidental posts during local testing. See [Posting Reports]({{< relref "/guides/posting-reports" >}}) for all `--post` options.

### Release metrics comment

Post a quality report when a release is published:

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
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Post release metrics
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
        run: |
          gh velocity quality release ${{ github.event.release.tag_name }} -r markdown > report.md
          gh issue create \
            --title "Metrics: ${{ github.event.release.tag_name }}" \
            --body-file report.md \
            --label "metrics"
```

### PR lead-time check

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
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

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
          gh velocity flow lead-time ${{ steps.issue.outputs.number }} -r markdown | \
            gh pr comment ${{ github.event.pull_request.number }} --body-file -
```

### Scheduled trend reports

Capture metrics as build artifacts for long-term trend analysis:

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
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Latest release metrics
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
        run: |
          TAG=$(git describe --tags --abbrev=0)
          gh velocity quality release "$TAG" -r json > metrics.json
          gh velocity quality release "$TAG" -r markdown > metrics.md

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: velocity-metrics
          path: metrics.json
```

## Real-world example

The gh-velocity repo uses this pattern itself. See [`docs/velocity-report-workflow.yml`](https://github.com/dvhthomas/gh-velocity/blob/main/docs/velocity-report-workflow.yml) for a production-ready workflow to copy into `.github/workflows/`.

## Tips

**Use `fetch-depth: 0` for commit analysis.** The `commit-ref` linking strategy and commit-enriched cycle time require full git history. Without it, the tool warns about a shallow clone and skips commit-based analysis. Lead time is unaffected.

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
```

**Start with `GITHUB_TOKEN` only.** No `project:` section means no `GH_VELOCITY_TOKEN` needed. Add it later if you enable project board features.

**Use `workflow_dispatch` for testing.** Adding `workflow_dispatch` lets you run the workflow manually from the GitHub UI while debugging.

## Next steps

- [Posting Reports]({{< relref "/guides/posting-reports" >}}) -- `--post` options, idempotent posting, and manual patterns
- [Troubleshooting]({{< relref "/guides/troubleshooting" >}}) -- fix common CI errors like "Resource not accessible" and dry-run `--post`
