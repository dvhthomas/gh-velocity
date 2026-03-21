---
title: "feat: Emit type: matchers in preflight config from discovered issue types"
type: feat
status: completed
date: 2026-03-21
origin: docs/brainstorms/2026-03-21-issue-type-preflight-requirements.md
---

# feat: Emit type: matchers in preflight config from discovered issue types

## Overview

Preflight discovers GitHub Issue Types via `ListIssueTypes()` and maps them through `typePatterns`, but the generated `--write` config never includes `type:` matchers. This is because evidence probing runs against REST-fetched issues that lack the `IssueType` field, so type matcher hit counts are always 0, and the rendering code silently drops them (see origin: `docs/brainstorms/2026-03-21-issue-type-preflight-requirements.md`).

## Problem Statement

Repos that use GitHub Issue Types (Bug, Feature, Task) get configs with only label and title matchers — missing the strongest, most authoritative classification signal. Users must manually add `type:` matchers, defeating the purpose of preflight auto-discovery.

## Proposed Solution

**Direct injection in `renderPreflightConfig()`** — bypass evidence hit counts for type matchers entirely. Build type matchers from `result.DiscoveredTypes` + `typePatterns` and inject them directly into each category's matcher list, before label matchers.

This is simpler than enriching `MatcherEvidence` with a new struct field because:
- No JSON output shape change
- No need to thread a new field through evidence collection, diagnostics, and rendering
- Type matchers derive from repo configuration, not evidence — conceptually different from probed matchers

## Technical Considerations

### Rendering injection point: `renderPreflightConfig()` (`cmd/preflight.go:1162-1226`)

**Current flow:**
1. Build `evidenceByCategory` from `r.MatchEvidence`
2. Per category: split into `labelHits` (non-suggested, count>0) and `titleHits` (suggested)
3. Fallback: if no labelHits, reconstruct from `r.Categories`
4. Combine: `labelHits + titleHits`

**New flow:**
1. Build `evidenceByCategory` from `r.MatchEvidence` (unchanged)
2. Build `typeMatchersByCategory` from `r.DiscoveredTypes` + `typePatterns` (new)
3. Per category: split evidence into `labelHits` and `titleHits` (unchanged)
4. Fallback for labels (unchanged)
5. Combine: **`typeHits + labelHits + titleHits`** (type matchers first per R2)

### Evidence display: `printPreflightDiagnostics()` (`cmd/preflight.go:1561-1611`)

Type matchers in evidence will still show "0 matches" since they're probed against REST items. Two options:
- **Option A:** Check `strings.HasPrefix(me.Matcher, "type:")` and display "repo-configured" instead of count
- **Option B:** Skip type matchers in evidence display entirely (they appear in the hints as "Discovered issue types: [Bug, Feature, Task]")

Recommend **Option A** — explicit is better than silent.

### Case sensitivity in `typePatterns` lookup (`cmd/preflight.go:1037`)

Current: `typeName == pattern` (exact match).
Change to: `strings.EqualFold(typeName, pattern)` for consistency with `TypeMatcher.Matches()` runtime behavior.

### Edge case: no evidence at all (`!hasEvidence` fallback at line 1176)

When no recent items exist, the `!hasEvidence` branch only reconstructs label matchers. The new `typeMatchersByCategory` injection must also apply in this path. Since type matchers are built independently of evidence, they should be prepended regardless of whether evidence exists.

### YAML comment for repo-configured matchers

Add inline comment `# repo-configured issue type` to type matchers in rendered YAML for user clarity.

## Acceptance Criteria

- [ ] Running `gh velocity config preflight` on a repo with GitHub Issue Types produces config containing `type:<Name>` matchers for mapped categories
- [ ] `type:` matchers appear before `label:` matchers in each category's match list
- [ ] Evidence diagnostics show "repo-configured" for type matchers instead of "0 matches"
- [ ] `typePatterns` lookup uses case-insensitive matching
- [ ] Config with type matchers round-trips through `config.Parse()` and `classify.ParseMatcher()`
- [ ] Repos with no issue types produce unchanged config (no regression)
- [ ] Repos with issue types but no labels produce categories with type matchers only
- [ ] Repos with no recent activity but valid issue types still get type matchers
- [ ] Unmapped types still produce the existing hint (R4, unchanged)

## MVP

### Step 1: Add case-insensitive `typePatterns` lookup in `collectMatchEvidence`

**File:** `cmd/preflight.go:1037`

```go
// Before:
if typeName == pattern {

// After:
if strings.EqualFold(typeName, pattern) {
```

### Step 2: Build `typeMatchersByCategory` helper

**File:** `cmd/preflight.go` (new helper near `renderPreflightConfig`)

```go
// typeMatchersFromDiscovery builds type: matchers from discovered repo types
// cross-referenced with typePatterns. Returns a map of category -> matcher strings.
func typeMatchersFromDiscovery(discoveredTypes []string) map[string][]string {
    result := make(map[string][]string)
    for cat, patterns := range typePatterns {
        for _, typeName := range discoveredTypes {
            for _, pattern := range patterns {
                if strings.EqualFold(typeName, pattern) {
                    result[cat] = append(result[cat], "type:"+typeName)
                }
            }
        }
    }
    return result
}
```

### Step 3: Inject type matchers in `renderPreflightConfig`

**File:** `cmd/preflight.go:1162-1226`

Build `typeByCategory` at the top of the evidence loop, then prepend to each category's combined matchers:

```go
// Build type matchers from discovered repo types (no evidence needed).
typeByCategory := typeMatchersFromDiscovery(r.DiscoveredTypes)

// ... existing category loop ...

for _, cat := range categoryOrder {
    // ... existing labelHits/titleHits logic ...

    // Prepend type matchers (strongest signal, first-match-wins).
    var typeHits []MatcherEvidence
    for _, tm := range typeByCategory[cat] {
        typeHits = append(typeHits, MatcherEvidence{Matcher: tm})
    }

    // Combine: type first, then labels, then title probes.
    var combined []MatcherEvidence
    combined = append(combined, typeHits...)
    combined = append(combined, labelHits...)
    combined = append(combined, titleHits...)
    // ...
}
```

Also handle the `!hasEvidence` fallback path — inject type matchers there too:

```go
if !hasEvidence {
    var mes []MatcherEvidence
    // Type matchers first
    for _, tm := range typeByCategory[cat] {
        mes = append(mes, MatcherEvidence{Matcher: tm})
    }
    // Then label matchers
    if labels, ok := r.Categories[cat]; ok && len(labels) > 0 {
        for _, l := range labels {
            mes = append(mes, MatcherEvidence{Matcher: "label:" + l})
        }
    }
    if len(mes) > 0 {
        cats = append(cats, effectiveCategory{name: cat, matchers: mes})
    }
    continue
}
```

### Step 4: Add YAML comment for type matchers

**File:** `cmd/preflight.go` in the matcher rendering loop (~line 1233)

```go
for _, m := range cat.matchers {
    if strings.HasPrefix(m.Matcher, "type:") {
        b.WriteString(fmt.Sprintf("        - %q  # repo-configured issue type\n", m.Matcher))
    } else {
        b.WriteString(fmt.Sprintf("        - %q\n", m.Matcher))
    }
}
```

### Step 5: Update `printPreflightDiagnostics` for type matchers

**File:** `cmd/preflight.go:1572-1577`

```go
for _, me := range ce.Matchers {
    if strings.HasPrefix(me.Matcher, "type:") {
        log.Notice("  %s / %s — repo-configured", ce.Category, me.Matcher)
    } else if me.Count > 0 {
        log.Notice("  %s / %s — %d matches, e.g. %s", ce.Category, me.Matcher, me.Count, me.Example)
    } else {
        log.Notice("  %s / %s — 0 matches", ce.Category, me.Matcher)
    }
}
```

### Step 6: Tests

**File:** `cmd/preflight_test.go`

**Test 1: `TestRenderPreflightConfig_TypeMatchersFromDiscoveredTypes`**
- Input: `PreflightResult` with `DiscoveredTypes: ["Bug", "Feature", "Task"]` and label-based `Categories`
- Assert: rendered YAML contains `"type:Bug"` before `"label:bug"`, `"type:Feature"` before `"label:enhancement"`, `"type:Task"` in chore
- Assert: round-trips through `config.Parse()`

**Test 2: `TestRenderPreflightConfig_TypeMatchersWithoutLabels`**
- Input: `DiscoveredTypes: ["Bug"]`, empty `Categories`, no `MatchEvidence`
- Assert: rendered YAML has bug category with `"type:Bug"` only (no label matchers)

**Test 3: `TestRenderPreflightConfig_TypeMatchersNoRecentActivity`**
- Input: `DiscoveredTypes: ["Bug", "Feature"]`, empty `MatchEvidence` (nil)
- Assert: type matchers still emitted (exercises `!hasEvidence` fallback path)

**Test 4: `TestRenderPreflightConfig_NoDiscoveredTypes`**
- Input: `DiscoveredTypes: nil`, existing label categories
- Assert: output unchanged from current behavior (no regression)

**Test 5: `TestTypeMatchersFromDiscovery_CaseInsensitive`**
- Input: `discoveredTypes: ["bug", "FEATURE", "Task"]`
- Assert: all three map correctly despite non-standard casing

**Test 6: `TestCollectMatchEvidence_TypeMatchersCaseInsensitive`**
- Existing test at line 549 extended with lowercase type names
- Assert: `type:bug` probe job created for discovered type "bug"

## Sources

- **Origin document:** [docs/brainstorms/2026-03-21-issue-type-preflight-requirements.md](docs/brainstorms/2026-03-21-issue-type-preflight-requirements.md) — key decisions: trust repo-configured types without extra API call, type: before label: in match order
- **Evidence-driven preflight config:** `docs/solutions/evidence-driven-preflight-config.md` — parallel probe pattern, evidence comments in YAML
- **Pipeline-per-metric config:** `docs/solutions/architecture-refactors/pipeline-per-metric-and-preflight-first-config.md` — preflight is source of truth for config correctness

### Key File References

- `cmd/preflight.go:986-992` — `typePatterns` map
- `cmd/preflight.go:1010-1081` — `collectMatchEvidence()` (type probe jobs at 1033-1042)
- `cmd/preflight.go:1133-1250` — `renderPreflightConfig()` (injection point)
- `cmd/preflight.go:1561-1611` — `printPreflightDiagnostics()` (display fix)
- `cmd/preflight_test.go:549-618` — existing type evidence tests
- `internal/classify/classify.go:139-146` — `TypeMatcher` implementation
- `internal/github/issuetypes.go` — `ListIssueTypes()` GraphQL query
