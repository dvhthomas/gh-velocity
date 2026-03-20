---
title: "feat: Configurable discussion titles with title-based dedup"
type: feat
status: active
date: 2026-03-20
origin: docs/brainstorms/2026-03-20-discussion-title-dedup-requirements.md
---

# feat: Configurable discussion titles with title-based dedup

## Overview

Switch Discussion posting from body-marker deduplication to title-based deduplication, with a configurable title template that supports Go `time.Format` placeholders. This gives users explicit control over when a new Discussion is created vs. updated, and enables multiple commands to share a single Discussion.

Conceptual identity model: **system + channel + title** — for Discussions this is `github + category + rendered_title`. This framing generalizes to future posting targets (see origin: `docs/brainstorms/2026-03-20-discussion-title-dedup-requirements.md`).

## Problem Statement / Motivation

Currently the Discussion title is hardcoded and dedup uses body markers (`<!-- gh-velocity:command:context -->`). This means:
1. Changing `--since 30d` to `--since 60d` silently creates a **new** discussion (different marker context)
2. Users cannot control discussion naming
3. Multiple commands always get separate discussions (no way to share)

The title should be the user-visible, controllable dedup key. Markers continue to handle section-level identity within a discussion body.

## Proposed Solution

Two-layer dedup: **title** identifies which Discussion to create/update; **markers** identify which section within that Discussion's body to replace.

### Config

Add `title` to `DiscussionsConfig`:

```yaml
discussions:
  category: General
  title: "Velocity Update {{2006-01-02}}"
```

`{{ }}` delimiters contain Go reference-time layouts passed to `time.Now().UTC().Format()`. Everything outside `{{ }}` is literal text.

### Title rendering

New function `RenderTitle(template string) string` in `internal/posting/`:
- Find all `{{...}}` placeholders
- Replace each with `time.Now().UTC().Format(contents)`
- Return the rendered string

Companion `ValidateTitleTemplate(template string) error` for config-time validation — attempts a render and checks for empty result.

### Discussion dedup flow

1. Render the title template
2. Search discussions in category (existing `SearchDiscussions`)
3. **Match on `d.Title == renderedTitle`** (not body marker)
4. If found: read existing body, call `InjectMarkedSection(existingBody, command, context, wrappedContent)`, update body
5. If not found: create new discussion with rendered title and wrapped content as body
6. `--new-post` skips search entirely (unchanged)

### Default title (backwards compat)

When `discussions.title` is not set, the default is assembled **in code** (not via the template engine):

```go
fmt.Sprintf("gh-velocity %s: %s (%s)", opts.Command, opts.Repo, time.Now().UTC().Format("2006-01-02"))
```

This matches the current hardcoded behavior exactly. Commands get separate discussions by default.

## Technical Considerations

### Update is partial, not full replacement

Current `UpdateDiscussion` replaces the entire body. The new flow must:
1. Read the existing discussion body (already available from `SearchDiscussions` — `d.Body`)
2. Call `InjectMarkedSection(d.Body, command, context, wrappedContent)`
3. Pass the result to `UpdateDiscussion`

This preserves human edits outside the markers (see origin: R3, R4).

### Search window

`SearchDiscussions` fetches 50 most-recently-updated discussions. Title matching within this window is sufficient for typical usage. If a discussion falls outside the 50 most recent, a new one is created — this is acceptable and consistent with the existing design. No GraphQL title-search API exists.

### Template validation

Invalid placeholders like `{{not-a-date}}` get passed to `time.Format()` which does not error — it returns the string with reference-time characters substituted. Validation checks:
- Template is non-empty after rendering
- Template does not exceed a reasonable length (256 chars)
- No nested `{{ }}` or unclosed delimiters

### Race conditions

Two concurrent CI runs could both fail to find a matching title and create duplicate discussions. This is a known, accepted limitation from the original posting design (last-write-wins on retry).

## Acceptance Criteria

- [ ] `discussions.title` config field parsed and validated
- [ ] `RenderTitle` correctly substitutes `{{ }}` with `time.Format` output
- [ ] Same rendered title → updates existing discussion (not duplicate)
- [ ] Different rendered title → creates new discussion
- [ ] Update preserves body content outside markers (`InjectMarkedSection`)
- [ ] Multiple commands sharing a title → each gets its own marked section
- [ ] `--new-post` always creates (bypass title match)
- [ ] Default (no `discussions.title`) matches current hardcoded format
- [ ] `config show` displays `discussions.title`
- [ ] Preflight generated config includes `discussions.title` example
- [ ] `defaultConfigTemplate` updated
- [ ] Never delete a discussion

## Implementation Phases

### Phase 1: Title template rendering + validation

**Files:**
- `internal/posting/title.go` (new) — `RenderTitle`, `ValidateTitleTemplate`
- `internal/posting/title_test.go` (new) — table-driven tests

```go
// RenderTitle replaces {{layout}} placeholders with time.Now().UTC().Format(layout).
func RenderTitle(template string) string

// ValidateTitleTemplate checks that a template renders to a non-empty string
// and has balanced {{ }} delimiters.
func ValidateTitleTemplate(template string) error
```

Test cases:
- `"Velocity Update {{2006-01-02}}"` → `"Velocity Update 2026-03-20"`
- `"Weekly {{Jan 2}}"` → `"Weekly Mar 20"`
- `"No placeholders"` → `"No placeholders"`
- `""` → error (empty)
- `"Unclosed {{2006-01-02"` → error (unbalanced)

### Phase 2: Config field + validation

**Files:**
- `internal/config/config.go` — add `Title string` to `DiscussionsConfig`, validate in `Validate()`
- `internal/config/config_test.go` — test parsing and validation

```go
type DiscussionsConfig struct {
    Category string `yaml:"category" json:"category"`
    Title    string `yaml:"title" json:"title"`
}
```

Validation: if `Title` is set, call `posting.ValidateTitleTemplate(Title)`.

### Phase 3: DiscussionPoster title-based dedup

**Files:**
- `internal/posting/poster.go` — rewrite `DiscussionPoster.Post()`
- `internal/posting/poster_test.go` — new/updated tests

Changes to `PostOptions`:
```go
type PostOptions struct {
    // ... existing fields ...
    Title string // rendered discussion title (set by caller)
}
```

New `DiscussionPoster.Post()` flow:
1. Build wrapped content: `WrapWithMarker(command, context, content)`
2. Use `opts.Title` (caller renders it; empty = default format in code)
3. If `ForceNew`: create with title + wrapped body, return
4. Search discussions, match on `d.Title == title`
5. If match: `newBody := InjectMarkedSection(d.Body, command, context, wrappedContent)` → update
6. If no match: create with title + wrapped body

Test cases:
- Title match → update with `InjectMarkedSection` (body preserved outside markers)
- No title match → create new
- Two commands, same title → each section injected independently
- `ForceNew` → always create regardless of title match
- Missing `CategoryID` → error
- Dry-run variants of all the above

### Phase 4: Wire config to caller + update touchpoints

**Files:**
- `cmd/post.go` — render title from config (or default), set `opts.Title`
- `cmd/config.go` — update `config show` output (line 89 area) and `defaultConfigTemplate` (line 271 area)
- `cmd/preflight.go` — update generated config snippet (line 1207 area) to include `title` example

In `setupPost()`:
```go
case posting.DiscussionTarget:
    titleTemplate := deps.Config.Discussions.Title
    if titleTemplate != "" {
        opts.Title = posting.RenderTitle(titleTemplate)
    }
    // else opts.Title stays empty → poster uses default format
```

In `DiscussionPoster.Post()`:
```go
title := opts.Title
if title == "" {
    title = fmt.Sprintf("gh-velocity %s: %s (%s)", opts.Command, opts.Repo, time.Now().UTC().Format("2006-01-02"))
}
```

Config show addition:
```
discussions.title:           Velocity Update {{2006-01-02}}
```

Default config template update:
```yaml
discussions:
  category: General
  # title: "Velocity Update {{2006-01-02}}"
```

Preflight generated config:
```yaml
discussions:
  category: General
  # title: "Velocity Update {{2006-01-02}}"
```

## Sources & References

### Origin

- **Origin document:** [docs/brainstorms/2026-03-20-discussion-title-dedup-requirements.md](docs/brainstorms/2026-03-20-discussion-title-dedup-requirements.md) — Key decisions: title is dedup key for discussions; Go `time.Format` for templates; default preserves current behavior; update uses `InjectMarkedSection`; never delete discussions.

### Internal References

- Discussion poster: `internal/posting/poster.go:206-283`
- Marker system: `internal/posting/marker.go`
- Config struct: `internal/config/config.go:147-149`
- Config validation: `internal/config/config.go:347-352`
- Config show: `cmd/config.go:89`
- Default template: `cmd/config.go:267-273`
- Preflight generated config: `cmd/preflight.go:1203-1209`
- Post wiring: `cmd/post.go:37-85`
