---
title: "Posting Reports"
weight: 4
---

# Posting Reports

gh-velocity can post metrics directly to GitHub using the `--post` flag. Reports go to GitHub Discussions (for bulk commands like `report` and `quality release`) or as issue/PR comments (for single-item commands like `flow lead-time 42`).

## The --post flag

Every command supports `--post`:

```bash
gh velocity report --since 30d --post
gh velocity quality release v1.2.0 --post
gh velocity flow lead-time 42 --post
```

By default, `--post` runs in **dry-run mode**. It shows what would be posted without actually writing to GitHub. To post for real, set the `GH_VELOCITY_POST_LIVE` environment variable:

```bash
GH_VELOCITY_POST_LIVE=true gh velocity report --since 30d --post
```

This safety net prevents accidental posts during local testing.

## Discussions config

Bulk commands post to GitHub Discussions. Configure the target category in your [config file]({{< relref "/getting-started/configuration" >}}):

```yaml
discussions:
  category: General
```

The tool creates a new Discussion in the specified category with the report as the body. The Discussion title includes the command, repo, and date range for easy identification.

## Idempotent posting

When you run the same command with `--post` multiple times, the tool updates the existing Discussion or comment instead of creating a duplicate. It matches on the title (for Discussions) or a signature comment (for issue/PR comments).

To force a new post instead of updating, use `--new-post`:

```bash
gh velocity report --since 30d --new-post
```

`--new-post` implies `--post`, so you do not need both flags.

## Posting in CI

The most common CI pattern is a weekly report posted to Discussions on a cron schedule. Here is the minimal workflow:

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

- **`GH_VELOCITY_POST_LIVE: 'true'`** is required in CI for `--post` to actually write. Without it, the run succeeds but nothing is posted.
- **`permissions: discussions: write`** is required for the `GITHUB_TOKEN` to create Discussions.
- **`permissions: issues: write`** is required for posting comments on issues or PRs.
- **`workflow_dispatch`** lets you trigger the workflow manually from the GitHub UI for testing.

See [CI Setup]({{< relref "/getting-started/ci-setup" >}}) for complete workflow examples including release metrics, PR lead-time checks, and scheduled trend reports.

## Posting patterns

### Weekly team report to Discussions

Post a trailing-window report every Monday:

```bash
GH_VELOCITY_POST_LIVE=true gh velocity report --since 30d --post --results markdown
```

The idempotent behavior means re-running this manually during the week updates the existing Discussion rather than creating a new one.

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

The default `GITHUB_TOKEN` in GitHub Actions can post to issues, PRs, and Discussions with the right workflow permissions. You do not need `GH_VELOCITY_TOKEN` for posting -- that token is only for project board access.

| Posting target | Required permission |
|---|---|
| Issue comments | `issues: write` |
| PR comments | `pull-requests: write` |
| Discussions | `discussions: write` |

See [CI Setup: Token setup]({{< relref "/getting-started/ci-setup" >}}#token-setup) for the full token permissions matrix.

## Next steps

- [CI Setup]({{< relref "/getting-started/ci-setup" >}}) -- complete workflow examples
- [Agent Integration]({{< relref "agent-integration" >}}) -- programmatic use with JSON output
- [Recipes]({{< relref "recipes" >}}) -- more patterns for generating and sharing reports
