---
title: "Configuration"
weight: 3
---

# Configuration Reference

Complete reference for `.gh-velocity.yml` fields, types, defaults, and validation rules.

All metric commands require a `.gh-velocity.yml` file. Run `gh velocity config preflight --write` to generate one, or create one manually in your repository root. Use `--config` to point to an alternate path. Every field is optional -- the tool uses sensible defaults for anything omitted.

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
gh velocity config show -r json      # JSON format
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

- `"pr"` -- PRs close issues (most teams). Enables PR strategy linking and cycle time.
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

A GitHub search query fragment appended to all Search API calls. Typically `"repo:owner/name"` when running against a remote repo. Accepts any valid GitHub search qualifier.

```yaml
scope:
  query: "repo:cli/cli"
```

Add label filters, milestone filters, or any search qualifier:

```yaml
scope:
  query: 'repo:myorg/myrepo label:"team:platform"'
```

The `--scope` CLI flag adds additional qualifiers at runtime (AND logic with the config scope).

---

## `project`

Identifies a GitHub Projects v2 board. Required when `velocity.iteration.strategy` is `"project-field"`, when `velocity.effort.strategy` is `"numeric"`, or when using `field:` matchers.

### `project.url`

| Property | Value |
|---|---|
| **Type** | string |
| **Default** | `""` (none) |
| **Format** | `https://github.com/users/{user}/projects/{N}` or `https://github.com/orgs/{org}/projects/{N}` |

The project board URL. Copy it from the browser address bar when viewing your project board.

```yaml
project:
  url: "https://github.com/users/dvhthomas/projects/1"
```

### `project.status_field`

| Property | Value |
|---|---|
| **Type** | string |
| **Default** | `""` (none) |

The visible name of the status (single-select) field on your project board. Usually `"Status"`. Used by the velocity `numeric` effort strategy and `field:` matchers.

Run `gh velocity config discover` to find available fields and options on your board.

```yaml
project:
  url: "https://github.com/users/dvhthomas/projects/1"
  status_field: "Status"
```

---

## `lifecycle`

Maps workflow stages to GitHub search qualifiers and label matchers. Commands use these to filter items by lifecycle stage. Labels are the sole source of truth for lifecycle signals (see [Labels as Lifecycle Signal]({{< relref "/concepts/labels-vs-board" >}})).

Each stage has two optional fields:

| Sub-field | Type | Description |
|---|---|---|
| `query` | string | Appended to search API calls (e.g., `"is:closed"`) |
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
    match: ["label:backlog"]
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
    match: ["label:in-progress", "label:wip"]
```

Uses [matcher syntax](#matcher-syntax). When a matching label is applied, its immutable `createdAt` timestamp becomes the cycle time start signal.

### `lifecycle.in-review`

Items in code review.

| Property | Value |
|---|---|
| **Default query** | `"is:open"` |

```yaml
lifecycle:
  in-review:
    query: "is:open"
    match: ["label:in-review"]
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
    match: ["label:done"]
```

### `lifecycle.released`

Items included in a release. No default query -- release detection is tag-based.

```yaml
lifecycle:
  released:
    query: ""
```

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

Unmatched issues are classified as `"other"`. When more than 50% of issues are "other," the tool warns about low classification coverage.

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

- `"issue"` -- Starts when an in-progress label is applied (`lifecycle.in-progress.match`), ends when the issue is closed.
- `"pr"` -- Starts when the closing PR is created, ends when it is merged.

```yaml
cycle_time:
  strategy: pr
```

See [Cycle Time]({{< relref "metrics/cycle-time" >}}) for the metric definition.

---

## `velocity`

Controls how velocity (effort per iteration) is measured. See [Velocity]({{< relref "metrics/velocity" >}}) for the metric definition.

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

Name of a Number field on the project board holding the effort value (e.g., `"Story Points"`).

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

Must be set to use the velocity command. Run `gh velocity config preflight` for a suggested value.

#### `velocity.iteration.project_field`

| Property | Value |
|---|---|
| **Type** | string |
| **Required** | when strategy is `"project-field"` |

Name of an Iteration field on the project board (e.g., `"Sprint"`). Requires `project.url`.

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

Number of past iterations to fetch. Higher values increase API consumption (one search query per iteration for the fixed strategy).

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

Commonly used to filter bot accounts. Applied as `-author:` qualifiers in search queries.

```yaml
exclude_users:
  - "dependabot[bot]"
  - "renovate[bot]"
```

---

## `discussions`

Configuration for posting reports to GitHub Discussions via the `--post` flag.

### `discussions.category`

| Property | Value |
|---|---|
| **Type** | string |
| **Format** | `owner/repo/category` |
| **Default** | `""` (none) |

The full discussion target: repository owner, repository name, and discussion category name, separated by `/`. The target repository can differ from the data source -- for example, posting reports from multiple repos to a shared `engops` repository.

If the category name contains a `/`, quote it:

```yaml
discussions:
  category: myorg/myrepo/General
```

```yaml
# Category name with a slash
discussions:
  category: myorg/myrepo/"Show / Tell"
```

The category must already exist in the target repository's Discussions settings. Invalid format (missing owner, repo, or category) is rejected at config validation time.

### `discussions.title`

| Property | Value |
|---|---|
| **Type** | string |
| **Default** | `""` (uses built-in format: `gh-velocity {command}: {owner/repo} ({date})`) |

Template for Discussion titles. The rendered title is the **deduplication key** -- if a Discussion with the same title already exists in the category, its body is updated instead of creating a new Discussion. Human edits and comments on the Discussion are preserved.

```yaml
discussions:
  category: General
  title: "Velocity Update {{date}}"
```

#### Title template variables

Placeholders use `{{variable}}` syntax. Only the variables listed below are supported -- unknown variables are rejected at config validation time.

| Variable | Description | Example output |
|---|---|---|
| `{{date}}` | Today's UTC date in `YYYY-MM-DD` format | `2026-03-20` |
| `{{date:FORMAT}}` | Today's UTC date in a custom [Go time layout](https://pkg.go.dev/time#pkg-constants) | `{{date:Jan 2}}` → `Mar 20` |
| `{{repo}}` | Repository in `owner/repo` format | `myorg/myrepo` |
| `{{owner}}` | Repository owner | `myorg` |
| `{{command}}` | The command being run | `report`, `lead-time` |

#### Title template examples

```yaml
# Daily report -- creates a new Discussion each day, updates on re-run
title: "Velocity Update {{date}}"

# Weekly report with readable date
title: "Weekly Velocity — {{date:Jan 2, 2006}}"

# Per-command Discussions (similar to the default)
title: "{{command}}: {{repo}} ({{date}})"

# Single rolling Discussion per repo (no date = always updates the same one)
title: "Velocity Dashboard — {{repo}}"
```

#### Deduplication behavior

- **Same rendered title** → updates the existing Discussion body (partial update via markers, preserving content outside markers)
- **Different rendered title** → creates a new Discussion
- **`--new-post`** → always creates a new Discussion regardless of title
- **Multiple commands, same title** → each command gets its own section in the Discussion body. Running `report --post` and `lead-time --post` with the same title template updates separate sections in the same Discussion.
- **Discussions are never deleted.** Human comments and edits outside the tool's marker sections are preserved across updates.

#### Default behavior (no title configured)

When `discussions.title` is not set, the title defaults to `gh-velocity {command}: {owner/repo} ({date})`. This means:
- Each command gets its own Discussion (the command name is in the title)
- A new Discussion is created each day (the date is in the title)
- Re-running the same command on the same day updates the existing Discussion

---

## `api_throttle_seconds`

| Property | Value |
|---|---|
| **Type** | int (nullable) |
| **Default** | `0` (no throttle). `preflight` recommends `2` for repos with many iterations. |
| **Recommended** | `2` |

Minimum delay in seconds between GitHub Search API calls. Prevents triggering GitHub's secondary (abuse) rate limits, which have undocumented thresholds and can cause multi-minute lockouts.

`preflight` recommends `api_throttle_seconds: 2` when generating a config. Set to `0` to disable (not recommended for repos with many iterations).

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

The `field:` matcher requires `project.url`. It reads the specified SingleSelect field from the project board and matches items whose field value equals `<Value>`.

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
  in-progress:
    match: ["label:in-progress", "label:wip"]
  in-review:
    match: ["label:in-review"]
  done:
    query: "is:closed"
    match: ["label:done"]

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
  category: myorg/myrepo/General
  title: "Velocity Update {{date}}"

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
| `discussions.category` | `""` (format: `owner/repo/category`) |
| `discussions.title` | `""` (built-in: `gh-velocity {command}: {owner/repo} ({date})`) |
| `api_throttle_seconds` | not set (no throttle) |

## See also

- [Configuration (Getting Started)]({{< relref "/getting-started/configuration" >}}) -- first-time setup with preflight, discover, and validate
- [Setting Up Flow Velocity]({{< relref "/guides/velocity-setup" >}}) -- effort and iteration strategy guides
- [Cycle Time Setup]({{< relref "/guides/cycle-time-setup" >}}) -- choosing a cycle time strategy
- [Examples]({{< relref "/examples" >}}) -- annotated real-world configs
