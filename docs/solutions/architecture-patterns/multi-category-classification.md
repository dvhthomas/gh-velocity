---
title: Multi-category classification
category: architecture-patterns
date: 2026-03-13
tags: [classify, categories, quality, data-quality, multi-match]
related: [ClassifyResult, Classifier, CategoryConfig]
---

# Multi-category classification

## Problem

`Classify()` used first-match-wins: an issue matching multiple category matchers would only be classified into the first match. This hid data quality issues — users couldn't see when their category matchers overlapped.

## Solution

Changed `Classify()` to return ALL matched categories. Multi-category matches are informational diagnostics, not errors.

```go
type ClassifyResult struct {
    Categories []string
}

func (r ClassifyResult) Category() string {
    if len(r.Categories) == 0 { return "other" }
    return r.Categories[0]  // backward compat: first match is primary
}

func (r ClassifyResult) MultiMatch() bool {
    return len(r.Categories) > 1
}
```

### Key distinction

- **Multi-category** (e.g., issue is both "bug" and "security"): informational, helpful diagnostic. Shows overlapping matchers in quality config.
- **Multi-lifecycle-stage** (e.g., issue matches both "backlog" and "in-progress"): actual data quality problem, since an issue can only be in one stage at a time.

### Backward compatibility

`Category()` method returns first match, preserving all existing code paths that use `result.Category()`. New code can inspect `result.Categories` for the full list and `result.MultiMatch()` to flag overlaps.

## Prevention

- When adding new classification dimensions, always return all matches — let the caller decide what to do with overlaps
- Reserve "error" treatment for truly conflicting states (lifecycle stages), not informational overlap (quality categories)
