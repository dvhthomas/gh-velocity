---
title: "Configuration"
weight: 3
---

# Configuration Reference

Complete reference for `.gh-velocity.yml` fields, types, defaults, and validation rules.

All metric commands require a `.gh-velocity.yml` file. Run `gh velocity config preflight --write` to generate one, or create one manually in your repository root. Use `--config` to point to an alternate path. Every field within the config is optional — the tool uses sensible defaults for anything you omit.

To generate a config tailored to your repository:

```bash
gh velocity config preflight             # preview to stdout (from inside your repo)
gh velocity config preflight --write     # save to .gh-velocity.yml
gh velocity config preflight -R cli/cli  # analyze a remote repo
```

To validate an existing config:

```bash
gh velocity config validate          # check syntax and values
gh velocity config show              # show resolved config with defaults
gh velocity config show -f json      # JSON format
```

Maximum config file size: 64 KB. Unknown top-level keys produce warnings but do not cause errors.

---

## `workflow`

How your team delivers code.

| Property | Value |
|---|---|
| **Type** | string |
| **Default** | `"pr"` |
| **Valid values** | `"pr"`, `"local"` |

- `"pr"` -- PRs close issues (most teams). Enables PR-based linking strategies and cycle time.
- `"local"` -- Direct commits to main (solo projects, scripts). Uses commit-ref strategy.

```yaml
workflow: pr
```

---

## `scope`

Filters which issues and PRs are analyzed. Uses GitHub search query syntax.

### `scope.query`

| Property | Value |
|---|---|
| **Type** | string |
| **Default** | `""` (empty -- uses repo context) |

A GitHub search query fragment appended to all search API calls. Typically `"repo:owner/name"` when running against a remote repo. Can include any valid GitHub search qualifier.

```yaml
scope:
  query: "repo:cli/cli"
```

You can add label filters, milestone filters, or any search qualifier:

```yaml
scope:
  query: 'repo:myorg/myrepo label:"team:platform"'
```

The `--scope` CLI flag adds additional qualifiers at runtime (AND logic with the config scope).

---

## `project`

Identifies a GitHub Projects v2 board. Required when lifecycle stages use `project_status`, when `velocity.iteration.strategy` is `"project-field"`, or when `velocity.effort.strategy` is `"numeric"`.

### `project.url`

| Property | Value |
|---|---|
| **Type** | string |
| **Default** | `""` (none) |
| **Format** | `https://github.com/users/{user}/projects/{N}` or `https://github.com/orgs/{org}/projects/{N}` |

The project board URL. Find it by navigating to your project board in GitHub and copying the URL from the browser.

```yaml
project:
  url: "https://github.com/users/dvhthomas/projects/1"
```

### `project.status_field`

| Property | Value |
|---|---|
| **Type** | string |
| **Default** | `""` (none) |

The visible name of the status (single-select) field on your project board. Usually `"Status"`. Required when any lifecycle stage uses `project_status`.

Run `gh velocity config discover` to find available fields and options on your board.

```yaml
project:
  url: "https://github.com/users/dvhthomas/projects/1"
  status_field: "Status"
```

---

## `lifecycle`

Maps workflow stages to GitHub search qualifiers and/or project board statuses. Commands use these to filter items by lifecycle stage. See [Labels vs. Project Board]({{< relref "/concepts/labels-vs-board" >}}) for guidance on choosing between labels and board status.

Each stage has three optional fields:

| Sub-field | Type | Description |
|---|---|---|
| `query` | string | Appended to search API calls (e.g., `"is:closed"`) |
| `project_status` | list of strings | Project board column names for GraphQL filtering |
| `match` | list of strings | Matcher patterns for client-side lifecycle grouping |

### `lifecycle.backlog`

Items in the backlog (not yet started).

| Property | Value |
|---|---|
| **Default query** | `"is:open"` |

```yaml
lifecycle:
  backlog:
    query: "is:open"
    project_status: ["Backlog", "Triage"]
```

### `lifecycle.in-progress`

Items actively being worked on. The `match` field is used by the issue cycle time strategy to detect when work started.

| Property | Value |
|---|---|
| **Default query** | `"is:open"` |

```yaml
lifecycle:
  in-progress:
    query: "is:open"
    project_status: ["In progress"]
    match: ["label:in-progress", "label:wip"]
```

The `match` patterns use [matcher syntax](#matcher-syntax). When a label matching one of these patterns is applied to an issue, its immutable `createdAt` timestamp becomes the cycle time start signal.

### `lifecycle.in-review`

Items in code review.

| Property | Value |
|---|---|
| **Default query** | `"is:open"` |

```yaml
lifecycle:
  in-review:
    query: "is:open"
    project_status: ["In review"]
```

### `lifecycle.done`

Completed items.

| Property | Value |
|---|---|
| **Default query** | `"is:closed"` |

```yaml
lifecycle:
  done:
    query: "is:closed"
    project_status: ["Done"]
```

### `lifecycle.released`

Items included in a release. No default query -- release detection is tag-based.

```yaml
lifecycle:
  released:
    project_status: ["Released"]
```

### Validation rules

- If any lifecycle stage uses `project_status`, both `project.url` and `project.status_field` are required.

---

## `quality`

Controls issue classification and hotfix detection for release quality reports.

### `quality.categories`

| Property | Value |
|---|---|
| **Type** | list of category objects |
| **Default** | `[{name: "bug", matchers: ["label:bug"]}, {name: "feature", matchers: ["label:enhancement"]}]` |

Ordered list of classification categories. Each category has:

| Sub-field | Type | Description |
|---|---|---|
| `name` | string | Category name. Use `"bug"` for the bug ratio numerator. |
| `match` | list of strings | Matcher patterns. First match wins across all categories. |

```yaml
quality:
  categories:
    - name: bug
      match:
        - "label:bug"
        - "label:defect"
        - "type:Bug"
    - name: feature
      match:
        - "label:enhancement"
        - "type:Feature"
    - name: chore
      match:
        - "label:tech-debt"
        - "title:/^chore[\\(: ]/i"
    - name: docs
      match:
        - "label:documentation"
```

Issues that do not match any category are classified as `"other"`. When more than 50% of issues are "other," the tool warns about low classification coverage.

### `quality.hotfix_window_hours`

| Property | Value |
|---|---|
| **Type** | number |
| **Default** | `72` |
| **Range** | > 0, <= 8760 (1 year) |

A release published within this many hours of the previous release is flagged as a hotfix.

```yaml
quality:
  hotfix_window_hours: 48
```

---

## `cycle_time`

Controls which strategy measures cycle time.

### `cycle_time.strategy`

| Property | Value |
|---|---|
| **Type** | string |
| **Default** | `"issue"` |
| **Valid values** | `"issue"`, `"pr"` |

- `"issue"` -- Starts when an in-progress label is applied (or project board status changes as fallback), ends when the issue is closed.
- `"pr"` -- Starts when the closing PR is created, ends when it is merged.

The value `"project-board"` is deprecated and silently treated as `"issue"`.

```yaml
cycle_time:
  strategy: pr
```

See [Cycle Time]({{< relref "metrics/cycle-time" >}}) for the metric definition, and [Cycle Time Setup]({{< relref "/guides/cycle-time-setup" >}}) for a step-by-step configuration guide.

---

## `velocity`

Controls how velocity (effort per iteration) is measured. See [Velocity]({{< relref "metrics/velocity" >}}) for the metric definition, and [Setting Up Velocity]({{< relref "/guides/velocity-setup" >}}) for a step-by-step configuration guide.

### `velocity.unit`

| Property | Value |
|---|---|
| **Type** | string |
| **Default** | `"issues"` |
| **Valid values** | `"issues"`, `"prs"` |

Which work item type to measure. Issues use `closed` + `reason:completed`. PRs use `merged`.

### `velocity.effort`

Controls how effort is assigned to work items.

#### `velocity.effort.strategy`

| Property | Value |
|---|---|
| **Type** | string |
| **Default** | `"count"` |
| **Valid values** | `"count"`, `"attribute"`, `"numeric"` |

- `"count"` -- every item = 1
- `"attribute"` -- map matchers to effort values (requires `velocity.effort.attribute`)
- `"numeric"` -- read from a project board Number field (requires `velocity.effort.numeric.project_field` and `project.url`)

#### `velocity.effort.attribute`

| Property | Value |
|---|---|
| **Type** | list of matcher objects |
| **Required** | when strategy is `"attribute"` |

Each entry has:

| Sub-field | Type | Description |
|---|---|---|
| `query` | string | Matcher pattern (see [matcher syntax](#matcher-syntax)) |
| `value` | number | Effort value (must be >= 0) |

```yaml
velocity:
  effort:
    strategy: attribute
    attribute:
      - query: "label:size/L"
        value: 5
      - query: "label:size/M"
        value: 3
      - query: "label:size/S"
        value: 1
```

#### `velocity.effort.numeric.project_field`

| Property | Value |
|---|---|
| **Type** | string |
| **Required** | when strategy is `"numeric"` |

The name of a Number field on the project board that holds the effort value (e.g., `"Story Points"`).

```yaml
velocity:
  effort:
    strategy: numeric
    numeric:
      project_field: "Story Points"
```

### `velocity.iteration`

Controls how iteration boundaries are determined.

#### `velocity.iteration.strategy`

| Property | Value |
|---|---|
| **Type** | string |
| **Default** | `""` (not configured) |
| **Valid values** | `"project-field"`, `"fixed"` |

Must be explicitly set to use the velocity command. Run `gh velocity config preflight` to get a suggested value.

#### `velocity.iteration.project_field`

| Property | Value |
|---|---|
| **Type** | string |
| **Required** | when strategy is `"project-field"` |

The name of an Iteration field on the project board (e.g., `"Sprint"`). Requires `project.url`.

#### `velocity.iteration.fixed`

Fixed-length calendar iterations. Required when strategy is `"fixed"`.

| Sub-field | Type | Format | Description |
|---|---|---|---|
| `length` | string | `"Nd"` or `"Nw"` | Iteration length (e.g., `"14d"`, `"2w"`) |
| `anchor` | string | `YYYY-MM-DD` | A known iteration start date |

```yaml
velocity:
  iteration:
    strategy: fixed
    fixed:
      length: "14d"
      anchor: "2026-01-06"
```

#### `velocity.iteration.count`

| Property | Value |
|---|---|
| **Type** | int |
| **Default** | `6` |
| **Range** | > 0 |

Number of past iterations to fetch and display. Higher values increase API consumption (one search query per iteration for fixed strategy).

---

## `commit_ref`

Controls how the commit-ref linking strategy scans commit messages for issue references.

### `commit_ref.patterns`

| Property | Value |
|---|---|
| **Type** | list of strings |
| **Default** | `[]` (empty -- not used unless configured) |
| **Valid values** | `"closes"`, `"refs"` |

- `"closes"` -- matches closing keywords: `fixes #N`, `closes #N`, `resolves #N` and variations.
- `"refs"` -- also matches bare `#N` references. Can produce false positives (e.g., "step #1").

```yaml
commit_ref:
  patterns: ["closes"]
```

---

## `exclude_users`

Excludes issues and PRs authored by specified users from all metrics.

| Property | Value |
|---|---|
| **Type** | list of strings |
| **Default** | `[]` (none) |

Commonly used to filter out bot accounts. Values are applied as `-author:` qualifiers in search queries.

```yaml
exclude_users:
  - "dependabot[bot]"
  - "renovate[bot]"
```

---

## `discussions`

Configuration for posting reports to GitHub Discussions.

### `discussions.category`

| Property | Value |
|---|---|
| **Type** | string |
| **Default** | `""` (none) |

The Discussion category name to post to (e.g., `"General"`, `"Reports"`). Must be a non-empty string if set. The category must already exist in the repository's Discussions settings.

```yaml
discussions:
  category: General
```

Used by the `--post` flag on bulk commands to create or update Discussion posts.

---

## `api_throttle_seconds`

| Property | Value |
|---|---|
| **Type** | int (nullable) |
| **Default** | `0` (no throttle, unless set by preflight) |
| **Recommended** | `2` |

Minimum delay in seconds between GitHub Search API calls. Prevents triggering GitHub's secondary (abuse) rate limits, which have undocumented thresholds and can result in multi-minute lockouts.

The `preflight` command recommends `api_throttle_seconds: 2` when generating a config. Set to `0` to disable throttling (not recommended for repos with many iterations).

```yaml
api_throttle_seconds: 2
```

---

## Matcher syntax {#matcher-syntax}

Several configuration fields (`quality.categories[].match`, `lifecycle.in-progress.match`, `velocity.effort.attribute[].query`) use a shared matcher syntax:

| Pattern | Description | Example |
|---------|-------------|---------|
| `label:<name>` | Exact match on issue/PR label name | `label:bug` |
| `type:<name>` | Match on GitHub Issue Type | `type:Bug` |
| `title:/<regex>/i` | Case-insensitive regex match on title | `title:/^fix[\(: ]/i` |
| `field:<Name>/<Value>` | Match a SingleSelect field value on a Projects v2 board | `field:Size/M` |

The `field:` matcher requires `project.url` to be configured. It reads the specified SingleSelect field from the project board and matches items whose field value equals `<Value>`. This is especially useful for effort classification — see [Labels vs. Board]({{< relref "/concepts/labels-vs-board" >}}) for examples.

Matchers are evaluated in config order. For classification (`quality.categories`), the first matching category wins across all categories. For effort (`velocity.effort.attribute`), the first matching rule determines the effort value.

---

## Full example

```yaml
workflow: pr

scope:
  query: "repo:myorg/myrepo"

project:
  url: "https://github.com/orgs/myorg/projects/5"
  status_field: "Status"

lifecycle:
  backlog:
    project_status: ["Backlog", "Triage"]
  in-progress:
    project_status: ["In progress"]
    match: ["label:in-progress", "label:wip"]
  in-review:
    project_status: ["In review"]
  done:
    query: "is:closed"
    project_status: ["Done"]

quality:
  categories:
    - name: bug
      match:
        - "label:bug"
        - "type:Bug"
    - name: feature
      match:
        - "label:enhancement"
        - "type:Feature"
    - name: chore
      match:
        - "label:tech-debt"
    - name: docs
      match:
        - "label:documentation"
  hotfix_window_hours: 48

cycle_time:
  strategy: issue

velocity:
  unit: issues
  effort:
    strategy: attribute
    attribute:
      - query: "label:size/L"
        value: 5
      - query: "label:size/M"
        value: 3
      - query: "label:size/S"
        value: 1
  iteration:
    strategy: project-field
    project_field: "Sprint"
    count: 6

commit_ref:
  patterns: ["closes"]

exclude_users:
  - "dependabot[bot]"
  - "renovate[bot]"

discussions:
  category: General

api_throttle_seconds: 2
```

---

## Defaults summary

| Field | Default |
|---|---|
| `workflow` | `"pr"` |
| `scope.query` | `""` |
| `project.url` | `""` |
| `project.status_field` | `""` |
| `lifecycle.backlog.query` | `"is:open"` |
| `lifecycle.in-progress.query` | `"is:open"` |
| `lifecycle.in-review.query` | `"is:open"` |
| `lifecycle.done.query` | `"is:closed"` |
| `quality.categories` | bug (label:bug) + feature (label:enhancement) |
| `quality.hotfix_window_hours` | `72` |
| `cycle_time.strategy` | `"issue"` |
| `velocity.unit` | `"issues"` |
| `velocity.effort.strategy` | `"count"` |
| `velocity.iteration.count` | `6` |
| `commit_ref.patterns` | `[]` |
| `exclude_users` | `[]` |
| `discussions.category` | `""` |
| `api_throttle_seconds` | not set (no throttle) |

## See also

- [Configuration (Getting Started)]({{< relref "/getting-started/configuration" >}}) -- first-time setup guide with preflight, discover, and validate
- [Setting Up Velocity]({{< relref "/guides/velocity-setup" >}}) -- effort strategies, iteration strategies, and validation
- [Cycle Time Setup]({{< relref "/guides/cycle-time-setup" >}}) -- choosing and configuring a cycle time strategy
- [Labels vs. Project Board]({{< relref "/concepts/labels-vs-board" >}}) -- why labels are preferred for lifecycle tracking
- [CI Setup]({{< relref "/getting-started/ci-setup" >}}) -- token setup for CI environments
- [Examples]({{< relref "/examples" >}}) -- annotated real-world configs for popular repositories
