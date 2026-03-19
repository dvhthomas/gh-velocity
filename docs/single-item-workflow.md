# Automatic metrics on issues and PRs

Post lead-time and cycle-time comments automatically when issues close or PRs merge. Each comment appears on the item itself — no bulk reports, no manual steps.

**Already have a working `.gh-velocity.yml`?** Skip to [The workflow](#the-workflow).

## Prerequisites

- [gh CLI](https://cli.github.com/) installed (comes pre-installed on GitHub Actions runners)
- gh-velocity installed: `gh extension install dvhthomas/gh-velocity`
- A `.gh-velocity.yml` file in your repository root (see [Configuration](#configuration))

## Configuration

gh-velocity requires a `.gh-velocity.yml` file in your repo root. For single-item lead-time and cycle-time, the defaults are sufficient — the file just needs to exist.

A practical starting point with category classification:

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

Or let gh-velocity auto-detect categories from your repo's labels and issues:

```bash
gh velocity config preflight --write
```

This probes your repo and generates a `.gh-velocity.yml` with evidence-based category matchers.

For the full configuration reference, see the [Configuration section](guide.md#configuration-reference) of the guide.

## The workflow

Create `.github/workflows/velocity-item.yaml`:

```yaml
name: Item Metrics

on:
  issues:
    types: [closed]
  pull_request:
    types: [closed]
  workflow_dispatch: # Manual testing from the Actions tab

permissions:
  contents: read
  issues: write
  pull-requests: write

jobs:
  lead-time:
    # Only when an issue is closed as completed (not "not planned")
    if: >-
      github.event_name == 'issues' &&
      github.event.issue.state_reason == 'completed'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - run: gh extension install dvhthomas/gh-velocity

      - name: Post lead time to issue
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_POST_LIVE: "true"
        run: |
          gh velocity flow lead-time ${{ github.event.issue.number }} --post

  cycle-time:
    # Only when a PR is actually merged (not just closed)
    if: >-
      github.event_name == 'pull_request' &&
      github.event.pull_request.merged == true
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - run: gh extension install dvhthomas/gh-velocity

      - name: Post cycle time to PR
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_POST_LIVE: "true"
        run: |
          gh velocity flow cycle-time --pr ${{ github.event.pull_request.number }} --post
```

Copy this file into your repo and commit it. No other changes needed — `GITHUB_TOKEN` is provided automatically by GitHub Actions.

### Trigger filtering explained

The workflow uses two `if:` conditions to avoid running on items that were not completed:

| Trigger | Condition | Why |
| --- | --- | --- |
| `issues: [closed]` | `state_reason == 'completed'` | Skips issues closed as "not planned" |
| `pull_request: [closed]` | `merged == true` | Skips PRs that were closed without merging |

**Note on `state_reason`:** Issues closed via some bots or older API integrations may have a null `state_reason`. The `== 'completed'` check skips these. If you want lead-time comments on all closed issues regardless of reason, remove the `state_reason` condition:

```yaml
if: github.event_name == 'issues'
```

### Permissions explained

| Permission | Required by |
| --- | --- |
| `contents: read` | Checking out the repo to read `.gh-velocity.yml` |
| `issues: write` | Posting lead-time comments on issues |
| `pull-requests: write` | Posting cycle-time comments on PRs |

### Why the workflow uses `--pr` for cycle time

The cycle-time job uses `cycle-time --pr <number>` rather than `cycle-time <number>`. This matters:

- **`--pr`** measures PR creation to merge. It works with just `GITHUB_TOKEN` and has no external dependencies.
- **Without `--pr`**, cycle-time uses your configured strategy (often project-board-based), which requires `GH_VELOCITY_TOKEN` and is sensitive to board timestamp accuracy.

The `--pr` flag is the right choice for this workflow — it measures the code review cycle for the specific PR being merged, using only data GitHub provides natively.

### When you need GH_VELOCITY_TOKEN

The workflow above uses only `GITHUB_TOKEN`, which is sufficient for lead-time and PR-based cycle-time.

If your config has a `project:` section and you want issue-based cycle time (from board status changes), you also need `GH_VELOCITY_TOKEN` — a classic PAT with `project` scope. See the [CI integration guide](guide.md#setting-up-gh_velocity_token-for-ci) for setup instructions.

## How posting works

The `--post` flag tells gh-velocity to post its output as a comment on the relevant issue or PR. Key behaviors:

- **Dry-run by default.** Without `GH_VELOCITY_POST_LIVE=true`, `--post` shows what would be posted but does not write to GitHub. The workflow above sets this variable so comments are posted for real.
- **Idempotent updates.** If an issue is reopened and closed again, the existing comment is updated rather than duplicated. Each comment includes a hidden marker that gh-velocity uses to find and update it.
- **Format is automatic.** The `--post` flag produces markdown output suitable for GitHub comments. Do not combine with `--results json` — that would post raw JSON as the comment body.

For more on posting mechanics, see the [Posting Reports guide](../site/content/guides/posting-reports.md).

## What to expect

### What the comments look like

On an issue, the lead-time comment shows a single-row table:

| Issue | Title | Created (UTC) | Lead Time |
| ---: | --- | --- | --- |
| #119 | feat(preflight): auto-detect and exclude noise labels | 2026-03-18 | 1m  (created -> closed) |

On a PR, the cycle-time comment is similar:

| PR | Title | Started (UTC) | Cycle Time |
| ---: | --- | --- | --- |
| #125 | feat: HTML format, insight flags, and cleanup | 2026-03-19 | 27m  (created -> merged) |

### Two comments from one PR merge

When a PR with "Closes #42" is merged, GitHub fires both the `issues[closed]` and `pull_request[closed]` events. Both workflow jobs run:

- **Lead-time comment** appears on issue #42 (time from issue creation to close)
- **Cycle-time comment** appears on the PR (time from PR creation to merge)

This is intentional — each metric belongs on the item it measures.

### Fork PRs

PRs from forks receive a restricted `GITHUB_TOKEN` that may not have write access to the base repository. The cycle-time job may fail silently for fork PRs. This is a GitHub Actions platform restriction, not a gh-velocity limitation.

### Concurrent runs

If multiple issues close or PRs merge at the same time, each workflow run is independent — no shared state, no conflicts.

## Testing locally

Before committing the workflow, verify the commands work against your repo:

```bash
# Dry-run (no comment posted, just shows output)
gh velocity flow lead-time 42 --post

# Live (posts the comment)
GH_VELOCITY_POST_LIVE=true gh velocity flow lead-time 42 --post
```

Replace `42` with a real issue number in your repo. Same for cycle-time:

```bash
gh velocity flow cycle-time --pr 10 --post
```

Once you see the expected output, commit the workflow file and the comments will appear automatically on future closures and merges.

## Troubleshooting

**Comment not appearing on the issue/PR**

1. Check the Actions tab — did the workflow run? Look for the "Item Metrics" workflow.
2. Is `GH_VELOCITY_POST_LIVE: "true"` set in the env block? Without it, `--post` runs in dry-run mode.
3. Check the workflow permissions — `issues: write` and `pull-requests: write` are both required.
4. Run with `--debug` to see diagnostic output: `gh velocity flow lead-time 42 --post --debug`

**Workflow runs but lead-time job is skipped**

The issue may have been closed as "not planned" or with a null `state_reason`. Check the issue's close reason in the GitHub UI (it appears as a label next to the closed status). See [Trigger filtering explained](#trigger-filtering-explained) for details.

**Config file not found**

The workflow checks out your repo with `actions/checkout`, which places `.gh-velocity.yml` in the working directory. Make sure the config file is committed to the branch that triggers the workflow (usually your default branch).

For more troubleshooting, see the [Troubleshooting section](guide.md#troubleshooting) of the main guide.
