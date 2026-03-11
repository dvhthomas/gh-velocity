---
title: "Evidence-Driven Preflight Config Generation"
category: features
tags: [preflight, config, matchers, classification, labels, title-regex, probing, parallel]
module: cmd/preflight.go
symptom: "Generated configs had zero-match matchers; users couldn't validate if categories would work"
root_cause: "Preflight guessed matchers without testing them against actual repo data"
date: 2026-03-11
---

## Problem

The `config preflight` command generated configs with matchers that often had zero matches
in the target repo. Users had no way to know if their categories would classify anything
until they ran a real command. Repos with conventional commit titles (e.g., `feat:`, `fix:`)
had no labels, making label-only matchers useless.

## Solution

### Parallel Matcher Probing

Preflight now probes ALL matcher candidates (both labels and title regex patterns) against
recent issues/PRs from the repo. Each probe runs as a goroutine since they're pure functions
operating on the same read-only slice:

```go
var wg sync.WaitGroup
for i, job := range jobs {
    wg.Add(1)
    go func(idx int, j probeJob) {
        defer wg.Done()
        results[idx] = probeResult{evidence: probeMatcher(j.matcher, items)}
    }(i, job)
}
wg.Wait()
```

### Evidence-Driven Selection

The config uses **labels when they have signal**, falling back to **title regex when labels
find nothing**. This handles two common repo styles:
- Label-heavy repos (e.g., kubernetes: `kind/bug`, `kind/feature`) → label matchers win
- Conventional-commit repos (e.g., our own) → title matchers like `title:/^feat[\(: ]/i` win

### Match Evidence Comments

Generated configs include a comment block showing every probed matcher with its hit count
and one example match. Zero-match matchers get "(review this matcher)" annotations:

```yaml
# Match evidence (last 30 days of issues + PRs):
#   bug / label:bug — 33 matches, e.g. #12893 error parsing "input[title]"...
#   bug / title:/^fix[\(: ]/i — 11 matches, e.g. #12836 Fix extension install...
#   feature / label:enhancement — 37 matches, e.g. #12862 Remove a book...
```

## Key Decisions

1. **Probe ALL ideas, select best** — Don't stop at first match. Show everything so users
   can make informed choices about which matchers to keep.
2. **Labels preferred over title regex** — Labels are explicit intent; title patterns are
   heuristic. When both have signal, labels are more reliable.
3. **First-match-wins is fine** — Categories don't need mutual exclusivity in matchers
   because the classifier stops at first matching category. But avoid redundant matchers
   within a category.
4. **Ignore prefixes** — Labels starting with `event/`, `do-not-merge`, `needs-` are
   filtered out to prevent false positive classification.

## Gotchas

- Labels with colons or spaces need careful YAML escaping: `label:\"Resolution: Backlog\"`
- `defaults()` always sets `BugLabels: ["bug"]` — comparing against defaults before warning
  about legacy label precedence prevents false alarms on preflight-generated configs.
- Title probes use conventional commit patterns (`^feat[\(: ]`) — the `[\(: ]` suffix
  prevents matching words like "feature" in the middle of titles.
