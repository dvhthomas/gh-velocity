# Daily Velocity Showcase Workflow

**Date:** 2026-03-15
**Status:** Brainstorm

## What We're Building

A GitHub Actions workflow that runs daily on cron, executing `gh-velocity` against 7+ diverse open-source repos with auto-generated configs. Results are posted to a single Discussion in a "Velocity Reports" category on `dvhthomas/gh-velocity`, with one comment per repo updated in place. This serves as both an end-to-end integration test and a living demo of what gh-velocity can do across different repo shapes.

## Why This Approach

### Shell orchestration over new Go code

The workflow script handles the multi-repo loop and Discussion comment creation via `gh api` GraphQL calls. No changes to the Go codebase are needed to get started. This is YAGNI-friendly — the only consumer is this one workflow.

**Trade-off acknowledged:** This means each user of gh-velocity who wants similar behavior would need to script it themselves. A future "posting configuration" feature (see Future Work) would provide an "easy button" with convention-over-configuration defaults.

### Fresh preflight configs each run

Rather than committing static configs, each run generates fresh configs with `config preflight --write --output`. This ensures the workflow always uses the latest detection logic and catches regressions in config generation. Configs are written to a temp directory within the workflow, not committed.

### One Discussion, comments per repo

A single Discussion is created (or found) per daily run. Each repo's output becomes a comment on that Discussion, updated in place via HTML comment markers. This gives a single URL to share that shows comprehensive output across all repo types.

## Key Decisions

1. **Target repos (7):** cli/cli, kubernetes/kubernetes, hashicorp/terraform, astral-sh/uv, facebook/react, dvhthomas/gh-velocity, microsoft/ebpf-for-windows — plus potentially 1-2 more OSS repos that use GitHub Projects with Status fields
2. **Discussion category:** "Velocity Reports" on dvhthomas/gh-velocity
3. **Orchestration:** Shell script in the workflow (gh api GraphQL for Discussion/comment CRUD)
4. **Config generation:** `config preflight --write` per repo each run (auto-detect, not committed configs)
5. **Comment idempotency:** HTML comment markers (e.g., `<!-- velocity-showcase:cli/cli -->`) to upsert comments per repo
6. **Failure handling:** Actions status only — no auto-created issues
7. **Cadence:** Daily cron
8. **Token:** Uses existing `GH_VELOCITY_TOKEN` secret (classic PAT with project scope) for project board access; `GITHUB_TOKEN` for Discussion writes on the home repo
9. **`GH_VELOCITY_POST_LIVE`:** Set by human in workflow file (per AGENTS.md hard rule)

## Workflow Structure

```
on:
  schedule:
    - cron: "0 8 * * *"   # Daily at 08:00 UTC
  workflow_dispatch:

jobs:
  showcase:
    steps:
      1. Checkout + setup Go + build gh-velocity
      2. Create (or find) today's Discussion in "Velocity Reports"
      3. For each repo:
         a. Run `config preflight --write --output=tmp/<repo>.yml -R <repo>`
         b. Run `gh-velocity report --since 30d --config tmp/<repo>.yml -R <repo> -f markdown`
         c. Run individual commands (lead-time, cycle-time, throughput, velocity, release)
         d. Compose comment: preflight config YAML + report output + individual command outputs
         e. Upsert comment on Discussion with composed output (via gh api GraphQL)
      4. Update Discussion body with summary/index of repos and links to comments
```

## Repos and Their Value

| Repo | Why included | Config shape |
|------|-------------|-------------|
| cli/cli | Large, active, many contributors | Standard issue/PR flow |
| kubernetes/kubernetes | Massive scale, complex release process | High-volume metrics |
| hashicorp/terraform | Mature OSS with structured releases | Release quality metrics |
| astral-sh/uv | Fast-moving Rust project | High throughput, short cycle times |
| facebook/react | Well-known, mixed workflow | Broad community contributions |
| dvhthomas/gh-velocity | Our own repo, small scale | Project board config, dog-fooding |
| microsoft/ebpf-for-windows | Uses GitHub Projects with Status field | Project-board cycle time strategy |
| github/roadmap | GitHub's own public roadmap (orgs/github/projects/4247) | Project board with rich Status field |
| grafana/k6 | Well-known load testing tool (orgs/grafana/projects/443) | Project board showcase |

## Resolved Questions

1. **Discussion lifecycle:** New Discussion each daily run with dated title (e.g., "Velocity Showcase (2026-03-15)"). History is visible as separate Discussions in the category. Each contains comments per repo, updated in place within that day's Discussion.

2. **Commands per repo:** Run both `report` AND individual commands (`flow lead-time`, `flow cycle-time`, `flow throughput`, `flow velocity`, `quality release`, etc.) for maximum granular data. Each command's output becomes a section within the repo's comment.

3. **Include preflight config in output:** Each repo's comment must include the auto-generated config YAML that was used. This makes the Discussion a source of rich debugging data — you can say "see this run's config and figure out how to improve preflight detection."

4. **Preflight for microsoft/ebpf-for-windows:** Try auto-detect first. If it can't find the project board, pass `--project-url` explicitly.

## Resolved: OSS Repos with GitHub Projects

Research identified these candidates with public GitHub Projects v2 boards using Status fields:

| Repo | Project URL | Why |
|------|------------|-----|
| github/roadmap | orgs/github/projects/4247 | GitHub's own public roadmap — canonical example of Projects v2 with Status (Exploring, In Design, In Development, Shipped) |
| microsoft/ebpf-for-windows | orgs/microsoft/projects/2098 | Real engineering board used for day-to-day work tracking |
| withastro/astro | orgs/withastro/projects/11 | Popular web framework, active roadmap board |
| grafana/k6 | orgs/grafana/projects/443 | Well-known load testing tool, public roadmap |

**Recommendation:** Add github/roadmap as the marquee project-board example alongside microsoft/ebpf-for-windows. Both are high-profile and would demonstrate project-board cycle time well. Verify board accessibility via `config preflight` during implementation.

## API Rate Limits and Action Duration

- **GitHub Actions time limit:** 6 hours max per job (default)
- **GitHub API rate:** 5,000 requests/hour for authenticated users; project board queries can be heavier
- **Mitigation:** Run repos sequentially (not parallel) with natural pauses between them. The existing `errgroup.SetLimit(5)` concurrency cap in the Go code handles per-repo rate limiting
- **Caching:** The tool's caching layer should prevent redundant API calls, but large repos (kubernetes, cli/cli) may still be slow
- **Practical concern:** 7-9 repos x (preflight + report + 5 individual commands) = ~50-60 command invocations. At ~30-60 seconds each, this could take 30-60 minutes. Well within Actions limits but worth monitoring

## Open Questions

None — all questions resolved through brainstorming.

## Future Work: Posting Configuration

This brainstorm surfaced a clear future feature need: **posting configuration in `.gh-velocity.yml`** that goes beyond today's `discussions.category`.

Desired capabilities:
- **Target repo:** Post to a different repo than the one being analyzed (e.g., post cli/cli results to dvhthomas/gh-velocity)
- **Discussion category:** Already exists, but should support the target repo context
- **Title template:** Control Discussion title format (e.g., `"Velocity: {{.Repo}} ({{.Date}}"`)
- **Comment mode:** Whether to create a new Discussion, update body, or add/update a comment on an existing Discussion
- **Convention over configuration:** Sensible defaults so `--post` "just works" without extensive config

This would be the "easy button" that replaces shell orchestration for the common case. Not in scope for this iteration.
