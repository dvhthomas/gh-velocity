---
title: "Recipes"
weight: 6
---

# How-To Recipes

Practical patterns for common tasks with gh-velocity. For understanding what the output means, see [Interpreting Results]({{< relref "/guides/interpreting-results" >}}). For CI automation, see [CI Setup]({{< relref "/getting-started/ci-setup" >}}).

## Compare two releases

```bash
gh velocity quality release v2.0.0 --format json > v2.json
gh velocity quality release v1.9.0 --format json > v1.json

echo "v1.9.0 median lead time: $(jq -r '.aggregates.lead_time.median_seconds / 86400 | round | "\(.)d"' v1.json)"
echo "v2.0.0 median lead time: $(jq -r '.aggregates.lead_time.median_seconds / 86400 | round | "\(.)d"' v2.json)"
```

Compare bug ratios:

```bash
echo "v1.9.0 bug ratio: $(jq -r '.composition.bug_ratio * 100 | round | "\(.)%"' v1.json)"
echo "v2.0.0 bug ratio: $(jq -r '.composition.bug_ratio * 100 | round | "\(.)%"' v2.json)"
```

## Find your slowest issues

```bash
gh velocity quality release v1.2.0 --format json | \
  jq -r '.issues | sort_by(-.lead_time_seconds) | .[0:5] | .[] |
    "#\(.number) \(.title[0:40]) -- \(.lead_time_seconds / 86400 | round)d"'
```

## Check label coverage before a release

```bash
gh velocity quality release v1.2.0 --format json | \
  jq '"Bug: \(.composition.bug_count), Feature: \(.composition.feature_count), Unlabeled: \(.composition.other_count)"'
```

If `other_count` is high, label your issues before publishing the release for more useful composition metrics. Run `gh velocity config preflight` to discover available labels and generate matching category matchers.

## Use --since to override the previous tag

When the auto-detected previous tag is wrong (non-linear tag history, pre-releases mixed with stable releases), override it explicitly:

```bash
gh velocity quality release v2.0.0 --since v1.9.0
gh velocity quality release v2.0.0 --since v1.9.0 --discover
```

The `--discover` flag shows which issues and PRs each linking strategy found, which helps debug unexpected results.

## Analyze a repo you don't have locally

Every command works with `-R` (or `--repo`):

```bash
gh velocity quality release v0.28.0 -R charmbracelet/bubbletea
gh velocity flow lead-time 500 -R charmbracelet/bubbletea
gh velocity quality release v5.2.1 -R go-chi/chi --discover
gh velocity flow throughput --since 30d -R cli/cli
```

All commands work remotely. Cycle time uses API-based signals (PR creation date, label events, project status). Running from inside a local checkout adds commit counts and a fallback signal from commit history.

## Generate a report for every release

```bash
for tag in $(gh api repos/owner/repo/tags --jq '.[].name' | head -5); do
  echo "=== $tag ==="
  gh velocity quality release "$tag" -R owner/repo 2>/dev/null
  echo
done
```

To save each report as JSON for later analysis:

```bash
mkdir -p reports
for tag in $(gh api repos/owner/repo/tags --jq '.[].name' | head -5); do
  gh velocity quality release "$tag" -R owner/repo --format json > "reports/${tag}.json" 2>/dev/null
done
```

## Export to CSV for spreadsheet analysis

```bash
gh velocity quality release v1.2.0 --format json | \
  jq -r '["number","title","lead_time_days","cycle_time_days","outlier"],
    (.issues[] | [
      .number,
      .title,
      ((.lead_time_seconds // 0) / 86400 | round),
      ((.cycle_time_seconds // 0) / 86400 | round),
      .lead_time_outlier
    ]) | @csv' > release-metrics.csv
```

## Use --scope for ad-hoc filtering

The `--scope` flag adds GitHub search qualifiers that are AND'd with any `scope.query` in your config:

```bash
# Only issues assigned to a specific person
gh velocity flow lead-time --since 30d --scope "assignee:octocat"

# Only issues with a specific label
gh velocity flow throughput --since 30d --scope "label:team-backend"

# Combine multiple qualifiers
gh velocity report --since 30d --scope "label:team-frontend assignee:alice"
```

## Check what each linking strategy found

The `--discover` flag on `quality release` shows what each [linking strategy]({{< relref "/concepts/linking-strategies" >}}) (`pr-link`, `commit-ref`, `changelog`) discovered:

```bash
gh velocity quality release v1.2.0 --discover
```

The output lists issues found by each strategy and marks items that appear in multiple strategies. Use this to understand how well the strategies cover your workflow and whether you need to adjust [`commit_ref.patterns`]({{< relref "/reference/config" >}}#commit_ref) in your config.

## Bulk lead-time analysis

Get per-issue lead times for all issues closed in a window:

```bash
gh velocity flow lead-time --since 30d --format json | \
  jq -r '.issues[] | "#\(.number) \(.title[0:40]) -- \(.lead_time_seconds / 86400 | round)d"'
```

## Cycle time for a specific PR

Override the configured strategy and measure a single PR directly:

```bash
gh velocity flow cycle-time --pr 99
gh velocity flow cycle-time --pr 99 --format json
```

This always uses PR created-to-merged timing, regardless of what `cycle_time.strategy` is set to in config.

## Weekly velocity in JSON for dashboards

```bash
gh velocity report --since 7d --format json > weekly.json
```

Extract key numbers:

```bash
jq '{
  issues_closed: .throughput.issues_closed,
  prs_merged: .throughput.prs_merged,
  median_lead_time_days: (.lead_time.median_seconds / 86400 | round),
  median_cycle_time_days: (.cycle_time.median_seconds / 86400 | round)
}' weekly.json
```

## Post a report and save it locally

```bash
# Save and post in one go
gh velocity report --since 30d --format markdown | tee report.md | \
  gh issue create --title "Weekly metrics" --body-file -
```

## Prep for a 1:1 with my-week

Get a personal summary of your recent activity — issues closed, PRs merged, reviews done — plus what's on your plate:

```bash
gh velocity status my-week
```

Customize the lookback period:

```bash
gh velocity status my-week --since 14d
```

The output includes cycle time for issues you closed, making it easy to discuss delivery speed with your manager.

## Next steps

- [Agent Integration]({{< relref "agent-integration" >}}) -- more jq patterns for programmatic use
- [CI Setup]({{< relref "/getting-started/ci-setup" >}}) -- automate these recipes in GitHub Actions
- [Posting Reports]({{< relref "posting-reports" >}}) -- use `--post` for built-in posting
