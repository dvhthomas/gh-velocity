---
title: GitHub Actions Integration and --post Implementation
date: 2026-03-10
status: completed
type: brainstorm
---

# GitHub Actions Integration and --post Implementation

## What We're Building

Two related capabilities that make gh-velocity a first-class CI citizen:

1. **Strong stdout/stderr separation** so CLI output is pipe-safe and machine-parseable in GitHub Actions workflows
2. **`--post` flag** that writes metric output directly to GitHub (issue comments for single items, Discussion posts for bulk/report commands)

## Why This Approach

### CI-first design

gh-velocity already has excellent stdout/stderr discipline — all primary output goes through `cmd.OutOrStdout()`, warnings/debug/errors to stderr. This means `gh velocity report -f json | jq` already works. The goal is to make CI usage trivially easy with example workflow snippets and posting that "just works."

### Distribution: CLI-only (no action.yml)

Users install the gh extension and call it directly in workflow steps. One distribution path, works everywhere the gh CLI works. No second versioning surface to maintain.

### Idempotent posting with HTML comment markers

Repeated CI runs should update existing posts, not create duplicates. This is critical for agent-native usage where tools may call `--post` repeatedly.

## Key Decisions

### 1. Posting targets by command type

| Command type | Post target | Example |
| --- | --- | --- |
| Single issue (`flow lead-time 42`) | Comment on issue #42 | Find/update marker comment on #42 |
| Single PR (`flow cycle-time --pr 5`) | Comment on PR #5 | Find/update marker comment on PR #5 |
| Bulk (`flow lead-time --since 30d`) | GitHub Discussion | Find/update discussion in configured category |
| Report (`report --since 30d`) | GitHub Discussion | Find/update discussion in configured category |
| Release (`quality release v1.0`) | GitHub Discussion | Find/update discussion in configured category |

### 2. Marker format: structured HTML comments

Opening and closing tags wrap the posted content so it can be found and replaced:

```
<!-- gh-velocity:lead-time:42 -->
| Metric | Value |
| --- | --- |
| Lead Time | 3d 4h 12m |
<!-- /gh-velocity -->
```

For bulk/report:
```
<!-- gh-velocity:report:30d -->
...full report markdown...
<!-- /gh-velocity -->
```

Markers are colon-delimited: `gh-velocity:{command}:{context}`. The closing tag is always `<!-- /gh-velocity -->`.

### 3. --post always writes stdout too

`--post` is a side-effect, not a replacement for stdout. The markdown output still goes to stdout so CI can capture/archive it. Posting confirmation goes to stderr:

```
# stdout: the markdown content
# stderr: "Posted to cli/cli#42 (updated)" or "Posted new discussion: https://..."
```

### 4. --new-post forces a fresh post

Default behavior is idempotent (find existing marker, update in-place). `--new-post` creates a new comment/discussion regardless. The flag name follows existing convention (`--new-post`, not `--post-new`).

### 5. --post implies --format markdown

When `--post` is used without an explicit `--format`, the format defaults to `markdown` (not `pretty`). The user can override with `-f json` if they want JSON posted to GitHub (unusual but valid).

### 6. Discussions require config

Bulk posting requires `discussions.category_id` in `.gh-velocity.yml`. The config field already exists and is validated (`^DIC_[a-zA-Z0-9]+$`). Without it, `--post` on bulk commands returns a clear error:

```
--post requires discussions.category_id for bulk commands

  To find your category ID:  gh api graphql ...
  Add to .gh-velocity.yml:
    discussions:
      category_id: "DIC_abc123"
```

### 7. Auth: uses existing GITHUB_TOKEN

No new auth mechanism. The `gh` CLI's token (from `gh auth login` or `GITHUB_TOKEN` in Actions) provides the permissions. Required scopes:
- `issues: write` for issue/PR comments
- `discussions: write` for discussion posts

## Posting Algorithm

### Single issue/PR comment:

1. List existing comments on the issue/PR
2. Search for `<!-- gh-velocity:{command}:{number} -->`
3. If found: update the comment body (replace everything between markers)
4. If not found: create a new comment with markers wrapping the content

### Bulk/report discussion:

1. Search discussions in the configured category for marker `<!-- gh-velocity:{command}:{context} -->`
2. If found: update the discussion body
3. If not found: create a new discussion with title like "gh-velocity report: cli/cli (2026-03-10)"
4. `--new-post` skips the search and always creates

## Example CI Workflow

```yaml
name: Velocity Report
on:
  schedule:
    - cron: '0 9 * * 1'  # Monday 9am
  workflow_dispatch:

permissions:
  issues: write
  discussions: write

jobs:
  report:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install dvhthomas/gh-velocity
      - run: gh velocity report --since 30d --post
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### 8. Structured CI logging

Use GitHub Actions workflow commands for structured log output on stderr so that CI platforms correctly surface failures and successes:

```
::error::--post failed: token lacks discussions:write permission
::warning::results capped at 1000; narrow the date range
::notice::Posted to cli/cli#42 (updated)
```

When `GH_ACTIONS=true` (or `GITHUB_ACTIONS=true`, which Actions sets automatically), format stderr messages as workflow commands. Outside Actions, use plain text. This gives Actions-native collapsible groups, annotations, and error highlighting for free.

### 9. Permissions errors include exact fix

When posting fails due to missing permissions, the error message includes the exact YAML to add to the workflow:

```
--post failed: insufficient permissions for discussions:write

  Add to your GitHub Actions workflow:
    permissions:
      discussions: write
```

## Resolved Questions

- **Release posting target**: Discussion (consistent with other bulk commands; release body is owned by the author)
- **Permissions error UX**: Include exact Actions permission YAML in error messages
- **Structured CI logging**: Use GitHub Actions workflow commands (::error::, ::warning::, ::notice::) when running in Actions

### 10. Preflight checks posting readiness

`config preflight` should also validate the environment for posting:

- **Discussions enabled?** — query the repo to check if Discussions are turned on
- **Token permissions?** — attempt a lightweight API call to verify `issues:write` and `discussions:write` scopes
- **Category exists?** — if `discussions.category_id` is configured, verify it resolves

Preflight hints would include:
```
# Discussions are enabled on this repo
# Token has issues:write permission
# Token lacks discussions:write — add to your workflow permissions
# Discussion category DIC_abc123 found: "Engineering Metrics"
```

This makes preflight the single "is everything ready?" check before enabling `--post`.

The JSON output (`-f json`) should be agent-parseable with clear boolean fields:

```json
{
  "repo": "cli/cli",
  "strategy": "issue",
  "has_project": false,
  "posting_readiness": {
    "discussions_enabled": true,
    "has_issues_write": true,
    "has_discussions_write": false,
    "category_valid": null
  },
  "hints": [
    "Token lacks discussions:write — add to your workflow permissions"
  ]
}
```

An agent (or a CI setup script) can parse `posting_readiness` to determine what's missing and fix it programmatically — no human interpretation needed.

## Open Questions

None — all resolved above.

## Out of Scope

- action.yml wrapper (decided: CLI-only distribution)
- Slack/webhook posting (future, separate feature)
- Automated scheduling within the tool (use cron in Actions)
