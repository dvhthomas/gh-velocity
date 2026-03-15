---
title: "CI Setup"
weight: 4
---

# CI / GitHub Actions Setup

Run gh-velocity in CI to post automated reports on releases, PRs, and schedules.

## How authentication works

gh-velocity uses the `GH_TOKEN` environment variable for all GitHub API calls -- the same variable that powers the `gh` CLI. Locally, `gh auth login` handles this automatically. In CI, you set `GH_TOKEN` in your workflow. For details on what each token can access, see the [Configuration: Token permissions]({{< relref "/getting-started/configuration" >}}#token-permissions) section.

## Token setup

The default `GITHUB_TOKEN` provided by GitHub Actions works for most commands. However, **`GITHUB_TOKEN` cannot access Projects v2 boards** -- this is a GitHub platform limitation.

gh-velocity handles this with the `GH_VELOCITY_TOKEN` environment variable. When set, the binary automatically uses it instead of `GH_TOKEN` for all API calls. No workflow fallback logic needed -- just pass both:

```yaml
env:
  GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
```

### What each token can do

See the [token permissions table]({{< relref "/getting-started/configuration" >}}#token-permissions) in the Configuration page for the full breakdown. The key distinction: `GITHUB_TOKEN` handles everything except Projects v2 board access. Add `GH_VELOCITY_TOKEN` only if your config has a `project:` section.

### Setting up GH_VELOCITY_TOKEN

1. **Create a classic PAT** with `project` (read-only) scope:

   [Create token](https://github.com/settings/tokens/new?scopes=project&description=gh-velocity) -- this link pre-fills the scope and description.

   > [!NOTE]
   > Fine-grained PATs do not currently support user-owned projects. Use a classic PAT for user projects, or a GitHub App for organization projects.

2. **Add it as a repository secret** named `GH_VELOCITY_TOKEN`:

   Go to your repo Settings, then Secrets and variables, then Actions, then New repository secret.

3. **Pass it in your workflow** alongside `GH_TOKEN`:

   ```yaml
   env:
     GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
     GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
   ```

   The binary prefers `GH_VELOCITY_TOKEN` when set. If it is empty or missing, it falls back to `GH_TOKEN` transparently.

## Workflow permissions

Your workflow needs explicit `GITHUB_TOKEN` permissions for `--post` to write back to GitHub:

```yaml
permissions:
  contents: read          # read repo and config
  issues: write           # --post comments on issues/PRs
  discussions: write      # --post bulk reports as Discussions
```

These are `GITHUB_TOKEN` permissions set in the workflow file. `GH_VELOCITY_TOKEN` only needs the `project` scope -- it inherits read access to public repos automatically.

## Which workflow should you use?

| Your goal | Workflow | Trigger |
|---|---|---|
| Regular team dashboard | [Weekly velocity report](#weekly-velocity-report) | `schedule` (cron) |
| Track release quality over time | [Release metrics comment](#release-metrics-comment) | `release: [published]` |
| PR-level feedback | [PR lead-time check](#pr-lead-time-check) | `pull_request` |
| Long-term trend data | [Scheduled trend reports](#scheduled-trend-reports) | `schedule` (cron) |

Start with the **weekly velocity report** — it covers the most ground with the least setup.

## Example workflows

### Weekly velocity report

Post a velocity report to GitHub Discussions every week. This is the most common CI use case.

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
        run: gh velocity report --since 30d --post -f markdown
```

> [!NOTE]
> The `GH_VELOCITY_POST_LIVE` environment variable is required for `--post` to actually write to GitHub. Without it, `--post` runs in dry-run mode. This is a safety net to prevent accidental posts during local testing. See [Posting Reports]({{< relref "/guides/posting-reports" >}}) for all `--post` options and patterns.

### Release metrics comment

Post a quality report automatically when a release is published:

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
          gh velocity quality release ${{ github.event.release.tag_name }} -f markdown > report.md
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
          gh velocity flow lead-time ${{ steps.issue.outputs.number }} -f markdown | \
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
          gh velocity quality release "$TAG" -f json > metrics.json
          gh velocity quality release "$TAG" -f markdown > metrics.md

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: velocity-metrics
          path: metrics.json
```

## Real-world example

The gh-velocity repo itself uses a CI workflow for velocity reports. See [`docs/examples/velocity-report.yml`](https://github.com/dvhthomas/gh-velocity/blob/main/docs/examples/velocity-report.yml) for a production-ready workflow you can copy into your `.github/workflows/` directory.

## Tips

**Use `fetch-depth: 0` for commit analysis.** If you want the `commit-ref` linking strategy or commit-enriched cycle time, check out the full git history. Without it, the tool warns about a shallow clone and skips commit-based analysis. Lead time (which only uses issue dates) is unaffected.

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
```

**Start with `GITHUB_TOKEN` only.** If your config has no `project:` section, you do not need `GH_VELOCITY_TOKEN` at all. Add it later if you enable project board features.

**Use `workflow_dispatch` for testing.** Adding `workflow_dispatch` to your workflow triggers lets you run the workflow manually from the GitHub UI while debugging.

## Next steps

- [Configuration]({{< relref "/getting-started/configuration" >}}) -- set up your `.gh-velocity.yml`
- [Posting Reports]({{< relref "/guides/posting-reports" >}}) -- `--post` options, idempotent posting, and manual patterns
- [Troubleshooting]({{< relref "/guides/troubleshooting" >}}) -- fix common CI errors like "Resource not accessible" and dry-run `--post`
- [How It Works]({{< relref "/getting-started/how-it-works" >}}) -- understand the metrics and strategies
