---
title: "fix: Issue Types not used as suggested category filters in preflight"
type: fix
status: completed
date: 2026-03-11
github_issue: https://github.com/dvhthomas/gh-velocity/issues/33
related: https://github.com/dvhthomas/gh-velocity/issues/32
---

# fix: Issue Types not used as suggested category filters in preflight

## Overview

GitHub Issue Types (Bug, Feature, Task) are never discovered or suggested as `type:` matchers by `config preflight`. The classifier infrastructure (`classify.TypeMatcher`, `classify.Input.IssueType`) already supports `type:` matching — the data pipeline simply never populates the field, and preflight never probes for it.

## Problem Statement

When running `gh velocity config preflight --project-url <url>`, the generated config suggests only `label:` and `title:` matchers. Even when project board issues have Issue Types assigned (e.g., `type:Bug`), these are never detected or suggested. The `model.Issue` struct lacks an `IssueType` field, GraphQL queries don't request `issueType { name }`, and `collectMatchEvidence()` has no `type:` probe jobs.

Additionally, the baseline category set should always be `bug`, `feature`, `chore` (with `other` as the implicit catch-all from the classifier), not just `bug` and `feature` as currently defaulted.

## Proposed Solution

Two concurrent discovery paths for issue types, plus pipeline-wide `IssueType` plumbing:

| Flags provided | Discovery path |
|---|---|
| `--project-url` only | Extract `issueType { name }` from project board items (extend existing `projectItemsQuery`) |
| `-R` only | New GraphQL query: `repository { issueTypes { nodes { name } } }` |
| Both `-R` + `--project-url` | Both paths run in parallel via `errgroup`, merge unique type names |

Discovered types are mapped to categories via `typePatterns`, then added as `type:` probe jobs in `collectMatchEvidence()` alongside existing `label:` and `title:` probes.

## Technical Approach

### Phase 1: Model & GraphQL plumbing

Add `IssueType string` to model types and extend GraphQL queries to request `issueType { name }`.

**`internal/model/types.go`**

```go
// Issue — add IssueType field
type Issue struct {
    Number    int
    Title     string
    State     string
    Labels    []string
    IssueType string // GitHub Issue Type (from GraphQL); empty for REST-sourced issues
    CreatedAt time.Time
    ClosedAt  *time.Time
    URL       string
}

// ProjectItem — add IssueType field
type ProjectItem struct {
    // ... existing fields ...
    IssueType string // GitHub Issue Type; empty for PRs and DraftIssues
}
```

**`internal/github/projectitems.go`** — extend `projectItemsQuery`:

```graphql
... on Issue {
  number title state createdAt
  repository { nameWithOwner }
  labels(first: 20) { nodes { name } }
  issueType { name }   # <-- NEW
}
```

Add `IssueType` to `projectContent` struct:

```go
type projectContent struct {
    // ... existing fields ...
    IssueType *struct {
        Name string `json:"name"`
    } `json:"issueType,omitempty"`
}
```

Wire in `ListProjectItems()`:

```go
if node.Content.IssueType != nil {
    item.IssueType = node.Content.IssueType.Name
}
```

**`internal/github/pullrequests.go`** — extend `gqlIssueNode`:

```go
type gqlIssueNode struct {
    // ... existing fields ...
    IssueType *struct {
        Name string `json:"name"`
    } `json:"issueType"`
}
```

**`internal/github/batch.go`** — extend `fetchIssuesBatch()` query fragment:

```go
// Add issueType { name } to the alias fragment
issue%d: issue(number: %d) {
  number title state createdAt closedAt url
  labels(first: 20) { nodes { name } }
  issueType { name }
}
```

Also extend `fetchPRLinkedIssuesBatch()` in `pullrequests.go` — the `closingIssuesReferences` nodes use `gqlIssueNode`, which now has `IssueType`.

**`internal/github/search.go`** — `searchItemToIssue()` stays unchanged. REST search does not expose issue types; `IssueType` remains empty for REST-sourced issues. No code change needed.

**Conversion functions** — wherever `gqlIssueNode` is converted to `model.Issue` (in `batch.go` and `pullrequests.go`), populate `IssueType`:

```go
if node.IssueType != nil {
    issue.IssueType = node.IssueType.Name
}
```

### Phase 2: Repository-level issue type discovery

New method on `Client` for the `-R` path:

**`internal/github/issuetypes.go`** (new file):

```go
// ListIssueTypes returns the issue types configured on the repository.
// Returns nil (not error) if the repository or GitHub instance does not support issue types.
func (c *Client) ListIssueTypes(ctx context.Context) ([]string, error) {
    query := `query($owner: String!, $repo: String!) {
        repository(owner: $owner, name: $repo) {
            issueTypes(first: 20) {
                nodes { name }
            }
        }
    }`
    variables := map[string]any{
        "owner": c.owner,
        "repo":  c.repo,
    }

    var resp struct {
        Repository struct {
            IssueTypes *struct {
                Nodes []struct {
                    Name string `json:"name"`
                } `json:"nodes"`
            } `json:"issueTypes"`
        } `json:"repository"`
    }

    if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
        // Graceful degradation: old GHE without issueTypes field
        return nil, nil
    }

    if resp.Repository.IssueTypes == nil {
        return nil, nil
    }

    var names []string
    for _, n := range resp.Repository.IssueTypes.Nodes {
        names = append(names, n.Name)
    }
    return names, nil
}
```

### Phase 3: Preflight discovery and probing

**`cmd/preflight.go`** — type-to-category mapping:

```go
// typePatterns maps issue type names to quality categories.
// Used to generate type: probe jobs from discovered issue types.
var typePatterns = map[string][]string{
    "bug":     {"Bug", "Defect"},
    "feature": {"Feature", "Enhancement"},
    "chore":   {"Task", "Chore", "Maintenance"},
}
```

**`cmd/preflight.go`** — update `classifyItem`:

```go
type classifyItem struct {
    number    int
    title     string
    labels    []string
    issueType string // GitHub Issue Type; empty for REST-sourced or PR items
}
```

**`cmd/preflight.go`** — update `probeMatcher()`:

```go
input := classify.Input{
    Labels:    item.labels,
    IssueType: item.issueType,
    Title:     item.title,
}
```

**`cmd/preflight.go`** — concurrent issue type discovery in `runPreflight()`:

After the existing project discovery (step 2) and before match evidence collection (step 5b), add:

```go
// 5a. Discover issue types (concurrent when both paths available)
var discoveredTypes []string

g, gCtx := errgroup.WithContext(ctx)
g.SetLimit(5)

var repoTypes, projectTypes []string

// Path 1: Repo-level issue types (when -R resolves a repo)
g.Go(func() error {
    types, err := client.ListIssueTypes(gCtx)
    if err != nil {
        result.Hints = append(result.Hints, fmt.Sprintf("Could not fetch repo issue types: %v", err))
        return nil // graceful degradation
    }
    repoTypes = types
    return nil
})

// Path 2: Project item issue types (when --project-url given and project discovered)
if result.HasProject {
    g.Go(func() error {
        items, err := client.ListProjectItems(gCtx, projectID, statusFieldID)
        if err != nil {
            result.Hints = append(result.Hints, fmt.Sprintf("Could not fetch project items for type discovery: %v", err))
            return nil
        }
        seen := make(map[string]bool)
        for _, item := range items {
            if item.IssueType != "" && !seen[item.IssueType] {
                seen[item.IssueType] = true
                projectTypes = append(projectTypes, item.IssueType)
            }
        }
        return nil
    })
}

_ = g.Wait()

// Merge and deduplicate
seen := make(map[string]bool)
for _, t := range append(repoTypes, projectTypes...) {
    if !seen[t] {
        seen[t] = true
        discoveredTypes = append(discoveredTypes, t)
    }
}
```

**Note on project items**: The preflight currently does NOT call `ListProjectItems()` — it only calls `DiscoverProjectByNumber()` to get fields/status options. The project item fetch for type discovery needs the project ID and status field ID from the discover step. We need to thread these through. Alternatively, the project items could be fetched with a lightweight query that only requests `issueType` — but the full `ListProjectItems()` call may be useful for future enhancements (e.g., using project items as evidence corpus). For now, reuse `ListProjectItems()`.

**`cmd/preflight.go`** — add `type:` probe jobs in `collectMatchEvidence()`:

Update `collectMatchEvidence` to accept discovered types:

```go
func collectMatchEvidence(categories map[string][]string, discoveredTypes []string, issues []model.Issue, prs []model.PR) []CategoryEvidence {
    // ... existing item conversion (add issueType) ...
    for _, iss := range issues {
        items = append(items, classifyItem{
            number:    iss.Number,
            title:     iss.Title,
            labels:    iss.Labels,
            issueType: iss.IssueType,
        })
    }

    // ... existing probe job building ...

    // Add type: probe jobs from discovered types
    for _, cat := range categoryOrder {
        if patterns, ok := typePatterns[cat]; ok {
            for _, typeName := range discoveredTypes {
                for _, pattern := range patterns {
                    if typeName == pattern {
                        jobs = append(jobs, probeJob{category: cat, matcher: "type:" + typeName})
                    }
                }
            }
        }
    }

    // Also report unmapped types as hints (done in runPreflight, not here)
    // ...
}
```

**`cmd/preflight.go`** — report unmapped types:

```go
// After discovery, check for types that don't map to any category
mapped := make(map[string]bool)
for _, patterns := range typePatterns {
    for _, p := range patterns {
        mapped[p] = true
    }
}
for _, t := range discoveredTypes {
    if !mapped[t] {
        result.Hints = append(result.Hints,
            fmt.Sprintf("Discovered issue type %q with no category mapping — add type:%s to a category if desired", t, t))
    }
}
```

**`cmd/preflight.go`** — update `categoryOrder` and baseline defaults:

```go
// Change from ["bug", "feature", "chore", "docs"] to:
categoryOrder := []string{"bug", "feature", "chore"}
```

Update the fallback in `renderPreflightConfig()` (lines 713-722) to emit three baseline categories:

```go
// Sensible defaults when nothing was detected or matched
b.WriteString("  categories:\n")
b.WriteString("    - name: bug\n")
b.WriteString("      match:\n")
b.WriteString("        - \"label:bug\"\n")
b.WriteString("    - name: feature\n")
b.WriteString("      match:\n")
b.WriteString("        - \"label:enhancement\"\n")
b.WriteString("    - name: chore\n")
b.WriteString("      match:\n")
b.WriteString("        - \"label:chore\"\n")
```

**`cmd/preflight.go`** — `type:` matchers are **peers with labels** (not suggested/fallback):

In `renderPreflightConfig()`, when separating `labelHits` vs `titleHits`, treat `type:` probes as non-suggested (same as labels). The `probeJob` for type matchers should have `suggested: false` (the default). This means `type:` matchers surface alongside labels in the preferred tier.

**`cmd/preflight.go`** — update `PreflightResult`:

```go
type PreflightResult struct {
    // ... existing fields ...
    DiscoveredTypes []string `json:"discovered_types,omitempty"` // issue types found via GraphQL
}
```

**`cmd/preflight.go`** — update `verifyConfig()`:

Add validation for `type:` matchers against discovered types, similar to the label validation at lines 916-933:

```go
// Cross-reference type: matchers against discovered types
if len(result.DiscoveredTypes) > 0 {
    typeSet := make(map[string]bool, len(result.DiscoveredTypes))
    for _, t := range result.DiscoveredTypes {
        typeSet[t] = true
    }
    for _, cat := range cfg.Quality.Categories {
        for _, m := range cat.Matchers {
            prefix, value, ok := strings.Cut(m, ":")
            if ok && prefix == "type" {
                if !typeSet[value] {
                    vr.Warnings = append(vr.Warnings,
                        fmt.Sprintf("issue type %q in %s category not found on repo — will never match", value, cat.Name))
                }
            }
        }
    }
}
```

### Phase 4: Release pipeline wiring

**`internal/metrics/release.go:108-112`** — resolve TODO:

```go
ci := classify.Input{
    Labels:    im.Issue.Labels,
    IssueType: im.Issue.IssueType, // was: "" with TODO
    Title:     im.Issue.Title,
}
```

This works because issues in the release pipeline are fetched via `FetchIssues()` (GraphQL batch) which now requests `issueType { name }`.

## Design Decisions

1. **`type:` matchers as peers with labels, not fallbacks.** Issue Types are explicit semantic classification — more reliable than title regex. They belong in the same precedence tier as labels.

2. **Graceful degradation for `issueType` field.** If the GraphQL field doesn't exist (old GHE), `ListIssueTypes` returns nil. The extended `projectItemsQuery` may fail on very old GHE — this is acceptable since issue types are a modern GitHub feature. If this becomes a real problem, we can add a fallback query without `issueType`.

3. **REST search evidence gap.** Recent issues from `SearchClosedIssues()` (REST) won't have `IssueType` populated. `type:` probes will show 0 evidence matches against these items. This is acceptable — the type was *discovered* as configured, and the `type:` matcher is correct even without evidence. For the `--project-url` path, project items provide richer evidence since they come from GraphQL.

4. **Category order: bug, feature, chore.** Replaces the old `["bug", "feature", "chore", "docs"]`. `docs` was rarely useful. `other` is the implicit catch-all from the classifier — not a named category.

5. **Reuse `ListProjectItems` for project-based type discovery.** This fetches all project items, which is more data than strictly needed for type names. However, it reuses an existing, tested method and avoids adding a specialized lightweight query. The data volume is bounded by project size and cursor pagination.

## System-Wide Impact

### Interaction Graph

- `runPreflight()` → `client.ListIssueTypes()` (new) + `client.ListProjectItems()` (existing, now returns `IssueType`)
- `collectMatchEvidence()` → `probeMatcher()` → `classify.ParseMatcher("type:Bug")` → `TypeMatcher.Matches()` (existing, now receives data)
- `BuildReleaseMetrics()` → `classify.Input{IssueType: issue.IssueType}` (resolves TODO)
- `FetchIssues()` → `fetchIssuesBatch()` → `gqlIssueNode.IssueType` → `model.Issue.IssueType` (new field)

### Error Propagation

- `ListIssueTypes()` GraphQL failure → returns `nil, nil` → no types discovered → no `type:` probes → no regression
- `ListProjectItems()` failure → hint added → type discovery from `-R` path still works (if available)
- Both paths fail → zero discovered types → preflight works exactly as before (labels + titles only)

### State Lifecycle Risks

None. This is read-only discovery — no state is persisted or mutated. Generated YAML is round-trip validated before output.

### API Surface Parity

- `model.Issue.IssueType` added — affects JSON serialization. New field appears in `--format json` output with `omitempty` behavior.
- `model.ProjectItem.IssueType` added — same.
- `PreflightResult.DiscoveredTypes` added — new JSON field.
- No breaking changes to existing API consumers.

## Acceptance Criteria

- [x] `gh velocity config preflight --project-url <url>` discovers and suggests `type:Bug` (etc.) when project issues have types
- [x] `gh velocity config preflight -R owner/repo` discovers and suggests types from repo configuration
- [x] Both flags together run discovery in parallel and merge results
- [x] Repos with no issue types → no `type:` matchers suggested, no errors
- [x] Generated config with `type:` matchers round-trip parses via `config.Parse()` and `classify.NewClassifier()`
- [x] `verifyConfig()` warns on `type:` matchers that reference non-existent types
- [x] Unmapped types (e.g., "Spike") appear as hints
- [x] Baseline categories are bug, feature, chore (not docs)
- [x] `release.go` TODO resolved — `IssueType` populated from GraphQL batch fetch
- [x] `task quality` passes

## Dependencies & Risks

- **GitHub GraphQL schema for `issueTypes`**: Need to verify the exact field name and shape. If `repository.issueTypes` doesn't exist, the `-R` path degrades gracefully.
- **Issue #32 (`-R` resolution)**: This plan works with current repo resolution. When #32 lands, the `-R` path will benefit automatically.
- **GHE compatibility**: Extending `projectItemsQuery` with `issueType { name }` could break on very old GHE instances that don't support the field. Risk is low — issue types are a modern feature and GHE instances that lack it likely don't use them.

## Implementation Order

Files listed in dependency order:

1. `internal/model/types.go` — add `IssueType` to `Issue` and `ProjectItem`
2. `internal/github/pullrequests.go` — add `IssueType` to `gqlIssueNode`
3. `internal/github/batch.go` — add `issueType { name }` to batch query, populate field
4. `internal/github/projectitems.go` — add `issueType { name }` to project items query, populate field
5. `internal/github/issuetypes.go` — new file: `ListIssueTypes()` method
6. `cmd/preflight.go` — type discovery, `typePatterns`, `classifyItem.issueType`, probe jobs, baseline categories, verification
7. `internal/metrics/release.go` — resolve TODO at line 110
8. Tests at each layer

## Test Plan

### Unit tests

- **`internal/github/issuetypes_test.go`** (new): table-driven tests for `ListIssueTypes` — types present, types absent (nil), GraphQL error (graceful nil)
- **`cmd/preflight_test.go`**: extend `TestCollectMatchEvidence` with `type:` probes and `classifyItem` with issueType field; test `typePatterns` mapping; test unmapped type hints; test baseline category defaults (bug/feature/chore)
- **`internal/classify/classify_test.go`**: existing `TestTypeMatcher` and `TestClassifier_Classify` already cover `type:` matching — no changes needed

### Integration / smoke tests

- Run preflight against a repo with issue types configured → verify `type:` suggestions appear in output
- Run preflight against a repo without issue types → verify no regression, no errors
- Run preflight with `--project-url` → verify project-based type discovery

### Round-trip

- Generated config with `type:` matchers parses cleanly (existing `TestRenderPreflightConfig_RoundTrips` should cover this once types flow through)

## Sources

- GitHub issue: https://github.com/dvhthomas/gh-velocity/issues/33
- Related: https://github.com/dvhthomas/gh-velocity/issues/32 (active work on `-R` resolution)
- Existing `TypeMatcher` tests: `internal/classify/classify_test.go:32-52`
- TODO to resolve: `internal/metrics/release.go:110`
- Institutional learning: `docs/solutions/evidence-driven-preflight-config.md` — parallel probing pattern, evidence-driven selection
