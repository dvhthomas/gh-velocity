---
title: "Quick Start"
weight: 1
---

# Quick Start

Install gh-velocity and run your first metrics query in under 5 minutes.

## Prerequisites

You need the [GitHub CLI](https://cli.github.com/) installed and authenticated.

```bash
# macOS
brew install gh

# Linux
sudo apt install gh    # Debian/Ubuntu
sudo dnf install gh    # Fedora

# Windows
winget install GitHub.cli
```

Then authenticate:

```bash
gh auth login
```

You need at least `repo` scope for private repositories. For public repos, no special scopes are required.

## Install the extension

```bash
# Latest stable release
gh extension install dvhthomas/gh-velocity

# Or pin a specific version
gh extension install dvhthomas/gh-velocity --pin v0.0.2
```

Verify the installation:

```bash
gh velocity version
```

To upgrade later:

```bash
gh extension upgrade velocity
```

{{< hint info >}}
`gh extension upgrade` installs the latest stable release. Pre-releases (e.g., `v0.0.2-rc.1`) must be pinned explicitly with `--pin`.
{{< /hint >}}

## First query against a public repo

All metric commands require a config file. When targeting a remote repo with `--repo`, use `--config` to point at an example config (see `docs/examples/` in the repo). From inside your own repo with `.gh-velocity.yml` present, the config is loaded automatically.

Start with a public repo to see what the output looks like:

```bash
# Release report for the GitHub CLI itself
gh velocity quality release v2.67.0 -R cli/cli
```

This takes 10-30 seconds depending on the number of issues. You will see:

- Release metadata (previous tag, cadence, hotfix status)
- Composition breakdown (bugs, features, other)
- Per-issue table with lead time, cycle time, release lag, and outlier flags
- Aggregate statistics with P90, P95, and outlier counts

Try the same report in JSON to see the full structured data:

```bash
gh velocity quality release v2.67.0 -R cli/cli -f json | jq '.aggregates.lead_time'
```

```json
{
  "count": 17,
  "mean_seconds": 24271200,
  "median_seconds": 5248800,
  "stddev_seconds": 43981056,
  "p90_seconds": 134236800,
  "p95_seconds": 138499200,
  "outlier_cutoff_seconds": 119448000,
  "outlier_count": 2
}
```

See which linking strategy found each issue:

```bash
gh velocity quality release v2.67.0 -R cli/cli --discover
```

This shows what `pr-link`, `commit-ref`, and `changelog` each discovered, and the merged result. Use it to understand how well the strategies cover your workflow.

## Running against your own repo

From inside a local checkout, you can omit `--repo`:

```bash
cd your-repo
gh velocity quality release v1.0.0
```

When run from inside a repo, the tool uses local git for tag listing and commit history. This is faster and enables the `commit-ref` linking strategy.

You can also query individual issues without cloning:

```bash
# Works remotely -- uses PR creation date, assignment, or project status
gh velocity flow cycle-time 42 -R owner/repo
```

From inside a local checkout, the tool enriches results with commit counts and can use the earliest commit as a fallback cycle time signal:

```bash
# From inside a clone -- enriched with commit data
gh velocity flow cycle-time 42
```

## Next steps

- [How It Works](../how-it-works/) -- understand how your GitHub workflow maps to metrics
- [Configuration](../configuration/) -- generate a config file tailored to your repo
- [CI Setup](../ci-setup/) -- automate reports with GitHub Actions
