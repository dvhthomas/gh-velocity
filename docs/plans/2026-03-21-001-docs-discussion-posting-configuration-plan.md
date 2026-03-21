---
title: "docs: Document discussion posting configuration, title format, and destination"
type: feat
status: completed
date: 2026-03-21
---

# docs: Document discussion posting configuration, title format, and destination

## Overview

The documentation for posting reports to GitHub Discussions is incomplete. An agent (or human) trying to configure a weekly velocity update discussion post cannot find answers to three key questions:

1. **Where does the discussion get created?** (answer: same repo being analyzed, not configurable)
2. **What will the discussion title be?** (answer: `gh-velocity {command}: {owner/repo} ({YYYY-MM-DD})`, hardcoded)
3. **How do I control the discussion category?** (answer: `discussions.category` in config)

The posting guide (`site/content/guides/posting-reports.md`) mentions `discussions.category` but doesn't explain the title format, destination behavior, or how idempotency actually works. The config reference (`site/content/reference/config.md`) documents the field but lacks context about how it interacts with `--post`. The in-repo guide (`docs/guide.md`) shows examples but never explains these mechanics.

Additionally, line 41 of the posting guide says idempotency "matches on title" but the code actually matches on HTML comment markers in the discussion body — this is a factual error.

## Problem Statement / Motivation

An agent tasked with "set up a weekly velocity update discussion post" needs to know:
- Which config fields to set and what values are valid
- What the resulting discussion will look like (title, category, destination)
- How idempotent updates work so it doesn't create duplicates
- What `--post` vs `--new-post` do and how `GH_VELOCITY_POST_LIVE` gates mutations

None of this is documented in one place with enough specificity for an agent to act on.

## Proposed Solution

Update three documentation files with the missing details:

### 1. `site/content/guides/posting-reports.md` — Posting guide (primary fix)

**Add a "Discussion details" section** after "Discussions config" that explains:
- **Destination**: Discussions are created in the repo being analyzed (the `--repo` / `-R` target, or auto-detected from git remote). Cross-repo posting is not supported.
- **Title format**: `gh-velocity {command}: {owner/repo} ({YYYY-MM-DD})` — hardcoded, not configurable. Examples:
  - `gh-velocity report: cli/cli (2026-03-21)`
  - `gh-velocity quality release: dvhthomas/gh-velocity (2026-03-21)`
- **Category requirement**: `discussions.category` must name an existing category in the repo's Discussions settings. The match is case-insensitive. If omitted, `--post` fails with an error.
- **Fix idempotency description** (line 41): Change "matches on title" to accurately describe the HTML-comment-marker mechanism. Explain that each command/context combination gets a unique marker, so `report --since 30d` and `report --since 7d` create separate discussions.

### 2. `site/content/reference/config.md` — Config reference

**Expand `discussions.category` entry** to add:
- That the category must already exist in the repo's Discussions settings
- Case-insensitive matching
- What happens when it's missing (error on `--post`)
- A note that this is the only configurable aspect of discussion posting — title and destination are automatic

### 3. `docs/guide.md` — In-repo guide

**Add a brief "Discussion posting details" note** near the existing `discussions:` config block (around line 421-424) explaining:
- The discussion is created in the analyzed repo
- Title format is automatic
- Category must exist in the repo

## Acceptance Criteria

- [ ] `site/content/guides/posting-reports.md` explains discussion destination (same repo), title format (with examples), and category matching behavior
- [ ] `site/content/guides/posting-reports.md` line 41 idempotency description is corrected from "matches on title" to describe the actual HTML comment marker mechanism
- [ ] `site/content/reference/config.md` `discussions.category` entry explains case-insensitive matching, must-exist requirement, and error behavior
- [ ] `docs/guide.md` has a brief note about discussion destination and title format near the discussions config block
- [ ] No code changes — documentation only
- [ ] An agent reading only the docs could correctly configure and predict the behavior of `gh velocity report --since 30d --post`

## Technical Considerations

- The title format is defined in `internal/posting/poster.go:225` — document it as-is, don't change code
- Idempotency uses `internal/posting/marker.go` HTML comment markers — document the mechanism accurately
- Category resolution is in `internal/github/discussions.go` via `ResolveDiscussionCategoryID()` — case-insensitive match against all repo categories
- No CLI flags exist for title or destination — document their absence explicitly so agents don't search for nonexistent flags

## Files to modify

- `site/content/guides/posting-reports.md`
- `site/content/reference/config.md`
- `docs/guide.md`

## Sources

- `internal/posting/poster.go:225` — title format
- `internal/posting/marker.go` — idempotency markers
- `internal/github/discussions.go` — category resolution, discussion CRUD
- `internal/config/config.go:156-158` — DiscussionsConfig struct
- `cmd/post.go` — posting wiring and dry-run logic
- `cmd/root.go:361-362` — `--post` and `--new-post` flag definitions
