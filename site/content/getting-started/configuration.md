---
title: "Configuration"
weight: 3
---

# Configuration

All metric commands require a `.gh-velocity.yml` file. Run `gh velocity config preflight --write` to generate one. This page covers first-time setup; see [Configuration Reference]({{< relref "/reference/config" >}}) for the full schema.

## Generate a config with preflight

Let preflight inspect your repo and generate a tailored config:

```bash
cd your-repo
gh velocity config preflight --write
```

Preflight examines labels, project boards, and recent activity, then generates a `.gh-velocity.yml` with **matchers** (patterns like `label:bug` that classify issues), lifecycle labels, and project board settings.

The generated config includes **match evidence** -- comments showing how many issues each matcher would catch:

```yaml
# Match evidence (last 30 days of issues + PRs):
#   bug / label:bug — 33 matches, e.g. #12893 error parsing "input[title]"...
#   feature / label:enhancement — 37 matches, e.g. #12862 Remove a book...
#   chore / label:tech-debt — 0 matches (review this matcher)
```

> [!TIP]
> Check these evidence comments before you commit your config. Matchers with 20+ matches are solid. Zero-match matchers may need a different label name -- check your repo's actual labels with `gh label list`.

To dry-run without writing the file:

```bash
gh velocity config preflight
```

## Discover project board settings

If you use a GitHub Projects v2 board, `config discover` finds your project URL, status field name, and available status values:

```bash
gh velocity config discover --repo owner/repo
```

Use this output to fill in the `project` and `lifecycle` sections of your config.

## Validate your config

After writing or editing your config:

```bash
gh velocity config validate
```

This checks field types, valid ranges, and formats. It makes no API calls.

To see the resolved configuration with all defaults applied:

```bash
gh velocity config show
gh velocity config show -r json
```

### Common validation errors

**`quality.categories[0].match: unknown matcher type "bug"`** -- Matchers need a prefix. Use `label:bug`, not bare `bug`.

**`velocity.effort.attribute[0].value: must be positive`** -- Effort values must be greater than zero. Change `value: 0` to `value: 1` or remove the entry.

**`lifecycle.in-progress.match: empty match list`** -- The `in-progress` stage is declared but has no matchers. Add at least one, e.g., `["label:in-progress"]`.

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

This is what preflight generates for repos with standard labels.

## What each section does

Each config section controls how gh-velocity measures your project. Most have sensible defaults -- only configure what matters for your workflow.

### `workflow`

How your team works. `pr` (default) means PRs close issues. `local` means direct commits to main (solo projects, scripts).

### `scope`

Which issues and PRs to analyze, using GitHub search query syntax:

```yaml
scope:
  query: "repo:myorg/myrepo"
```

### `quality.categories`

Ordered list of classification categories. First match wins; unmatched issues become "other." Matcher types:

- `label:<name>` -- exact label match
- `type:<name>` -- GitHub Issue Type
- `title:/<regex>/i` -- title regex, case-insensitive
- `field:<Name>/<Value>` -- GitHub Projects v2 SingleSelect field value (e.g., `field:Size/M`)

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

Which strategy to use: `issue` (label-based) or `pr` (PR creation to merge). See [Cycle Time Setup]({{< relref "/guides/cycle-time-setup" >}}) for step-by-step configuration.

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

Maps labels to workflow stages. Labels are the sole lifecycle signal -- they provide immutable timestamps for cycle time measurement:

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

How to measure effort per iteration. Three effort strategies: `count` (count items), `attribute` (labels like `size:M` with point values), or `numeric` (a project board number field). See [Velocity Setup]({{< relref "/guides/velocity-setup" >}}) for a walkthrough.

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

The default `gh auth login` token works for all local usage. In CI, different tokens cover different capabilities:

| Capability | Default `GITHUB_TOKEN` | + `GH_VELOCITY_TOKEN` (PAT with `project` scope) |
| --- | --- | --- |
| Lead time, throughput, release quality | Yes | Yes |
| Reviews | Yes | Yes |
| `--post` to issues/PRs/Discussions | Yes | Yes |
| Velocity (project-field iteration or numeric effort) | No | Yes |
| WIP command | No | Yes |

**No `project:` section** -- `GH_VELOCITY_TOKEN` is not needed.

**With a `project:` section** -- commands that need the board warn and skip unless `GH_VELOCITY_TOKEN` is set. The rest of the report still works.

For CI token setup, see [CI Setup]({{< relref "/getting-started/ci-setup" >}}).

## Next steps

- [Configuration Reference]({{< relref "/reference/config" >}}) -- full schema with all options and defaults
- [CI Setup]({{< relref "/getting-started/ci-setup" >}}) -- automate reports with GitHub Actions
- [Velocity Setup]({{< relref "/guides/velocity-setup" >}}) -- configure effort-weighted sprint metrics
- [Cycle Time Setup]({{< relref "/guides/cycle-time-setup" >}}) -- configure cycle time measurement
