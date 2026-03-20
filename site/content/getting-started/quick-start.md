---
title: "Quick Start"
weight: 1
---

# Quick Start

Install gh-velocity and run your first metrics in under 5 minutes.

## Prerequisites

The [GitHub CLI](https://cli.github.com/) must be installed and authenticated:

```bash
# Install (macOS shown -- see gh docs for Linux/Windows)
brew install gh

# Authenticate
gh auth login
```

Private repos require at least `repo` scope. Public repos work without special scopes.

## Install

```bash
gh extension install dvhthomas/gh-velocity
```

Verify it works:

```bash
gh velocity version
```

## Generate a config file

All metric commands require a `.gh-velocity.yml` file. The `preflight` command analyzes your repo and generates one:

```bash
cd your-repo
gh velocity config preflight --write
```

This inspects your repo's labels, project boards, PRs, and issues, then writes a starter config:

```
Analyzing dvhthomas/gh-velocity...
  Labels: 8 found
  Projects: 1 board (gh-velocity tracker)
  PRs (last 30d): 14
  Issues (last 30d): 22

Wrote .gh-velocity.yml
```

> [!TIP]
> Preflight reads your repo's actual labels and recent issues, then suggests matchers based on what it finds. The generated config includes match evidence showing how many issues each matcher would catch -- check the comments in `.gh-velocity.yml`.

## Run your first report

```bash
gh velocity report --since 30d
```

This produces a dashboard covering the last 30 days:

- **Lead time** -- how long from issue creation to close
- **Cycle time** -- how long from work started to close
- **Throughput** -- how many items closed per week
- **Velocity** -- effort delivered per iteration (if configured)

Each section computes independently. If a section lacks the required config (for example, no project board for velocity), it is skipped -- the rest still appears.

The same report as structured JSON:

```bash
gh velocity report --since 30d --results json | jq '.lead_time.stats'
```

## Analyze a specific release

If your repo uses tags for releases:

```bash
gh velocity quality release v1.0.0
```

This shows release composition (bugs vs features), per-issue timing, and aggregate statistics.

## Analyze remote repos

Cloning is not required. Use `--repo` to analyze any accessible repository:

```bash
gh velocity report --since 30d --repo cli/cli
```

> [!TIP]
> Running from inside a local clone is faster: gh-velocity uses local git for tag listing and commit history. It also enables the `commit-ref` [linking strategy]({{< relref "/concepts/linking-strategies" >}}), which discovers issues not linked via PRs.

## Next steps

- [How It Works]({{< relref "/getting-started/how-it-works" >}}) -- understand how your GitHub workflow maps to metrics
- [Configuration]({{< relref "/getting-started/configuration" >}}) -- customize your config for lifecycle tracking, quality categories, and velocity
- [CI Setup]({{< relref "/getting-started/ci-setup" >}}) -- automate reports with GitHub Actions
- [Interpreting Results]({{< relref "/guides/interpreting-results" >}}) -- understand what the numbers mean
