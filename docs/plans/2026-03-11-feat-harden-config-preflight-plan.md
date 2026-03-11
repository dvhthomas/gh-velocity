---
title: "feat: Harden config preflight with round-trip validation and dry-run verification"
type: feat
status: completed
date: 2026-03-11
---

# feat: Harden Config Preflight

## Overview

Make `config preflight` trustworthy: every config it generates should parse cleanly, classify labels correctly, and work when the user immediately runs a command. Three changes: round-trip YAML validation, tighter label heuristics with word-boundary matching, and a dry-run verification step that proves the suggested config works.

## Problem Statement

Users run `gh velocity config preflight --write` and trust the output. Today that trust is misplaced:

1. **Broken YAML**: The `posting:` block in generated YAML is not a valid config key — loading it triggers `config: unknown key "posting" (ignored)`. The tool generates config it would warn about.
2. **False-positive labels**: `classifyLabels` uses `strings.Contains` substring matching. "debugging" → bug bucket, "defeat" → feature bucket, "proactive" → active bucket, "translator" → backlog bucket. These wrong guesses propagate into the config.
3. **No proof it works**: A user who runs `--write` then `report --since 30d` may get errors if the suggested strategy or labels are wrong. Preflight doesn't verify its own output.

## Proposed Solution

### Phase 1: Extract `config.Parse([]byte)` and fix the posting block

**Goal**: Enable round-trip validation without temp files. Fix the confirmed bug.

- [x] **`internal/config/config.go`**: Extract parse+validate logic from `Load()` into `Parse(data []byte) (*Config, error)`
  - `Load()` becomes: read file → `Parse(data)`
  - `Parse()` does: unmarshal raw map (unknown keys) → unmarshal typed struct → `validate()` → `resolveCategories()`
  - Accept an optional `WarnFunc` override or temporarily suppress warnings during internal round-trip calls
- [x] **`internal/config/config_test.go`**: Add tests for `Parse()` — same table-driven patterns as existing `Load()` tests
- [x] **`cmd/preflight.go`**: Move the `posting:` information to YAML comments instead of a config block
  ```yaml
  # Posting readiness:
  #   discussions: enabled
  #   issues: accessible
  ```
- [x] **`cmd/preflight.go`**: After `renderPreflightConfig()`, round-trip validate:
  ```go
  yaml := renderPreflightConfig(result)
  if _, err := config.Parse([]byte(yaml)); err != nil {
      // This is a bug in preflight, not the user's fault
      return fmt.Errorf("preflight generated invalid config (please report this): %w", err)
  }
  ```
**Tests**: Round-trip `renderPreflightConfig()` output through `config.Parse()`. Verify no unknown key warnings. Test `Parse()` with the same table-driven patterns as existing `Load()` tests.

### Phase 2: Tighten label heuristics with word-boundary matching

**Goal**: Eliminate false positives while catching real labels.

- [x] **`cmd/preflight.go`**: Replace `matchesAny` with `matchesWord` that uses word-boundary logic
  ```go
  // matchesWord returns true if pattern appears in label at a word boundary.
  // Word boundaries: start/end of string, hyphen, space, underscore, slash, colon.
  // "bug" matches "bug", "bug-report", "type:bug" but NOT "debugging".
  func matchesWord(label, pattern string) bool
  ```
- [x] **Expand pattern sets** with commonly-used GitHub label names:
  ```go
  bugPatterns     := []string{"bug", "defect", "regression", "crash"}
  featurePatterns := []string{"enhancement", "feature", "improvement"}
  chorePatterns   := []string{"chore", "maintenance", "housekeeping", "cleanup", "tech-debt", "refactor"}
  docsPatterns    := []string{"documentation", "docs"}
  activePatterns  := []string{"in-progress", "in progress", "wip"}
  backlogPatterns := []string{"backlog", "icebox", "deferred", "wishlist"}
  ```
  - Remove "error" (too many false positives: "error-handling", "user-error")
  - Remove "feat" (matches "defeat", "feather" — "feature" is sufficient)
  - Remove "working" (matches "networking", "not-working")
  - Remove "active" (matches "proactive", "interactive", "reactive")
  - Remove "later" (matches "translator", "collateral")
  - Remove "someday" and "doing" (uncommon, ambiguous)
  - Add "chore" category patterns — very common in OSS repos
  - Add "docs" category patterns
  - Defer test/ci/security/performance categories — uncommon as labels, can add later if needed
- [x] **Replace typed label fields on `PreflightResult`** with a categories map:
  ```go
  // Replace BugLabels, FeatureLabels with:
  Categories map[string][]string `json:"categories"` // e.g. {"bug": ["bug", "defect"], "feature": ["enhancement"]}
  ```
  Keep `ActiveLabels` and `BacklogLabels` — these are status signals, not quality categories.
- [x] **Generate modern `categories` format** in `renderPreflightConfig` instead of legacy `bug_labels`/`feature_labels`:
  ```yaml
  quality:
    categories:
      - name: bug
        match: ["label:bug", "label:defect"]
      - name: feature
        match: ["label:enhancement", "label:feature"]
      - name: chore
        match: ["label:chore", "label:maintenance"]
  ```
  This skips the legacy-to-categories auto-generation hop and gives users the richer matcher syntax from the start. Only emit categories that have at least one matched label.
- [x] **First-match-wins for category assignment**: if a label like "bug-fix" matches both bug and chore, assign to the first matching category only. Add a YAML comment noting the ambiguity for labels that matched multiple patterns.

**Tests**: Table-driven tests for `matchesWord`:

| Label | Pattern | Expected |
|---|---|---|
| `"bug"` | `"bug"` | true |
| `"bug-report"` | `"bug"` | true |
| `"type:bug"` | `"bug"` | true |
| `"debugging"` | `"bug"` | false |
| `"feature"` | `"feature"` | true |
| `"feature-request"` | `"feature"` | true |
| `"defeat"` | `"feat"` | false (pattern removed) |
| `"in-progress"` | `"in-progress"` | true |
| `"proactive"` | `"active"` | false (pattern removed) |
| `"chore"` | `"chore"` | true |
| `"chores-daily"` | `"chore"` | true |

### Phase 3: Dry-run verification

**Goal**: Prove the generated config works against the target repo without duplicate API calls.

The dry-run does NOT re-run `report`. Instead it validates the generated config can produce a working pipeline:

- [x] **Parse the generated YAML** through `config.Parse()` (already done in Phase 1)
- [x] **Construct a `classify.NewClassifier(cfg.Quality.Categories)`** to verify all matchers compile and are valid
- [x] **Validate strategy prerequisites**: if `cycle_time.strategy: project-board`, verify `project.id` and `status_field_id` are present. If `strategy: pr`, verify API access worked (preflight already fetched PRs).
- [x] **Cross-reference detected labels against actual repo labels**: check that every label in `bug_labels`/`feature_labels`/categories actually exists in the repo's label set (preflight already fetched these). If a suggested label doesn't exist on the repo, warn.
- [x] **Produce a verification summary** (added to hints):
  ```
  Config verification:
    ✓ YAML parses cleanly
    ✓ 3 categories defined (bug: 2 matchers, feature: 1 matcher, chore: 1 matcher)
    ✓ Cycle time strategy: issue (no project board required)
    ✗ Label "regression" in bug category not found on repo — will never match
  ```
- [x] **Add verification to `PreflightResult` JSON**:
  ```go
  type PreflightResult struct {
      // ... existing fields ...
      Verification *VerificationResult `json:"verification,omitempty"`
  }
  type VerificationResult struct {
      Valid          bool     `json:"valid"`
      ConfigParses   bool     `json:"config_parses"`
      MatchersValid  bool     `json:"matchers_valid"`
      CategoryCount  int      `json:"category_count"`
      MissingLabels  []string `json:"missing_labels,omitempty"`
      Warnings       []string `json:"warnings,omitempty"`
  }
  ```

**Why not run the actual report?** The report command requires `Deps` (which preflight skips via `PersistentPreRunE` bypass), makes duplicate API calls, and produces output that would pollute preflight stdout. The config-level verification catches the same classes of errors without the complexity.

**Tests**: Verify that a config with a nonexistent label produces a warning. Verify that a valid config produces `Valid: true`. Also add tests for `writeStatusMapping`, `findStatus`, and `countLabelUsage` (all currently untested).

## Technical Considerations

- **`config.Parse` warning suppression**: During round-trip validation inside preflight, `config.WarnFunc` should be temporarily swapped to a collector that captures warnings as structured data (not printed to stderr). Restore it after validation.
- **Backward compatibility**: Existing users of `config preflight -f json` will get new `verification` field. This is additive (omitempty), not breaking.
- **The `posting:` section in pretty output moves to comments**: Users still see the information, but it won't interfere with config loading.

## Acceptance Criteria

- [x] `renderPreflightConfig()` output round-trips through `config.Parse()` without errors or warnings
- [x] `matchesWord("debugging", "bug")` returns false
- [x] `matchesWord("bug-report", "bug")` returns true
- [x] Preflight generates modern `categories` format with `label:` matchers
- [x] Preflight JSON includes `verification` field showing parse and matcher validation
- [x] Missing repo labels produce a warning in verification output
- [ ] `--write` followed by `report --since 30d` works with no config warnings
- [x] Zero false positives from the documented test cases (debugging, defeat, proactive, translator, networking)
- [x] `task quality` passes (all tests, lint, staticcheck, smoke tests)

## Dependencies & Risks

- **Risk**: Changing label patterns may produce different preflight output for existing users. This is intentional — the old output had false positives. Not a breaking change since preflight is advisory.
- **Risk**: Generating `categories` instead of `bug_labels`/`feature_labels` changes the YAML format. The old format still works via `resolveCategories()`, so both are valid.
- **Dependency**: Phase 1 (`config.Parse`) must complete before Phase 3 (verification uses it).

## References

- `cmd/preflight.go:253-284` — current `classifyLabels` with loose substring matching
- `cmd/preflight.go:309-404` — `renderPreflightConfig` generates unvalidated YAML
- `cmd/preflight.go:375-389` — `posting:` block (confirmed bug: not a valid config key)
- `internal/config/config.go:102-141` — `Load()` (file-only, no `[]byte` variant)
- `internal/config/config.go:173-182` — `knownTopLevelKeys` (no "posting" entry)
- `internal/config/config.go:194-261` — `validate()` function
- `internal/classify/classify.go:121-128` — `LabelMatcher` uses `EqualFold` (exact match)
- `internal/classify/classify.go:178-198` — `FromLegacyLabels()` bridge
- `docs/solutions/cycle-time-signal-hierarchy.md` — case sensitivity gotcha in label matching
