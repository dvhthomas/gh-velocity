---
title: "Getting Started"
weight: 1
bookCollapseSection: true
---

# Getting Started

gh-velocity has three levels of depth. Each builds on the previous one.

| Level | What you get | What you need |
|-------|-------------|---------------|
| **Summary reports** | Lead time, throughput, quality metrics for a time window. | `preflight --write` to generate a config. Run `report --since 30d`. Done. |
| **Better classification** | Accurate bug/feature breakdowns in reports. | Consistent labels on issues. The [project-label-sync](https://github.com/dvhthomas/project-label-sync) Action can apply labels from a GitHub Projects board automatically. |
| **Per-issue and per-PR metrics** | Metrics posted to each issue/PR body on close/merge. | A [workflow file]({{< relref "/guides/posting-reports" >}}) committed to each repo. GitHub Actions triggers are per-repo — there is no cross-repo mechanism. |

Start with the [Quick Start]({{< relref "/getting-started/quick-start" >}}) to get summary reports running in under 5 minutes.

{{< children >}}
