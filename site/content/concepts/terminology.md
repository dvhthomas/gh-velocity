---
title: "Key Concepts"
weight: 1
---

# Key Concepts

gh-velocity is a command-line tool that measures development velocity and quality from GitHub data. It works at a terminal, in GitHub Actions, or in any automation platform -- it is not specific to CI.

Four independent axes control what gh-velocity measures and how. **Scope** and **lifecycle** filter *which* items to measure. **Iteration** and **effort** control *how* velocity is computed. They combine freely -- changing one does not affect the others.

## Scope

Which items to measure. Configured via `scope.query` in your config file and the `--scope` flag at the command line. Scope determines the universe of issues or PRs that all metrics operate on.

```yaml
scope:
  query: "repo:acme/web label:team-platform"
```

The `--scope` flag narrows further at runtime (AND with config scope). See [Configuration Reference]({{< relref "/reference/config" >}}) for full syntax.

## Lifecycle

Where an item is in its workflow journey: backlog, in-progress, in-review, done. Labels are the sole lifecycle signal -- their timestamps are immutable, which makes them reliable for cycle-time measurement. A project board can drive your workflow, but labels provide the timestamps gh-velocity reads.

```yaml
lifecycle:
  in-progress:
    match: ["label:in-progress"]
```

See [Labels as Lifecycle Signal]({{< relref "/concepts/labels-vs-board" >}}) for why labels won and how to configure them.

## Iteration

A bounded time period used to bucket velocity results. Answers: "what sprint, phase, or period are we measuring?"

| Strategy | Source | Best for |
|----------|--------|----------|
| `project-field` | GitHub Projects Iteration field | Teams using board-based sprints |
| `fixed` | Calendar math from an anchor date and length | Fixed-length cycles without a board |

Iteration is independent of scope and lifecycle. It determines *when* to measure, not *what* to measure. See [Setting Up Velocity]({{< relref "/guides/velocity-setup" >}}) for configuration.

## Effort

How much work an item represents. Answers: "how do we weight work output?"

| Strategy | Source | Best for |
|----------|--------|----------|
| `count` | Every item = 1 | No estimation process |
| `attribute` | Label or field matchers mapped to numeric values | T-shirt sizes, custom categories |
| `numeric` | Project board Number field | Story points or other numeric estimates |

When effort uses a project board field (`numeric` or `field:` matchers), items that are in scope but not on the board have no effort data. gh-velocity surfaces these as an [insight]({{< relref "/guides/velocity-setup" >}}#controlling-the-output) so you can decide whether to add them to the board or accept the gap.

See [Setting Up Velocity]({{< relref "/guides/velocity-setup" >}}) for configuration.

## How the axes combine

| Axis | Controls | Configured in |
|------|----------|---------------|
| Scope | Which items enter the pipeline | `scope.query`, `--scope` flag |
| Lifecycle | Workflow stage and cycle-time signals | `lifecycle.*` label matchers |
| Iteration | Time periods for velocity bucketing | `velocity.iteration.*` |
| Effort | Work weight per item | `velocity.effort.*` |

A project board is an optional data source for iteration and effort. It plays no role in scope or lifecycle.

## See also

- [Labels as Lifecycle Signal]({{< relref "/concepts/labels-vs-board" >}}) -- why labels are the sole lifecycle signal
- [Setting Up Velocity]({{< relref "/guides/velocity-setup" >}}) -- configuring iteration and effort strategies
- [Cycle Time Setup]({{< relref "/guides/cycle-time-setup" >}}) -- lifecycle in practice
- [Configuration Reference]({{< relref "/reference/config" >}}) -- full schema
