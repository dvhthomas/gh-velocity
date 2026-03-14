---
title: "GitHub Velocity"
type: docs
---

# GitHub Velocity

**Measure how fast your team ships** — velocity, quality, and flow metrics from GitHub data.

`gh-velocity` is a GitHub CLI extension that computes development metrics and posts them where the work happens: issues, discussions, and release notes.

## What you can measure

- **Flow metrics**: [Lead time]({{< relref "/reference/metrics/lead-time" >}}), [cycle time]({{< relref "/reference/metrics/cycle-time" >}}), [velocity]({{< relref "/reference/metrics/velocity" >}}) (effort per sprint), [throughput]({{< relref "/reference/metrics/throughput" >}})
- **Quality metrics**: [Defect rate, hotfix detection, category composition]({{< relref "/reference/metrics/quality" >}}) per release
- **Risk signals**: Bus factor, knowledge concentration per directory
- **Status**: Work in progress, review pressure, personal weekly summary

## Get started

- **[Getting Started]({{< relref "getting-started" >}})** — Install, configure, and run your first command in 5 minutes
- **[Guides]({{< relref "guides" >}})** — Task-oriented help: interpreting results, setting up velocity, CI integration
- **[Concepts]({{< relref "concepts" >}})** — How gh-velocity works: metric definitions, statistics, linking strategies
- **[Reference]({{< relref "reference" >}})** — Complete CLI, config, and metric reference documentation
- **[Examples]({{< relref "examples" >}})** — Real-world configs for popular repositories

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
