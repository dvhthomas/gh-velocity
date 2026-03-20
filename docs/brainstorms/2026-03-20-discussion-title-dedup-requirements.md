---
date: 2026-03-20
topic: discussion-title-dedup
---

# Configurable Discussion Titles with Title-Based Deduplication

## Problem Frame

When `--post` targets a Discussion, the title is hardcoded and deduplication uses body markers. This creates two problems: (1) users cannot control how discussions are named, and (2) changing command parameters (e.g., `--since 30d` to `--since 60d`) silently creates a new discussion because the marker context changed — even though the user intended to update the same post. The title should be the dedup key for discussions, giving users explicit control over when a new discussion is created vs. when an existing one is updated.

## Requirements

- R1. `discussions.title` config field accepts a template string for Discussion titles. Placeholders in `{{ }}` are passed to Go's `time.Format()` using the current UTC time. Example: `"Velocity Update {{2006-01-02}}"` renders to `"Velocity Update 2026-03-20"`.
- R2. When no discussion with the rendered title exists in the configured category, create a new discussion with that title.
- R3. When a discussion with the rendered title already exists, update its body — specifically, replace only the marked section (via `InjectMarkedSection`), preserving all content outside the markers. Never replace the entire body.
- R4. Never delete a discussion. Human comments and edits must be preserved.
- R5. Multiple commands can share one discussion if they use the same `discussions.title`. Each command's output occupies its own marked section in the body. If commands use different title templates, they get separate discussions.
- R6. When `discussions.title` is not set, default to the current format: `"gh-velocity {command}: {owner/repo} ({{2006-01-02}})"`. This preserves backwards compatibility and means commands get separate discussions by default.
- R7. `--new-post` bypasses title matching and always creates a new discussion (existing behavior preserved).

## Success Criteria

- Running the same command with `--post` twice against the same rendered title updates the existing discussion, not creating a duplicate.
- Changing `--since` or other parameters does not create a new discussion if the title template renders to the same value.
- Human-written content in the discussion body (outside markers) survives updates.
- Discussion comments (replies) are untouched.

## Scope Boundaries

- No changes to comment or issue-body posting — title dedup is Discussion-only.
- No template variables beyond `time.Format` placeholders (no `{command}`, `{repo}` interpolation beyond what Go time layouts provide). The default title is constructed in code, not via the template engine.
- No discussion deletion, archiving, or locking.

## Key Decisions

- **Title is the dedup key for discussions**: Replaces body-marker matching for discussion-level identity. Markers still handle section-level identity within the body.
- **Go time.Format for templates**: `{{ }}` delimiters with Go reference-time layouts. No strftime, no external template library.
- **Default preserves current behavior**: Unset `discussions.title` falls back to the existing hardcoded format, so this is non-breaking.
- **Update uses InjectMarkedSection**: Partial body update instead of full replacement, protecting human edits.

## Outstanding Questions

### Deferred to Planning

- [Affects R1][Technical] How should invalid template placeholders (e.g., `{{not-a-date}}`) be handled? Validation at config load vs. runtime error.
- [Affects R2][Technical] Current `SearchDiscussions` fetches 50 recent discussions by category. Title matching needs to work within that window — is 50 sufficient, or should we search by title via the GraphQL API?
- [Affects R6][Technical] The default title includes `{command}` and `{owner/repo}` which are not time.Format placeholders. Confirm the default is assembled in code (not passed through the template engine) to avoid confusion.

## Next Steps

-> `/ce:plan` for structured implementation planning
