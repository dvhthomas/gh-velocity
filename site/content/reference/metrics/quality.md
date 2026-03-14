---
title: "Quality"
weight: 5
---

# Quality Metrics

Quality metrics analyze the composition and health of a release. They answer: "What did this release contain, and how healthy was the release process?"

## Per-release defect rate

Defect rate is the proportion of issues in a release classified as bugs.

```
defect_rate = bug_count / total_issues
```

"Bug" classification depends on your `quality.categories` configuration. The tool looks for a category named `"bug"` in the classification results. If you name your bug category differently, use `"bug"` as the `name` field in your config.

### Example

A release with 20 issues, 4 of which are classified as "bug":

```json
{
  "composition": {
    "total_issues": 20,
    "category_counts": { "bug": 4, "feature": 12, "other": 4 },
    "category_ratios": { "bug": 0.2, "feature": 0.6, "other": 0.2 }
  }
}
```

Defect rate: 20%.

## Category composition

Every issue in a release is classified into exactly one category using the configured matchers. The first matching category wins; unmatched issues are classified as "other."

### Classification matchers

Three matcher types are supported:

| Matcher | Syntax | Example |
|---------|--------|---------|
| Label | `label:<name>` | `label:bug` |
| Issue Type | `type:<name>` | `type:Bug` |
| Title regex | `title:/<regex>/i` | `title:/^fix[\(: ]/i` |

Matchers are evaluated in config order. First match wins.

### Configuration

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
    - name: docs
      match:
        - "label:documentation"
```

### Low coverage warning

When more than 50% of issues in a release are classified as "other" (unmatched), the tool emits a warning:

```
Low classification coverage: 12/20 issues are unclassified
```

This indicates your category matchers do not cover enough of your issue labeling. Add more matchers or improve your team's labeling practice.

## Hotfix detection

A release is flagged as a **hotfix** when it was published within a configurable time window of the previous release.

```
is_hotfix = (current_release.created_at - previous_release.created_at) <= hotfix_window
```

### Configuration

```yaml
quality:
  hotfix_window_hours: 72   # default: 72 (3 days)
```

| Field | Type | Default | Range |
|---|---|---|---|
| `quality.hotfix_window_hours` | number | `72` | > 0, <= 8760 (1 year) |

A release published 48 hours after the previous release with a 72-hour window is flagged as a hotfix. This helps identify emergency patches versus planned releases.

### Example

```json
{
  "tag": "v1.2.1",
  "previous_tag": "v1.2.0",
  "cadence_seconds": 172800,
  "is_hotfix": true
}
```

## Release cadence

When a previous release exists, the tool computes the time between the two releases:

```
cadence = current_release.created_at - previous_release.created_at
```

This is reported as `cadence_seconds` in JSON and as a human-readable duration in pretty/markdown formats. Tracking cadence over multiple releases reveals whether your team is shipping faster or slower.

## Release lag

Release lag measures the delay between an issue being closed and the release that includes it being published:

```
release_lag = release.created_at - issue.closed_at
```

A long release lag means finished work is sitting unreleased. This is common in teams with infrequent release cadences or manual release processes.

Release lag is computed per-issue and aggregated with the same statistics as lead time (mean, median, P90, P95, outlier detection).

## Outlier detection

Both lead time and cycle time within a release use IQR-based outlier detection:

1. Compute Q1 (25th percentile) and Q3 (75th percentile)
2. IQR = Q3 - Q1
3. Outlier cutoff = Q3 + 1.5 * IQR
4. Any issue with a duration exceeding the cutoff is flagged as an outlier

Outlier detection requires at least 4 data points to compute meaningful quartiles.

Individual issues are flagged with `lead_time_outlier: true` and/or `cycle_time_outlier: true` in the output, making it easy to identify items that took disproportionately long.

## Configuration reference

| Config field | Type | Default | Description |
|---|---|---|---|
| `quality.categories` | list | bug + feature | Ordered classification categories |
| `quality.categories[].name` | string | -- | Category name (use `"bug"` for defect rate) |
| `quality.categories[].match` | list | -- | Matcher patterns for this category |
| `quality.hotfix_window_hours` | number | `72` | Releases within this window of the previous release are flagged as hotfixes |

## Example output

### Pretty format

```
Release v1.2.0 (2026-03-01)
  Previous: v1.1.0 (14 days ago)
  Hotfix: No

  Composition (8 issues)
    bug:     2 (25.0%)
    feature: 4 (50.0%)
    chore:   1 (12.5%)
    other:   1 (12.5%)

  Aggregates
    Metric       Count  Median    Mean      P90       P95
    Lead time    8      5d 2h     12d 4h    28d       30d
    Cycle time   6      1d 18h    2d 6h     4d 12h    5d
    Release lag  8      3d 1h     4d 8h     8d        9d
```

### JSON format

```json
{
  "tag": "v1.2.0",
  "previous_tag": "v1.1.0",
  "date": "2026-03-01T10:00:00Z",
  "cadence_seconds": 1209600,
  "is_hotfix": false,
  "total_issues": 8,
  "category_names": ["bug", "feature", "chore", "other"],
  "category_counts": { "bug": 2, "feature": 4, "chore": 1, "other": 1 },
  "category_ratios": { "bug": 0.25, "feature": 0.5, "chore": 0.125, "other": 0.125 },
  "aggregates": {
    "lead_time": { "count": 8, "mean_seconds": 1060800, "median_seconds": 446400 },
    "cycle_time": { "count": 6, "mean_seconds": 194400, "median_seconds": 151200 },
    "release_lag": { "count": 8, "mean_seconds": 374400, "median_seconds": 263700 }
  }
}
```

## Commands

- `gh velocity quality release <tag>` -- full release quality report
- `gh velocity quality release <tag> --discover` -- show which linking strategies found each issue
- `gh velocity quality release <tag> --since <prev-tag>` -- specify previous tag explicitly
