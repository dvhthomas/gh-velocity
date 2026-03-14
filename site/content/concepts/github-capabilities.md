---
title: "GitHub's Capabilities & Limits"
weight: 1
---

# What GitHub Can and Cannot Tell You

`gh-velocity` computes metrics directly from the GitHub API. That means zero setup, but it also means you are constrained to what the API exposes. This page lays out exactly what works, what has rough edges, and what is fundamentally impossible.

## What works well

**Issue lifecycle.** Creation and closure dates are precise and always available. Lead time (created to closed) is the most reliable metric the tool produces.

**PR merge timestamps.** The search API returns exact merge dates. The `pr-link` strategy uses these to find PRs merged within a release window, giving you accurate release composition.

**Closing references.** GitHub tracks which PRs close which issues. The GraphQL `closingIssuesReferences` field is the most reliable way to connect PRs to issues. When your PRs include "Fixes #42" or "Closes #42", the tool picks up the connection automatically.

**Release metadata.** Tags, release dates, and release bodies are all available via the REST API. Publishing GitHub Releases (not just tags) gives the tool precise timestamps for computing release lag and cadence.

**Labels.** Issue labels are the basis for classification (bug, feature, etc.) and lifecycle tracking. Label event timestamps are immutable, making them the most reliable signal for cycle time measurement.

## What has limits

**Cycle time depends on your configured strategy.** With the `pr` strategy, the tool uses the closing PR's creation and merge dates. With the `issue` strategy, it prefers label events (`lifecycle.in-progress.match`) and falls back to project board status. If neither strategy has a signal for a given issue, cycle time is reported as N/A. The tool warns you when this happens.

**Project board timestamps are unreliable for cycle time.** The GitHub Projects v2 API exposes only `updatedAt` on field values -- the timestamp of the *last* status change, not the original transition. If someone moves a card to "Done" after an issue is already closed, `updatedAt` reflects that post-closure move. This can produce negative cycle times. The tool filters negative durations from aggregate statistics and warns you, but the root cause cannot be fixed without switching to label-based timestamps. See [Labels vs. Project Board]({{< relref "labels-vs-board" >}}) for the full explanation.

**The PR search API caps at 1000 results.** If a release window contains more than 1000 merged PRs, the `pr-link` strategy warns you and returns partial results. This is rare outside the largest monorepos.

**Tag ordering is by API default, not semver.** Tags are returned in the order GitHub's API provides, which is usually creation date. The tool picks the tag immediately before your target tag in this list. If your tag history is non-linear, use `--since` to specify the previous tag explicitly.

**"Closed" is not "merged."** GitHub issues can be closed without a PR being merged -- by a maintainer, a bot, or the author. `gh-velocity` treats closure as the end event regardless of cause. For most teams this is fine. For teams that close stale issues aggressively, it may inflate lead time counts.

**Label-based classification is only as good as your labels.** If more than half the issues in a release lack bug/feature labels, the tool warns you. You can customize which labels map to which categories in your config file.

## What is not possible

**Project board transition history.** GitHub Projects v2 has no API for field change history. You cannot query "when did this issue move to In Progress?" -- only "what is the current status, and when was it last modified?" This is why label events are the recommended cycle time signal: `LABELED_EVENT.createdAt` is immutable and records the exact moment a label was applied.

**Work-in-progress duration as separate phases.** Without transition history, there is no way to measure time-in-review or time-in-backlog as separate phases using project board data alone. Labels partially address this -- you could use separate labels for each phase (`in-review`, `blocked`) and measure durations between label events.

**Developer-level attribution.** The tool measures issue and release velocity, not individual performance. This is intentional.

**Cross-repo tracking.** Each invocation targets a single repository. Multi-repo releases require separate runs.

## See also

- [Labels vs. Project Board]({{< relref "/concepts/labels-vs-board" >}}) -- detailed explanation of the project board timestamp limitation
- [Linking Strategies]({{< relref "/concepts/linking-strategies" >}}) -- how the tool connects PRs to issues and releases
- [API Consumption]({{< relref "/reference/api-consumption" >}}) -- rate limits, per-command costs, and optimization tips
- [Configuration]({{< relref "/getting-started/configuration" >}}) -- set up your config to work within these constraints
