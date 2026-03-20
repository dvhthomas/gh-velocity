---
title: "Rename Recipes to Ad Hoc Queries and fix Velocity Setup title"
type: refactor
status: completed
date: 2026-03-20
---

# Rename Recipes to Ad Hoc Queries and Fix Velocity Setup Title

## Overview

Two documentation titles are inaccurate:
1. **"Recipes"** — the content is ad hoc CLI queries, not recipes. Rename to "Ad Hoc Queries".
2. **"Setting Up Velocity"** — the feature is called "Flow Velocity". Rename to "Setting Up Flow Velocity".

## Proposed Solution

File renames + title updates + fix all inbound references. Inside-out approach: update content first, then rename files, then fix all relrefs.

## Acceptance Criteria

- [ ] `site/content/guides/recipes.md` renamed to `ad-hoc-queries.md` with title "Ad Hoc Queries" and H1 updated
- [ ] `site/content/guides/velocity-setup.md` title and H1 changed to "Setting Up Flow Velocity" (filename stays — "velocity-setup" is still accurate as a slug)
- [ ] All inbound relref links updated (6 total across 5 files)
- [ ] Hugo site builds without broken links (`hugo --minify` succeeds)
- [ ] Self-reference text inside the renamed file updated

## Changes

### 1. Rename Recipes → Ad Hoc Queries

**Rename file:** `site/content/guides/recipes.md` → `site/content/guides/ad-hoc-queries.md`

**In `ad-hoc-queries.md`:**
- Front matter `title: "Recipes"` → `title: "Ad Hoc Queries"`
- H1 `# How-To Recipes` → `# Ad Hoc Queries`
- Line ~190: update self-referencing text "automate these recipes" → "automate these queries"

**Update relrefs (3 files):**
- `site/content/guides/posting-reports.md` line 156: relref `"recipes"` → `"ad-hoc-queries"`, link text → "Ad Hoc Queries"
- `site/content/guides/agent-integration.md` line 223: same
- `site/content/guides/interpreting-results.md` line 263: same

### 2. Fix "Setting Up Velocity" → "Setting Up Flow Velocity"

**In `site/content/guides/velocity-setup.md`:**
- Front matter `title: "Setting Up Velocity"` → `title: "Setting Up Flow Velocity"`
- H1 `# Setting Up Velocity` → `# Setting Up Flow Velocity`

**Update link text (4 files, 5 occurrences):**
- `site/content/reference/config.md` line 609: link text → "Setting Up Flow Velocity"
- `site/content/reference/metrics/velocity.md` line 235: link text → "Setting Up Flow Velocity"
- `site/content/guides/cycle-time-setup.md` line 169: link text → "Flow Velocity Setup"
- `site/content/getting-started/configuration.md` lines 176, 242: link text → "Flow Velocity Setup"

### 3. Verify

- Run `hugo --minify` in `site/` to confirm no broken links
- Grep for stale references to "recipes" and "Setting Up Velocity"

## Sources

- Research: all inbound references identified via grep across `site/content/`
- Learnings: inside-out rename strategy from `docs/solutions/architecture-refactors/cobra-command-hierarchy-thematic-grouping.md`
