# gh-velocity

A GitHub CLI extension that measures how fast your team ships.

`gh velocity` computes lead time, cycle time, release quality, and work-in-progress metrics from your existing GitHub data — issues, pull requests, releases, and commits. No external services, no tracking pixels, no configuration databases. Just your repo or your project board, or both.

**[Read the docs](https://dvhthomas.github.io/gh-velocity/)** — getting started, configuration, metric definitions, CI setup, and more.

## Install

```bash
# Install the latest stable release
gh extension install dvhthomas/gh-velocity

# Pin to a specific release
gh extension install dvhthomas/gh-velocity --pin v0.0.2

# Upgrade to the latest stable release
gh extension upgrade velocity

# Check installed version
gh velocity version
```

Requires [GitHub CLI](https://cli.github.com/) 2.0+.

**Note:** `gh extension install` and `gh extension upgrade` only use stable releases (not pre-releases like `v0.0.2-rc.1`). To install a pre-release, pin it explicitly with `--pin`.

## Quick start

A config file (`.gh-velocity.yml`) is required for all metric commands. Generate one first:

```bash
# 1. Analyze your repo and generate a tailored config
gh velocity config preflight -R owner/repo --project-url https://github.com/users/you/projects/1

# 2. Save it
gh velocity config preflight --write --project-url https://github.com/users/you/projects/1

# 3. Validate
gh velocity config validate
```

Not sure which project URL to use? Run `gh velocity config discover -R owner/repo` to list them.

Then run metrics:

```bash
# Generate a config for any repo (auto-detects labels, categories, lifecycle)
gh velocity config preflight -R cli/cli --write=/tmp/cli.yml

# How long do issues take to close? (last 30 days)
gh velocity flow lead-time --since 30d -R cli/cli --config /tmp/cli.yml

# How did the last release go?
gh velocity quality release v2.67.0 -R cli/cli --since v2.66.0 --config /tmp/cli.yml

# Everything at a glance (composite dashboard)
gh velocity report --since 30d -R cli/cli --config /tmp/cli.yml
```

From inside your own repo (with `.gh-velocity.yml` present), omit `-R` and `--config`:

```bash
cd your-repo
gh velocity flow lead-time 42
gh velocity report
```

## Commands

```
gh velocity
├── flow                              # How fast are we?
│   ├── lead-time [<issue> | --since] # Created → closed
│   ├── cycle-time [<issue> | --pr N | --since]
│   └── throughput [--since] [--until]
│
├── quality                           # How good is our output?
│   └── release <tag> [--since] [--discover]
│
├── status                            # What's happening now?
│   ├── wip                           # Work in progress
│   ├── my-week [--since]             # Your recent activity
│   └── reviews                       # PRs awaiting review
│
├── report [--since] [--until]        # Composite dashboard
│
├── config
│   ├── preflight [-R repo] [--project-url URL]  # Analyze repo, suggest config
│   ├── create                                   # Generate starter config
│   ├── discover [-R repo]                       # Find project board URLs
│   ├── show                                     # Display resolved config
│   └── validate                                 # Check for errors
│
└── version
```

### Output formats

Every command supports multiple output formats:

```bash
gh velocity report                    # human-readable (default)
gh velocity report -r json            # structured JSON (for CI/scripts)
gh velocity report -r markdown        # paste into an issue or PR
gh velocity report -r html            # self-contained HTML dashboard

# Write multiple formats at once (single data-gathering pass)
gh velocity report --results md,json,html --write-to ./out
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

Discussion posting requires `discussions.category` (e.g., `General`) in your config. The tool resolves the name to a GraphQL ID at runtime.

### CI / GitHub Actions

gh-velocity works in GitHub Actions. The default `GITHUB_TOKEN` handles most metrics, but **cannot access Projects v2 boards** (a GitHub platform limitation). For cycle time and WIP, set `GH_VELOCITY_TOKEN` to a PAT with `project` scope — the binary picks it up automatically. See [Token permissions](docs/guide.md#token-permissions) in the guide.

```yaml
# .github/workflows/velocity-report.yml
name: Velocity Report
on:
  schedule:
    - cron: '0 9 * * 1'  # Monday 9am UTC

permissions:
  contents: read
  issues: write         # --post to issues/PRs
  discussions: write    # --post bulk reports

jobs:
  report:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install dvhthomas/gh-velocity
      - run: gh velocity report --since 30d --post -r markdown
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_TOKEN: ${{ secrets.GH_VELOCITY_TOKEN }}
          GH_VELOCITY_POST_LIVE: 'true'
```

### Common flags

| Flag | Short | Description |
| --- | --- | --- |
| `--results` | `-r` | Output format(s): `pretty` (default), `json`, `markdown`, `html` |
| `--write-to` | | Write result files to a directory (silences stdout) |
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
- **My Week** — your recent activity: issues closed, PRs merged, what's ahead
- **Reviews** — PRs awaiting code review, with staleness detection (>48h)

### Report (composite dashboard)

- Lead time, cycle time, throughput, WIP, and quality in a single view
- Each section computes independently — a failure in one doesn't block others

## How issues are discovered

Three strategies run in parallel to find which issues belong to a release:

1. **pr-link** — merged PRs in the release window → GitHub closing references → linked issues
2. **commit-ref** — commit messages scanned for `fixes #N`, `closes #N`, `resolves #N`
3. **changelog** — release body parsed for `#N` references

Results merge with priority (pr-link > commit-ref > changelog). Use `--discover` to see what each strategy finds.

## Configuration

A config file (`.gh-velocity.yml`) is required for all metric commands. The fastest way to create one:

### Generate a tailored config

```bash
# Preflight analyzes your repo's labels, project boards, and recent activity
gh velocity config preflight -R owner/repo --project-url https://github.com/users/you/projects/1

# Save directly
gh velocity config preflight --write --project-url https://github.com/users/you/projects/1
```

### Manual config

Create `.gh-velocity.yml` in your repo root:

```yaml
# Issue/PR classification — first matching category wins; unmatched = "other".
# Matchers: label:<name>, type:<name>, title:/<regex>/i
quality:
  categories:
    - name: bug
      match:
        - "label:bug"
        - "label:defect"
    - name: feature
      match:
        - "label:enhancement"
  hotfix_window_hours: 72

# Commit message patterns
commit_ref:
  patterns: ["closes"]    # "refs" also matches bare #N references

# Cycle time strategy: "issue" (default) or "pr"
cycle_time:
  strategy: issue

# Projects v2 board (enables WIP and lifecycle-based cycle time)
# Find your project URL with: gh velocity config discover -R owner/repo
# project:
#   url: "https://github.com/users/yourname/projects/1"
#   status_field: "Status"

# Lifecycle stages: map project board columns to workflow stages
# lifecycle:
#   backlog:
#     project_status: ["Backlog", "Triage"]
#   in-progress:
#     project_status: ["In progress"]

# Exclude bot accounts from metrics
# exclude_users:
#   - "dependabot[bot]"
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
