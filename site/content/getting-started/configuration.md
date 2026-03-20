---
title: "Configuration"
weight: 3
---

# Configuration

All metric commands require a `.gh-velocity.yml` file. Run `gh velocity config preflight --write` to generate one. This page covers first-time setup; for the full schema, see [Configuration Reference]({{< relref "/reference/config" >}}).

## Generate a config with preflight

The fastest way to get started is to let preflight inspect your repo and generate a tailored config:

```bash
cd your-repo
gh velocity config preflight --write
```

Preflight examines your repo's labels, project boards, and recent activity, then generates a `.gh-velocity.yml` with appropriate **matchers** (patterns like `label:bug` that classify issues), lifecycle labels, and project board settings.

The generated config includes **match evidence** — comments showing how many issues each matcher would catch:

```yaml
# Match evidence (last 30 days of issues + PRs):
#   bug / label:bug — 33 matches, e.g. #12893 error parsing "input[title]"...
#   feature / label:enhancement — 37 matches, e.g. #12862 Remove a book...
#   chore / label:tech-debt — 0 matches (review this matcher)
```

> [!TIP]
> Check these evidence comments before you commit your config. Matchers with 20+ matches are solid. Zero-match matchers may need a different label name — check your repo's actual labels with `gh label list`.

You can also preview what preflight would generate without writing the file (dry run):

```bash
gh velocity config preflight
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
gh velocity config show -r json
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

This is equivalent to what preflight generates for repos with standard labels.

## What each section does

Each config section represents a decision about how gh-velocity should measure your project. Most have sensible defaults — you only need to configure what matters for your workflow. See [Configuration Reference]({{< relref "/reference/config" >}}) for the full schema.

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

- `label:<name>` — exact label match
- `type:<name>` — GitHub Issue Type
- `title:/<regex>/i` — title regex, case-insensitive
- `field:<Name>/<Value>` — GitHub Projects v2 SingleSelect field value (e.g., `field:Size/M`). See [Labels as Lifecycle Signal]({{< relref "/concepts/labels-vs-board" >}})

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

> [!NOTE]
> The report's bug ratio counts issues classified as "bug". If you name your bug category differently (e.g., "defect"), use `bug` as the name in the config.

### `quality.hotfix_window_hours`

A release is flagged as a hotfix if published within this many hours of the previous release. Default: 72 (3 days). Maximum: 8760 (1 year).

### `cycle_time`

Which strategy to use: `issue` (label-based) or `pr` (PR creation to merge). See [How It Works]({{< relref "/getting-started/how-it-works" >}}) for guidance on choosing, and [Cycle Time Setup]({{< relref "/guides/cycle-time-setup" >}}) for step-by-step configuration.

```yaml
cycle_time:
  strategy: pr
```

### `project`

GitHub Projects v2 settings. Used for velocity iteration/effort reads:

```yaml
project:
  url: "https://github.com/users/yourname/projects/1"
  status_field: "Status"
```

### `lifecycle`

Maps labels to workflow stages. Labels are the sole source of truth for lifecycle signals -- they provide immutable timestamps for cycle time measurement. See [Labels as Lifecycle Signal]({{< relref "/concepts/labels-vs-board" >}}) for details:

```yaml
lifecycle:
  in-progress:
    match: ["label:in-progress"]
  in-review:
    match: ["label:in-review"]
  done:
    query: "is:closed"
    match: ["label:done"]
```

### `velocity`

How to measure effort per iteration. Three effort strategies: `count` (just count items), `attribute` (labels like `size:M` with point values), or `numeric` (a project board number field). See [Velocity Setup]({{< relref "/guides/velocity-setup" >}}) for a walkthrough.

```yaml
velocity:
  effort:
    strategy: attribute
    attribute:
      - query: "label:size/S"
        value: 1
      - query: "label:size/M"
        value: 3
      - query: "label:size/L"
        value: 5
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
| Reviews | Yes | Yes |
| `--post` to issues/PRs/Discussions | Yes | Yes |
| Velocity (project-field iteration or numeric effort) | No | Yes |
| WIP command | No | Yes |

**If your config has no `project:` section**, you do not need `GH_VELOCITY_TOKEN`.

**If your config has a `project:` section**, commands that need the board will warn and skip that data unless `GH_VELOCITY_TOKEN` is set. The rest of the report still works.

For CI token setup details, see [CI Setup]({{< relref "/getting-started/ci-setup" >}}).

## Next steps

- [Configuration Reference]({{< relref "/reference/config" >}}) -- full schema with all options and defaults
- [CI Setup]({{< relref "/getting-started/ci-setup" >}}) -- automate reports with GitHub Actions
- [Velocity Setup]({{< relref "/guides/velocity-setup" >}}) -- configure effort-weighted sprint metrics
- [Cycle Time Setup]({{< relref "/guides/cycle-time-setup" >}}) -- configure cycle time measurement
- [How It Works]({{< relref "/getting-started/how-it-works" >}}) -- understand the metrics and strategies
