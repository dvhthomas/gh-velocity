---
title: "GitHub Velocity"
type: docs
---

# GitHub Velocity

**Measure how fast your team ships** — velocity, quality, and flow metrics from GitHub data.

`gh-velocity` is a GitHub CLI extension that computes development metrics and posts them where the work happens: issues, discussions, and release notes.

## What you can measure

- **Flow metrics**: Lead time, cycle time, velocity (effort per sprint), throughput
- **Quality metrics**: Defect rate, hotfix detection, category composition per release
- **Risk signals**: Bus factor, knowledge concentration per directory
- **Status**: Work in progress, review pressure, personal weekly summary

## Get started

| | | |
|---|---|---|
| **[Getting Started]({{< relref "getting-started" >}})** | **[Guides]({{< relref "guides" >}})** | **[Concepts]({{< relref "concepts" >}})** |
| Install, configure, and run your first command in 5 minutes. | Task-oriented help: interpreting results, setting up velocity, CI integration. | How gh-velocity works: metric definitions, statistics, linking strategies. |
| **[Reference]({{< relref "reference" >}})** | **[Examples]({{< relref "examples" >}})** | |
| Complete CLI, config, and metric reference documentation. | Real-world configs for popular repositories. | |

## Install

```bash
gh extension install dvhthomas/gh-velocity
```

Then generate a config tailored to your repo:

```bash
gh velocity config preflight --write
```

Run `gh velocity config preflight --help` to see all options — the repo is auto-detected from your git remote.

---

Maintained by [BitsByD](https://bitsby.me/about) · [Source on GitHub](https://github.com/dvhthomas/gh-velocity)
