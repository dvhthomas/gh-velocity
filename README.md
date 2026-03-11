# gh-velocity

A GitHub CLI extension that measures how fast your team ships.

`gh velocity` computes lead time, cycle time, release quality, and work-in-progress metrics from your existing GitHub data — issues, pull requests, releases, and commits. No external services, no tracking pixels, no configuration databases. Just your repo.

## Install

```
gh extension install dvhthomas/gh-velocity
```

Requires [GitHub CLI](https://cli.github.com/) 2.0+.

## Quick start

Try it now against any public repo — no config needed:

```bash
# How long do issues take to close? (last 30 days)
gh velocity flow lead-time --since 30d -R cli/cli

# How did the last release go?
gh velocity quality release v2.67.0 -R cli/cli --since v2.66.0

# Everything at a glance (composite dashboard)
gh velocity report --since 30d -R cli/cli
```

From inside your own repo, omit `-R`:

```bash
cd your-repo
gh velocity flow lead-time 42
gh velocity report
```

### Set up your repo (optional but recommended)

```bash
# 1. Analyze your repo and generate a tailored config
gh velocity config preflight -R owner/repo --project 3

# 2. Save it
gh velocity config preflight --write --project 3

# 3. Validate
gh velocity config validate
```

Not sure which project number to use? Run `gh velocity config discover -R owner/repo` to list them.

## Commands

```
gh velocity
├── flow                              # How fast are we?
│   ├── lead-time [<issue> | --since] # Created → closed
│   ├── cycle-time [<issue> | --pr N | --since]
│   └── throughput [--since] [--until]
│
├── quality                           # How good is our output?
│   └── release <tag> [--since] [--scope]
│
├── status                            # What's happening now?
│   └── wip                           # Work in progress
│
├── report [--since] [--until]        # Composite dashboard
│
├── config
│   ├── preflight [-R repo] [--project N]  # Analyze repo, suggest config
│   ├── create                             # Generate starter config
│   ├── discover [-R repo]                 # Find project board IDs
│   ├── show                               # Display resolved config
│   └── validate                           # Check for errors
│
└── version
```

### Output formats

Every command supports three formats:

```bash
gh velocity report                    # human-readable (default)
gh velocity report -f json            # structured JSON (for CI/scripts)
gh velocity report -f markdown        # paste into an issue or PR
```

### Posting results to GitHub

Use `--post` to write results back to GitHub as comments or Discussion posts:

```bash
# Post lead time as a comment on issue #42
GH_VELOCITY_POST_LIVE=true gh velocity flow lead-time 42 --post

# Post a 30-day report as a Discussion
GH_VELOCITY_POST_LIVE=true gh velocity report --since 30d --post

# Force a new comment (skip idempotent update)
GH_VELOCITY_POST_LIVE=true gh velocity flow lead-time 42 --new-post
```

**Safety:** `--post` runs in **dry-run mode by default** — it prints what it would do but makes no mutations. Set `GH_VELOCITY_POST_LIVE=true` to enable live writes. This prevents tests, agents, and accidental runs from modifying GitHub state.

| Command type | Post target |
| --- | --- |
| Single issue (`flow lead-time 42`) | Issue comment |
| Single PR (`flow cycle-time --pr 5`) | PR comment |
| Bulk (`report --since 30d`) | Discussion post |

Discussion posting requires `discussions.category_id` in your config (find it with `config discover`).

### CI / GitHub Actions

gh-velocity detects `GITHUB_ACTIONS=true` and emits structured log annotations:

```yaml
# .github/workflows/velocity-report.yml
name: Velocity Report
on:
  schedule:
    - cron: '0 9 * * 1'  # Monday 9am UTC
jobs:
  report:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install dvhthomas/gh-velocity
      - run: gh velocity report --since 30d --post -f markdown
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_POST_LIVE: 'true'
```

### Common flags

| Flag | Short | Description |
| --- | --- | --- |
| `--format` | `-f` | Output: `pretty` (default), `json`, `markdown` |
| `--repo` | `-R` | Target repo as `owner/name` |
| `--since` | | Start of date window or previous tag |
| `--until` | | End of date window (default: now) |
| `--post` | | Post output to GitHub (dry-run by default) |
| `--new-post` | | Force a new post, skip idempotent update (implies `--post`) |
| `--debug` | | Print diagnostic info to stderr |

## What gets measured

### Flow metrics

- **Lead time** — issue created to issue closed
- **Cycle time** — work started to issue closed (configurable strategy: issue, PR, or project board)

### Quality metrics (per release)

- **Per-issue lead time, cycle time, and release lag** (closed → released)
- **Composition** — categorize issues by label (bug, feature, or custom categories)
- **Hotfix detection** — flag issues closed very close to release
- **Aggregates** — mean, median, P90, P95, IQR-based outlier detection

### Status

- **WIP** — items currently in progress (from Projects v2 board or labels)

### Report (composite dashboard)

- Lead time, cycle time, throughput, WIP, and quality in a single view
- Each section computes independently — a failure in one doesn't block others

## How issues are discovered

Three strategies run in parallel to find which issues belong to a release:

1. **pr-link** — merged PRs in the release window → GitHub closing references → linked issues
2. **commit-ref** — commit messages scanned for `fixes #N`, `closes #N`, `resolves #N`
3. **changelog** — release body parsed for `#N` references

Results merge with priority (pr-link > commit-ref > changelog). Use `--scope` to see what each strategy finds.

## Configuration

All fields are optional. `gh velocity` works without a config file using sensible defaults.

### Generate a tailored config

```bash
# Preflight analyzes your repo's labels, project boards, and recent activity
gh velocity config preflight -R owner/repo --project 3

# Save directly
gh velocity config preflight --write --project 3
```

### Manual config

Create `.gh-velocity.yml` in your repo root:

```yaml
# Issue classification
quality:
  bug_labels: ["bug", "defect"]
  feature_labels: ["enhancement"]
  hotfix_window_hours: 72

# Commit message patterns
commit_ref:
  patterns: ["closes"]    # "refs" also matches bare #N references

# Cycle time strategy: "issue" (default), "pr", or "project-board"
cycle_time:
  strategy: issue

# Projects v2 board (enables WIP and project-board cycle time)
# Find these IDs with: gh velocity config discover
# project:
#   id: "PVT_kwDOAbc123"
#   status_field_id: "PVTSSF_kwDOAbc123"

# Label-based status (alternative to project board)
# statuses:
#   active_labels: ["in-progress", "wip"]
#   backlog_labels: ["backlog", "icebox"]
```

See [docs/guide.md](docs/guide.md) for the full configuration reference, metric definitions, and examples for popular OSS repos.

## Example output

```
$ gh velocity report --since 30d -R cli/cli

Report: cli/cli (2026-02-08 – 2026-03-10 UTC)

  Lead Time:   median 4.2d, mean 12.1d, P90 30.5d (n=47, 3 outliers)
  Cycle Time:  median 1.1d, mean 3.8d, P90 9.2d (n=47)
  Throughput:  47 issues closed, 52 PRs merged
```

```
$ gh velocity flow lead-time 9876 -R cli/cli

Issue #9876  Fix table alignment in `gh pr list`
  Created:   2026-02-15T14:30:00Z UTC
  Lead Time: 3d 4h 12m
```

## License

MIT
