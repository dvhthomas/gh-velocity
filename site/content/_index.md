---
title: "gh-velocity"
type: docs
---

<div style="text-align: center; margin: 4rem 0 2rem;">
  <div style="display: inline-flex; align-items: flex-end; gap: 0.5rem;">
    {{< asset-img src="images/logo.svg" alt="Shelly" style="height: 5rem; image-rendering: pixelated;" >}}
    <span style="font-family: var(--font-title); font-size: 5.6rem; font-style: italic; font-weight: 600; color: #22C55E; line-height: 1;">velocity</span>
  </div>
</div>

<h1 class="no-shelly" style="text-align: center; font-size: 2.4rem; margin-bottom: 1rem; line-height: 1.2;">
  Measure what matters.
</h1>

<p style="text-align: center; color: #666; font-size: 1.2rem; max-width: 560px; margin: 0 auto 2.5rem;">
  gh-velocity turns your GitHub issues, PRs, and releases into flow metrics and actionable insights — then posts them right where your team works. Not just data. Answers.
</p>

<p style="text-align: center; margin-bottom: 4rem;">
  <a href="{{< relref "getting-started" >}}" style="display: inline-block; background: #22C55E; color: white; padding: 0.85rem 2.5rem; border-radius: 6px; text-decoration: none; font-weight: bold; font-size: 1.2rem;">Get Started</a>
</p>

---

## I don't want to configure anything.

You don't have to. [Preflight]({{< relref "/getting-started/configuration" >}}#generate-a-config-with-preflight) analyzes your repo — labels, boards, issue types, recent activity — and writes the config for you. One command. Zero decisions. [Start measuring]({{< relref "/getting-started/quick-start" >}}).

```bash
gh velocity config preflight --write
```

```
Analyzing dvhthomas/gh-velocity...

  Labels found      12 (bug, enhancement, in-progress, ...)
  Issue types       bug, feature (native GitHub Issue Types)
  Project board     users/dvhthomas/projects/1
  Lifecycle labels  in-progress, in-review, done
  Noise labels      duplicate, invalid (auto-excluded)

Wrote .gh-velocity.yml
```

---

## What did I even do this week?

Your [week]({{< relref "/guides/ad-hoc-queries" >}}#prep-for-a-11-with-my-week), summarized — including where you're stuck.

```bash
gh velocity status my-week
```

```
My Week — dvhthomas (dvhthomas/gh-velocity)
  2026-03-14 to 2026-03-21

── Insights ────────────────────────────────

  Shipped 10 items (4 issues closed, 6 PRs merged) in 7 days.
  Reviewed 3 PRs.
  2 of 6 PRs were AI-assisted (33%).
  Median lead time: 2d 4h, p90: 11d (issue created → closed).
  WAITING: 1 stale issue(s) (7+ days idle).

── What I shipped ──────────────────────────

Issues Closed: 4
  #152  2026-03-18  feat: add --title flag to override discussion title
  #148  2026-03-17  fix: show bug emoji for type-based matches
  #145  2026-03-15  feat: add discussions.title config
  #140  2026-03-14  feat: enrich REST issues with IssueType via GraphQL

── Waiting on ──────────────────────────────

Stale Issues: 1
  #38  Investigate cross-invocation cache  (no update in 14d)
```

---

## How is our project actually going?

[One command]({{< relref "/getting-started/quick-start" >}}) gives you flow metrics, quality breakdown, and insights across your repos.

```bash
gh velocity report --since 30d
```

```
Report: cli/cli (2026-02-19 – 2026-03-21 UTC)

Key Findings:

  Lead Time:
  → 5 items took 8x longer than the median (4d 12h).
  → Moderate delivery time variability (CV 0.8) — some items
    take significantly longer than others.

  Throughput:
  → 62 PRs merged but 47 issues closed — PRs may not be
    linked to issues.

  Lead Time:   median 4d 12h, P90 18d, predictability: moderate (n=47)
  Cycle Time:  median 1d 8h, P90 6d, predictability: moderate (n=39)
  Throughput:  47 issues closed, 62 PRs merged
  WIP:         23 items (4 stale)
```

The [gh-velocity showcase](https://github.com/dvhthomas/gh-velocity/discussions/categories/velocity-reports) includes configurations and reports for popular open source projects.

---

## Are bots helping or hurting?

Dependabot merged 47 PRs last month — are those inflating your numbers? [`--scope`]({{< relref "/getting-started/configuration" >}}) accepts any [GitHub search qualifier](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests) — author, label, milestone, you name it.

```bash
# Overall lead time
gh velocity flow lead-time --since 30d -R microsoft/vscode

# Just humans
gh velocity flow lead-time --since 30d \
  --scope "repo:microsoft/vscode -author:app/dependabot"
```

```
Lead Time: microsoft/vscode (2026-02-19 – 2026-03-21 UTC)

  → 47 issues closed in under 60 seconds — consider narrowing
    scope to exclude noise (e.g., scope: '-label:invalid').
  → Mean (14d) is much higher than median (1d 4h) — a few slow
    items are pulling the average up.

  median 1d 4h, P90 12d 8h, predictability: low (n=312)
```

---

## What's the story on this one issue?

Full [lifecycle]({{< relref "/guides/ad-hoc-queries" >}}), linked PRs, timing breakdown — and what stands out.

```bash
gh velocity issue 42
gh velocity pr 108
```

```
Issue #42: Add dark mode support

  Created      2026-02-10
  In-progress  2026-02-14
  Closed       2026-02-18
  Released     v2.5.0

  Lead time    8d
  Cycle time   4d
  Release lag  3d
  Category     feature
  Linked PRs   #105, #108

  → Lead time ranges from 4d to 8d.
  → bug (median 2d) faster than feature (median 6d 4h).
```

---

## Can this just... run itself?

Yes. A [GitHub Actions cron job]({{< relref "/getting-started/ci-setup" >}}) [posts a velocity report]({{< relref "/guides/posting-reports" >}}) to Discussions every Monday. Your team reads it over coffee.

```yaml
# .github/workflows/velocity.yml
name: Weekly Velocity
on:
  schedule:
    - cron: '0 9 * * 1'  # Monday 9am UTC
  workflow_dispatch:

permissions:
  contents: read
  issues: write
  discussions: write

jobs:
  report:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install dvhthomas/gh-velocity
        env: { GH_TOKEN: "${{ secrets.GITHUB_TOKEN }}" }
      - run: gh velocity report --since 30d --post -r markdown
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GH_VELOCITY_POST_LIVE: 'true'
```

---

Maintained by [BitsByD](https://bitsby.me/about) · [Source on GitHub](https://github.com/dvhthomas/gh-velocity) · [Meet Shelly]({{< relref "shelly" >}})
