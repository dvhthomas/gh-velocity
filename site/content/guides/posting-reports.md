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

Bulk commands post to GitHub Discussions. Configure the target category and optionally a title template in your [config file]({{< relref "/getting-started/configuration" >}}):

```yaml
discussions:
  category: myorg/myrepo/General
  title: "Velocity Update {{date}}"
```

The `category` is the full discussion target: `owner/repo/category`. The target repo can differ from the data source -- for example, posting to a shared `engops` repo. If the category name contains a `/`, quote it: `myorg/myrepo/"Show / Tell"`.

The `title` controls the Discussion title and is the **deduplication key** -- if a Discussion with the rendered title exists, it is updated. If not, a new one is created. See [Configuration Reference: discussions.title]({{< relref "/reference/config" >}}#discussionstitle) for the full variable reference.

When `title` is not set, the default includes the command name, repo, and date -- meaning each command creates a separate Discussion per day.

### Title template variables

| Variable | Description | Example |
|---|---|---|
| `{{date}}` | UTC date (`YYYY-MM-DD`) | `2026-03-20` |
| `{{date:FORMAT}}` | Custom date format ([Go time layout](https://pkg.go.dev/time#pkg-constants)) | `{{date:Jan 2}}` → `Mar 20` |
| `{{repo}}` | `owner/repo` | `myorg/myrepo` |
| `{{owner}}` | Repository owner | `myorg` |
| `{{command}}` | Command name | `report` |

## Idempotent posting

Running the same command with `--post` multiple times updates the existing post instead of creating a duplicate:

- **Discussions** -- matched by title. If a Discussion with the rendered title exists in the configured category, its body is updated. Human comments and edits are preserved.
- **Issue/PR comments** -- matched by a hidden signature marker in the comment body.

Multiple commands can share a Discussion when they use the same `discussions.title`. Each command's output occupies its own section in the Discussion body.

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
