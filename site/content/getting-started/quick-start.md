---
title: "Quick Start"
weight: 1
---

# Quick Start

Install gh-velocity and run your first metrics in under 5 minutes.

## Prerequisites

You need the [GitHub CLI](https://cli.github.com/) installed and authenticated:

```bash
# Install (macOS shown — see gh docs for Linux/Windows)
brew install gh

# Authenticate
gh auth login
```

For private repos you need at least `repo` scope. Public repos work without special scopes.

## Install

```bash
gh extension install dvhthomas/gh-velocity
```

Verify it works:

```bash
gh velocity version
```

## Generate a config file

All metric commands require a `.gh-velocity.yml` file. The `preflight` command analyzes your repo and generates one for you:

```bash
cd your-repo
gh velocity config preflight --write
```

This inspects your repo's labels, project boards, PRs, and issues, then writes a starter config. You'll see output like:

```
Analyzing dvhthomas/gh-velocity...
  Labels: 8 found
  Projects: 1 board (gh-velocity tracker)
  PRs (last 30d): 14
  Issues (last 30d): 22

Wrote .gh-velocity.yml
```

> [!TIP]
> How did preflight know which labels to use? It reads your repo's actual labels and recent issues, then suggests matchers based on what it finds. The generated config includes match evidence showing how many issues each matcher would catch — check the comments in `.gh-velocity.yml` to see.

## Run your first report

```bash
gh velocity report --since 30d
```

This produces a composite dashboard covering the last 30 days:

- **Lead time** — how long from issue creation to close
- **Cycle time** — how long from work started to close
- **Throughput** — how many items closed per week
- **Velocity** — effort delivered per iteration (if configured)

Each section computes independently. If your config doesn't have everything set up yet (for example, no project board for velocity), those sections are gracefully skipped — you still see the rest.

Try the same report as structured JSON:

```bash
gh velocity report --since 30d --format json | jq '.lead_time.stats'
```

## Analyze a specific release

If your repo uses tags for releases:

```bash
gh velocity quality release v1.0.0
```

This shows release composition (bugs vs features), per-issue timing, and aggregate statistics.

## Analyze remote repos

You don't need to clone a repo to analyze it. Use `--repo` (short: `-R`):

```bash
gh velocity report --since 30d -R cli/cli
```

> [!TIP]
> When you run from inside a local clone, gh-velocity uses local git for faster tag listing and commit history. It also enables the `commit-ref` [linking strategy]({{< relref "/concepts/linking-strategies" >}}), which can discover issues that aren't linked via PRs.

## Next steps

- [How It Works]({{< relref "/getting-started/how-it-works" >}}) — understand how your GitHub workflow maps to metrics
- [Configuration]({{< relref "/getting-started/configuration" >}}) — customize your config for lifecycle tracking, quality categories, and velocity
- [CI Setup]({{< relref "/getting-started/ci-setup" >}}) — automate reports with GitHub Actions
- [Interpreting Results]({{< relref "/guides/interpreting-results" >}}) — understand what the numbers mean
