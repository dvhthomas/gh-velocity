---
title: "Definitions"
weight: 1
---

# Definitions

gh-velocity is a command-line tool that measures development velocity and quality from GitHub data. It works at a terminal, in GitHub Actions, or in any automation platform -- it is not specific to CI.

---

## Measurement axes

Four independent axes control what gh-velocity measures and how. **Scope** and **lifecycle** filter *which* items to measure. **Iteration** and **effort** control *how* velocity is computed. They combine freely -- changing one does not affect the others.

### Scope

Which items to measure. Configured via `scope.query` in your config file and the `--scope` flag at the command line. Scope determines the universe of issues or PRs that all metrics operate on.

```yaml
scope:
  query: "repo:acme/web label:team-platform"
```

The `--scope` flag narrows further at runtime (AND with config scope). See the [scope configuration reference]({{< relref "/reference/config" >}}#scope) for full syntax.

### Lifecycle

Where an item is in its workflow journey. The stages are: **backlog**, **in-progress**, **in-review**, **done**, and **released**. Labels are the sole lifecycle signal -- their timestamps are immutable, which makes them reliable for cycle-time measurement. A project board can drive your workflow, but labels provide the timestamps gh-velocity reads.

```yaml
lifecycle:
  in-progress:
    match: ["label:in-progress"]
```

See [Labels as Lifecycle Signal]({{< relref "/concepts/labels-vs-board" >}}) for why labels won, and the [lifecycle configuration reference]({{< relref "/reference/config" >}}#lifecycle) for the full list of stages and their fields.

### Iteration

A bounded time period used to bucket velocity results. Answers: "what sprint, phase, or period are we measuring?"

| Strategy | Source | Best for |
|----------|--------|----------|
| `project-field` | GitHub Projects Iteration field | Teams using board-based sprints |
| `fixed` | Calendar math from an anchor date and length | Fixed-length cycles without a board |

Iteration is independent of scope and lifecycle. It determines *when* to measure, not *what* to measure. See [Setting Up Velocity]({{< relref "/guides/velocity-setup" >}}) for configuration.

### Effort

How much work an item represents. Answers: "how do we weight work output?"

| Strategy | Source | Best for |
|----------|--------|----------|
| `count` | Every item = 1 | No estimation process |
| `attribute` | Label or field matchers mapped to numeric values | T-shirt sizes, custom categories |
| `numeric` | Project board Number field | Story points or other numeric estimates |

When effort uses a project board field (`numeric` or `field:` matchers), items that are in scope but not on the board have no effort data. gh-velocity surfaces these as an [insight](#insight) so you can decide whether to add them to the board or accept the gap.

See [Setting Up Velocity]({{< relref "/guides/velocity-setup" >}}) for configuration.

### How the axes combine

| Axis | Controls | Configured in |
|------|----------|---------------|
| Scope | Which items enter the pipeline | `scope.query`, `--scope` flag |
| Lifecycle | Workflow stage and cycle-time signals | `lifecycle.*` label matchers |
| Iteration | Time periods for velocity bucketing | `velocity.iteration.*` |
| Effort | Work weight per item | `velocity.effort.*` |

A project board is an optional data source for iteration and effort. It plays no role in scope or lifecycle.

---

## Metrics

### Velocity

Effort completed per iteration -- a number, not a ratio. Answers "how much work did we ship this period?" See [Velocity reference]({{< relref "/reference/metrics/velocity" >}}).

### Throughput

Count of items closed or merged in a sliding time window. Unlike velocity, throughput is unweighted and not iteration-aligned. See [Throughput reference]({{< relref "/reference/metrics/throughput" >}}).

### Lead time

Elapsed time from issue creation to close. Includes all waiting time (backlog, triage, etc.). See [Lead Time reference]({{< relref "/reference/metrics/lead-time" >}}).

### Cycle time

Elapsed time from when active work begins to issue close. Shorter than lead time because it excludes backlog wait. Two strategies detect when work starts: labels (`issue` strategy) or closing PR creation (`pr` strategy). See [Cycle Time reference]({{< relref "/reference/metrics/cycle-time" >}}).

### Quality metrics

Bug ratio and release composition. Classifies issues using configurable [matchers]({{< relref "/reference/config" >}}#matcher-syntax) and measures what fraction of closed work is bugs. See [Quality reference]({{< relref "/reference/metrics/quality" >}}).

---

## Other key terms

### Insight

A human-readable observation derived from metric data. Insights surface notable patterns (e.g., "60% of items lack effort values") and appear in all output formats. They contain a judgment or comparison, not just a restatement of what is visible in a table.

### Linking strategy

How gh-velocity connects PRs to issues for release quality analysis. Three strategies (`pr-link`, `commit-ref`, `changelog`) are merged to maximize coverage. See [Linking Strategies]({{< relref "/concepts/linking-strategies" >}}).

### Matcher

A pattern used to classify issues and PRs. Matchers appear in lifecycle stages, effort attributes, and quality categories. Syntax: `label:<name>`, `type:<name>`, `field:<Name>/<Value>`. See [Matcher syntax]({{< relref "/reference/config" >}}#matcher-syntax).

---

## See also

- [Labels as Lifecycle Signal]({{< relref "/concepts/labels-vs-board" >}}) -- why labels are the sole lifecycle signal
- [Setting Up Velocity]({{< relref "/guides/velocity-setup" >}}) -- configuring iteration and effort strategies
- [Cycle Time Setup]({{< relref "/guides/cycle-time-setup" >}}) -- lifecycle in practice
- [Configuration Reference]({{< relref "/reference/config" >}}) -- full schema
