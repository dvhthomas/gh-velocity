---
title: "Posting Reports"
weight: 4
---

# Posting Reports

gh-velocity posts metrics directly to GitHub via the `--post` flag. Bulk commands (`report`, `quality release`) post to GitHub Discussions; single-item commands (`flow lead-time 42`) post as issue/PR comments.

## The --post flag

Every command supports `--post`:

```bash
gh velocity report --since 30d --post
gh velocity quality release v1.2.0 --post
gh velocity flow lead-time 42 --post
```

By default, `--post` runs in **dry-run mode** -- it shows what would be posted without writing to GitHub. To post for real, set `GH_VELOCITY_POST_LIVE`:

```bash
GH_VELOCITY_POST_LIVE=true gh velocity report --since 30d --post
```

This prevents accidental posts during local testing.

## Discussions config

Bulk commands post to GitHub Discussions. Configure posting in your [config file]({{< relref "/getting-started/configuration" >}}):

```yaml
discussions:
  category: General                                     # required
  title: "Weekly Velocity: {{.Repo}} ({{.Date}})"       # optional Go template
  repo: myorg/team-reports                               # optional: post to a different repo
```

GitHub Discussions must be enabled on the target repository (Settings > General > Features > Discussions). Run `gh velocity config preflight` to check this.

### `discussions.category` (required)

The category name in the target repo's Discussions settings (e.g., `"General"`, `"Reports"`). The match is case-insensitive. If not set, `--post` fails with an error on bulk commands.

### `discussions.title` (optional)

A [Go template](https://pkg.go.dev/text/template) for the Discussion title. Available fields:

| Field | Description | Example |
|---|---|---|
| `{{.Command}}` | Internal command name | `report`, `release`, `throughput` |
| `{{.Repo}}` | The analyzed repo (owner/repo) | `cli/cli` |
| `{{.Date}}` | UTC date when the command runs | `2026-03-21` |

**Default** (when omitted): `gh-velocity {{.Command}}: {{.Repo}} ({{.Date}})` — e.g., `gh-velocity report: cli/cli (2026-03-21)`.

Examples:

```yaml
# Simple weekly report title
title: "Weekly Velocity: {{.Repo}} ({{.Date}})"

# Command-specific
title: "{{.Command}} — {{.Repo}}"
```

### `discussions.repo` (optional)

Post to a different repo than the one being analyzed. Must be in `owner/repo` format. The category must exist in this target repo's Discussions settings.

```yaml
# Analyze cli/cli but post the report to myorg/team-reports
repo: myorg/team-reports
```

When omitted, discussions are created in the analyzed repo (the `--repo` / `-R` target or auto-detected from git remote).

## Idempotent posting

Running the same command with `--post` multiple times updates the existing Discussion or comment instead of creating a duplicate. The tool identifies matching posts using a combination of command name and parameters (e.g., `report` with `--since 30d`), so different parameter combinations create separate discussions.

To force a new post instead of updating, use `--new-post`:

```bash
gh velocity report --since 30d --new-post
```

`--new-post` implies `--post`, so you do not need both flags.

## Posting in CI

The most common CI pattern is a weekly report posted to Discussions on a cron schedule:

```yaml
name: Velocity Report

on:
  schedule:
    - cron: '0 9 * * 1'  # Monday 9am UTC
  workflow_dispatch:

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
        run: gh velocity report --since 30d --post --results markdown
```

Key points:

- **`GH_VELOCITY_POST_LIVE: 'true'`** -- required for `--post` to write. Without it, the run succeeds but nothing is posted.
- **`permissions: discussions: write`** -- required to create Discussions.
- **`permissions: issues: write`** -- required for issue/PR comments.
- **`workflow_dispatch`** -- lets you trigger the workflow manually for testing.

See [CI Setup]({{< relref "/getting-started/ci-setup" >}}) for complete workflow examples.

## Posting patterns

### Weekly team report to Discussions

Post a trailing-window report every Monday:

```bash
GH_VELOCITY_POST_LIVE=true gh velocity report --since 30d --post --results markdown
```

Re-running during the week updates the existing Discussion rather than creating a new one.

### Release metrics on every release

Triggered by a GitHub release event in CI:

```bash
GH_VELOCITY_POST_LIVE=true gh velocity quality release "$TAG" --post --results markdown
```

### Lead-time context on PRs

Post a lead-time summary as a PR comment when a PR references an issue:

```bash
gh velocity flow lead-time 42 --results markdown | \
  gh pr comment 99 --body-file -
```

This pattern uses piping to `gh pr comment` rather than `--post`, which gives you more control over the target.

### Manual posting with markdown

Generate a report and post it yourself:

```bash
# Post as an issue comment
gh velocity quality release v1.2.0 --results markdown | \
  gh issue comment 100 --body-file -

# Create a new issue with the report
gh velocity quality release v1.2.0 --results markdown | \
  gh issue create --title "Release v1.2.0 metrics" --body-file -
```

## Token permissions for posting

The default `GITHUB_TOKEN` in GitHub Actions can post to issues, PRs, and Discussions with the right workflow permissions. `GH_VELOCITY_TOKEN` is only for project board access, not posting.

| Posting target | Required permission |
|---|---|
| Issue comments | `issues: write` |
| PR comments | `pull-requests: write` |
| Discussions | `discussions: write` |

See [CI Setup: Token setup]({{< relref "/getting-started/ci-setup" >}}#token-setup) for the full token permissions matrix.

## Next steps

- [CI Setup]({{< relref "/getting-started/ci-setup" >}}) -- complete workflow examples
- [Agent Integration]({{< relref "agent-integration" >}}) -- programmatic use with JSON output
- [Ad Hoc Queries]({{< relref "ad-hoc-queries" >}}) -- more patterns for generating and sharing reports
