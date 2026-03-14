---
title: "Configuration"
weight: 3
---

# Configuration

A `.gh-velocity.yml` file is required for all metric commands. This page covers first-time setup. For the full schema reference, see [Configuration Reference]({{< relref "/reference/config" >}}).

## Generate a config with preflight

The fastest way to get started is to let preflight inspect your repo and generate a tailored config:

```bash
# Preview what preflight detects (dry run)
gh velocity config preflight -R owner/repo

# Write the config file directly
gh velocity config preflight -R owner/repo --write
```

Preflight examines your repo's labels, project boards, and recent activity, then generates a `.gh-velocity.yml` with appropriate category matchers, lifecycle labels, and project board settings.

If you are inside a local checkout, you can omit `--repo`:

```bash
cd your-repo
gh velocity config preflight --write
```

## Discover project board settings

If you use a GitHub Projects v2 board, the `config discover` command finds your project URL, status field name, and available status values:

```bash
gh velocity config discover -R owner/repo
```

Use this output to fill in the `project` and `lifecycle` sections of your config.

## Validate your config

After writing or editing your config, validate it:

```bash
gh velocity config validate
```

This checks all fields for correct types, valid ranges, and proper formats. It does not make API calls.

To see the resolved configuration with all defaults applied:

```bash
gh velocity config show
gh velocity config show -f json
```

## Minimal config

If your repo uses standard `bug` and `enhancement` labels, this is all you need:

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

This is equivalent to the defaults. You only need a config file if you want to change something -- but the file itself must exist.

## What each section does

Here is a brief overview of every config section. See [Configuration Reference]({{< relref "/reference/config" >}}) for the full schema with all options.

### `workflow`

How your team works. `pr` (default) means PRs close issues. `local` means direct commits to main (solo projects, scripts).

### `scope`

Which issues and PRs to analyze, using GitHub search query syntax:

```yaml
scope:
  query: "repo:myorg/myrepo"
```

### `quality.categories`

Ordered list of classification categories. The first matching category wins; unmatched issues are classified as "other." Matcher types:

- `label:<name>` -- exact label match
- `type:<name>` -- GitHub Issue Type
- `title:/<regex>/i` -- title regex, case-insensitive

```yaml
quality:
  categories:
    - name: bug
      match:
        - "label:bug"
        - "label:defect"
    - name: feature
      match:
        - "label:enhancement"
    - name: chore
      match:
        - "label:tech-debt"
        - "title:/^chore[\\(: ]/i"
```

{{< hint info >}}
The report's defect rate counts issues classified as "bug". If you name your bug category differently (e.g., "defect"), use `bug` as the name in the config.
{{< /hint >}}

### `quality.hotfix_window_hours`

A release is flagged as a hotfix if published within this many hours of the previous release. Default: 72 (3 days). Maximum: 8760 (1 year).

### `cycle_time`

Which strategy to use: `issue` (label-based) or `pr` (PR creation to merge). See [How It Works](../how-it-works/) for guidance on choosing.

```yaml
cycle_time:
  strategy: pr
```

### `project`

GitHub Projects v2 settings. Enables the `wip` command and backlog detection:

```yaml
project:
  url: "https://github.com/users/yourname/projects/1"
  status_field: "Status"
```

### `lifecycle`

Maps labels and project board columns to workflow stages. Labels (`match`) provide reliable cycle time timestamps. Board columns (`project_status`) power WIP and backlog detection:

```yaml
lifecycle:
  backlog:
    project_status: ["Backlog", "Triage"]
  in-progress:
    project_status: ["In progress"]
    match: ["label:in-progress", "label:wip"]
```

### `commit_ref`

Controls how the `commit-ref` linking strategy scans commit messages:

```yaml
commit_ref:
  patterns: ["closes"]           # default: only closing keywords
  # patterns: ["closes", "refs"] # also match bare #N references
```

### `exclude_users`

Exclude bot accounts from metrics:

```yaml
exclude_users:
  - "dependabot[bot]"
  - "renovate[bot]"
```

### `discussions`

GitHub Discussions integration for `--post` on bulk commands:

```yaml
discussions:
  category: General
```

## Token permissions

The default `gh auth login` token works for all local usage. In CI, different tokens cover different capabilities.

| Capability | Default `GITHUB_TOKEN` | + `GH_VELOCITY_TOKEN` (PAT with `project` scope) |
| --- | --- | --- |
| Lead time, throughput, release quality | Yes | Yes |
| Bus factor, reviews | Yes | Yes |
| `--post` to issues/PRs/Discussions | Yes | Yes |
| Cycle time (issue strategy with project board) | No | Yes |
| WIP command | No | Yes |

**If your config has no `project:` section**, you do not need `GH_VELOCITY_TOKEN`.

**If your config has a `project:` section**, commands that need the board will warn and skip that data unless `GH_VELOCITY_TOKEN` is set. The rest of the report still works.

For CI token setup details, see [CI Setup](../ci-setup/).

## Next steps

- [Configuration Reference]({{< relref "/reference/config" >}}) -- full schema with all options and defaults
- [CI Setup](../ci-setup/) -- automate reports with GitHub Actions
- [How It Works](../how-it-works/) -- understand the metrics and strategies
