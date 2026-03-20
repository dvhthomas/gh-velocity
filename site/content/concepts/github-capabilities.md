---
title: "GitHub's Capabilities & Limits"
weight: 1
---

# What GitHub Can and Cannot Tell You

gh-velocity computes metrics directly from the GitHub API. That means zero setup, but also means the tool is constrained to what the API exposes. This page covers what works, what has rough edges, and what is fundamentally impossible.

## What works well

**Issue lifecycle.** Creation and closure dates are precise and always available. Lead time (created to closed) is the most reliable metric.

**PR merge timestamps.** The search API returns exact merge dates. The `pr-link` strategy uses these to find PRs merged within a release window, giving you accurate release composition.

**Closing references.** GitHub tracks which PRs close which issues via the GraphQL `closingIssuesReferences` field. When PRs include "Fixes #42" or "Closes #42", the tool picks up the connection automatically.

**Release metadata.** Tags, release dates, and release bodies are all available via the REST API. Publishing GitHub Releases (not just tags) gives the tool precise timestamps for computing release lag and cadence.

**Labels.** Issue labels are the basis for classification (bug, feature, etc.) and lifecycle tracking. Label event timestamps are immutable, making them the most reliable signal for cycle time measurement.

## What has limits

**Cycle time depends on the configured strategy.** The PR strategy uses the closing PR's creation and merge dates. The issue strategy uses label events (`lifecycle.in-progress.match`). If neither strategy has a signal for a given issue, cycle time is N/A.

**Project board timestamps are mutable.** The GitHub Projects v2 API exposes only `updatedAt` on field values -- the last status change, not the original transition. Labels are the sole lifecycle signal. Project boards remain useful for velocity reads (iteration tracking, effort fields). See [Labels as Lifecycle Signal]({{< relref "labels-vs-board" >}}).

**The PR search API caps at 1,000 results.** If a release window contains more than 1,000 merged PRs, the pr-link strategy warns and returns partial results. This is rare outside the largest monorepos.

**Tag ordering is by API default, not semver.** Tags are returned in the order GitHub's API provides, which is usually creation date. The tool picks the tag immediately before your target tag in this list. If your tag history is non-linear, use `--since` to specify the previous tag explicitly.

**"Closed" is not "merged."** GitHub issues can be closed without a PR being merged -- by a maintainer, a bot, or the author. gh-velocity treats closure as the end event regardless of cause. For teams that close stale issues aggressively, this may inflate lead time counts.

**Label-based classification is only as good as your labels.** If more than half the issues in a release lack bug/feature labels, the tool warns. Customize which labels map to which categories in your config.

## What is not possible

**Project board transition history.** GitHub Projects v2 has no API for field change history. There is no way to query "when did this issue move to In Progress?" -- only "what is the current status, and when was it last modified?" This is why labels are the sole lifecycle signal: `LABELED_EVENT.createdAt` is immutable and records the exact moment a label was applied.

**Work-in-progress duration as separate phases.** Without transition history, there is no way to measure time-in-review or time-in-backlog as separate phases from project board data alone. Labels address this: use separate labels for each phase (`in-progress`, `in-review`) and measure durations between label events.

**Developer-level attribution.** The tool measures issue and release velocity, not individual performance. This is intentional.

**Cross-repo tracking.** Each invocation targets a single repository. Multi-repo releases require separate runs.

## See also

- [Labels as Lifecycle Signal]({{< relref "/concepts/labels-vs-board" >}}) -- why labels are the sole lifecycle signal
- [API Consumption]({{< relref "/reference/api-consumption" >}}) -- rate limits, per-command costs, and optimization tips
- [Configuration]({{< relref "/getting-started/configuration" >}}) -- set up your config to work within these constraints
