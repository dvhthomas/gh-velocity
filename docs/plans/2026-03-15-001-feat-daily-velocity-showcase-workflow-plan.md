---
title: "feat: Daily velocity showcase workflow"
type: feat
status: completed
date: 2026-03-15
origin: docs/brainstorms/2026-03-15-daily-velocity-showcase-brainstorm.md
---

# feat: Daily Velocity Showcase Workflow

## Overview

A GitHub Actions workflow that runs daily on cron, executing `gh-velocity` against 9 diverse OSS repos with auto-generated configs. Each run creates a new Discussion in a "Velocity Reports" category on `dvhthomas/gh-velocity`, with one comment per repo containing the generated config, composite report, and individual command outputs. This serves as both a living e2e integration test and a showcase of gh-velocity's capabilities across different repo shapes.

(see brainstorm: `docs/brainstorms/2026-03-15-daily-velocity-showcase-brainstorm.md`)

## Problem Statement / Motivation

Today, gh-velocity has unit tests and smoke tests, but no continuous, real-world validation against diverse repos. The existing `velocity.yaml` workflow only runs against `dvhthomas/gh-velocity` with `--new-post`. There's no way to point someone at a URL and say "look at what gh-velocity produces across many repo shapes." A daily showcase solves both problems: it catches regressions in preflight config generation and command output, and it produces a browsable archive of real metric reports.

## Proposed Solution

Shell orchestration in a GitHub Actions workflow. No Go code changes required.

The workflow:
1. Builds gh-velocity from source
2. Creates a new Discussion titled "Velocity Showcase (YYYY-MM-DD)" via `gh api graphql`
3. Loops over 9 repos sequentially, for each:
   - Generates config via `config preflight --write=tmp/<slug>.yml -R <owner/repo>`
   - Runs `report` and individual `flow` commands, capturing markdown output
   - Composes a comment body (config YAML + all outputs)
   - Posts as a discussion comment via `gh api graphql` `addDiscussionComment`
4. Updates the Discussion body with an index of repos and their status

(see brainstorm: shell orchestration over new Go code)

## Technical Considerations

### Token Strategy

A **single classic PAT** (`GH_VELOCITY_TOKEN`) with scopes `repo`, `read:project`, `write:discussion` handles all operations. This token is already configured as a repository secret. It is used as `GH_TOKEN` for both reading external repos (including project boards) and writing Discussions on dvhthomas/gh-velocity.

The default `GITHUB_TOKEN` is NOT sufficient because:
- It cannot read project boards on external orgs
- It is scoped to the workflow's repository only for write operations

### Preflight Invocation

**Critical correction from SpecFlow:** The CLI flag is `--write=<path>`, NOT `--write --output=<path>`. There is no `--output` flag. Correct syntax:

```bash
./gh-velocity config preflight --write=tmp/cli-cli.yml -R cli/cli
```

For repos needing explicit project board URLs:
```bash
./gh-velocity config preflight --write=tmp/microsoft-ebpf-for-windows.yml \
  -R microsoft/ebpf-for-windows \
  --project-url https://github.com/orgs/microsoft/projects/2098
```

### Discussion Node ID Flow

The `createDiscussion` GraphQL mutation returns a `discussion` object. The shell script must capture the `id` (node ID) from the response, not just the `url`. This node ID is then passed to each `addDiscussionComment` call.

```graphql
mutation {
  createDiscussion(input: {
    repositoryId: $repoId,
    categoryId: $categoryId,
    title: $title,
    body: $body
  }) {
    discussion {
      id      # <-- needed for addDiscussionComment
      url
    }
  }
}
```

### Commands Per Repo

Run `report` (composite) AND individual commands. While `report` already includes lead-time, cycle-time, throughput, and velocity, the individual commands provide more detailed per-metric output. This is intentional for the showcase — it exercises more code paths and produces richer debugging data. (see brainstorm: resolved question #2)

Individual commands to run per repo:
- `flow lead-time --since 30d`
- `flow cycle-time --since 30d`
- `flow throughput --since 30d`
- `flow velocity --since 30d` (may fail if no iteration strategy — handled gracefully)
- `quality release` (needs tag detection — skip if no recent tags)

All commands receive `--config tmp/<slug>.yml -R <owner/repo> -f markdown`.

### Comment Markdown Structure

Each repo's comment uses this template:

```markdown
## <owner/repo>

**Generated:** YYYY-MM-DD HH:MM UTC
**Config:** auto-generated via `config preflight`

<details>
<summary>Generated Config (.gh-velocity.yml)</summary>

\`\`\`yaml
# contents of tmp/<slug>.yml
\`\`\`

</details>

### Composite Report

<output from `report --since 30d -f markdown`>

<details>
<summary>Lead Time</summary>

<output from `flow lead-time --since 30d -f markdown`>

</details>

<details>
<summary>Cycle Time</summary>

<output from `flow cycle-time --since 30d -f markdown`>

</details>

<details>
<summary>Throughput</summary>

<output from `flow throughput --since 30d -f markdown`>

</details>

<details>
<summary>Velocity</summary>

<output from `flow velocity --since 30d -f markdown` or "Not configured: no iteration strategy detected">

</details>
```

### Comment Size Limits

GitHub Discussion comments have a 65,536 character limit. For very active repos (kubernetes), output could be large. Mitigation: if composed comment exceeds 60,000 chars, truncate individual command details and note the truncation.

### API Budget Estimate

Per repo (approximate):
- Preflight: ~5-8 REST calls (labels, PRs, issues, project board check)
- Report: ~10-20 search/GraphQL calls (issues, PRs, timelines, cycle time)
- Individual commands: ~15-30 calls (overlapping data, but no shared cache between invocations)
- Total per repo: ~30-60 calls

Across 9 repos: ~270-540 REST calls, plus GraphQL calls for project boards and discussion operations.

With 5,000 REST calls/hour and `api_throttle_seconds: 2` in generated configs, this is feasible but will take 20-60 minutes of wall-clock time. Well within the 6-hour Actions limit.

**High-risk repos:** kubernetes/kubernetes may consume 100+ calls alone due to high issue/PR volume. Consider using `--since 14d` instead of 30d for this repo if rate limits become an issue.

## System-Wide Impact

- **No Go code changes.** All orchestration is in the workflow YAML and inline shell.
- **No changes to existing commands.** Uses gh-velocity as a black-box CLI.
- **Discussion category:** "Velocity Reports" must be created manually on dvhthomas/gh-velocity before first run (GitHub API does not support category creation).
- **Existing `velocity.yaml` workflow:** Remains unchanged. The new workflow is additive.
- **Token:** Uses the existing `GH_VELOCITY_TOKEN` secret, which already has appropriate scopes.

## Target Repos

| # | Repo | Slug | Project Board | Notes |
|---|------|------|--------------|-------|
| 1 | cli/cli | cli-cli | No | Large, active, standard issue/PR flow |
| 2 | kubernetes/kubernetes | kubernetes-kubernetes | No | Massive scale, may hit API limits |
| 3 | hashicorp/terraform | hashicorp-terraform | No | Mature OSS, structured releases |
| 4 | astral-sh/uv | astral-sh-uv | No | Fast-moving Rust project |
| 5 | facebook/react | facebook-react | No | Well-known, mixed workflow |
| 6 | dvhthomas/gh-velocity | dvhthomas-gh-velocity | Yes (local) | Dog-fooding, project board config |
| 7 | microsoft/ebpf-for-windows | microsoft-ebpf-for-windows | Yes (`--project-url`) | Project board cycle time showcase |
| 8 | github/roadmap | github-roadmap | Yes (roadmap only) | Issue-only repo, no PRs/releases |
| 9 | grafana/k6 | grafana-k6 | Yes | Load testing tool, public roadmap |

**Note:** github/roadmap is a non-code repo (issues only, no PRs or releases). Commands like `cycle-time`, `throughput`, and `quality release` will produce empty or error results. This is expected and useful — it shows how gh-velocity handles edge-case repos.

## Implementation Phases

### Phase 1: Prerequisites (Manual)

- [ ] Create "Velocity Reports" Discussion category on dvhthomas/gh-velocity via Settings > Discussions > Categories
- [ ] Verify `GH_VELOCITY_TOKEN` secret has scopes: `repo`, `read:project`, `write:discussion`
- [ ] Verify microsoft/ebpf-for-windows project board URL (orgs/microsoft/projects/2098) is accessible

### Phase 2: Workflow Script

**File:** `.github/workflows/showcase.yaml`

- [x] Workflow triggers: `schedule` (daily cron `0 6 * * *` / 06:00 UTC) + `workflow_dispatch`
- [x] Permissions: `contents: read`, `pull-requests: read`, `discussions: write`
- [x] Steps:
  1. `actions/checkout@v6`
  2. `actions/setup-go@v6` with `go-version-file: go.mod`
  3. `go build -o gh-velocity .`
  4. Shell script: `scripts/showcase.sh`

**Shell orchestration logic:**

```bash
# .github/workflows/showcase.yaml (run: block)

set -euo pipefail

# ── Config ──────────────────────────────────────────────────────
SHOWCASE_DATE=$(date -u +%Y-%m-%d)
SHOWCASE_REPO="dvhthomas/gh-velocity"
DISCUSSION_CATEGORY="Velocity Reports"
BINARY="./gh-velocity"
TMP_DIR="tmp/showcase"
SINCE="30d"

# Repo definitions: slug|owner/repo|extra_preflight_flags
REPOS=(
  "cli-cli|cli/cli|"
  "kubernetes-kubernetes|kubernetes/kubernetes|"
  "hashicorp-terraform|hashicorp/terraform|"
  "astral-sh-uv|astral-sh/uv|"
  "facebook-react|facebook/react|"
  "dvhthomas-gh-velocity|dvhthomas/gh-velocity|"
  "microsoft-ebpf-for-windows|microsoft/ebpf-for-windows|--project-url https://github.com/orgs/microsoft/projects/2098"
  "github-roadmap|github/roadmap|"
  "grafana-k6|grafana/k6|"
)

# ── Clean tmp ───────────────────────────────────────────────────
rm -rf "$TMP_DIR"
mkdir -p "$TMP_DIR"

# ── Resolve repo and category IDs ──────────────────────────────
REPO_NODE_ID=$(gh api graphql -f query='...' | jq -r '.data.repository.id')
CATEGORY_ID=$(gh api graphql -f query='...' | jq -r '...')

# ── Create Discussion ──────────────────────────────────────────
TITLE="Velocity Showcase ($SHOWCASE_DATE)"
BODY="# Velocity Showcase — $SHOWCASE_DATE\n\nRunning gh-velocity against ${#REPOS[@]} repos...\n"
DISC_ID=$(gh api graphql ... createDiscussion ... | jq -r '.data.createDiscussion.discussion.id')
DISC_URL=$(... | jq -r '.data.createDiscussion.discussion.url')

# ── Process each repo ──────────────────────────────────────────
INDEX=""
for entry in "${REPOS[@]}"; do
  IFS='|' read -r slug repo extra_flags <<< "$entry"
  CONFIG="$TMP_DIR/$slug.yml"
  COMMENT_BODY=""
  STATUS="success"

  echo "::group::$repo"

  # 1. Preflight
  $BINARY config preflight --write="$CONFIG" -R "$repo" $extra_flags 2>/dev/null || {
    STATUS="preflight-failed"
    # Post failure comment and continue
  }

  # 2. Include generated config in comment
  COMMENT_BODY+="<details><summary>Generated Config</summary>\n\n\`\`\`yaml\n$(cat "$CONFIG")\n\`\`\`\n\n</details>\n\n"

  # 3. Report
  REPORT=$($BINARY report --since "$SINCE" --config "$CONFIG" -R "$repo" -f markdown 2>/dev/null) || {
    REPORT="*Report failed*"
    STATUS="partial"
  }
  COMMENT_BODY+="### Composite Report\n\n$REPORT\n\n"

  # 4. Individual commands (each wrapped in <details>)
  for cmd in "flow lead-time" "flow cycle-time" "flow throughput" "flow velocity"; do
    CMD_NAME=$(echo "$cmd" | tr ' ' '-' | sed 's/flow-//')
    OUTPUT=$($BINARY $cmd --since "$SINCE" --config "$CONFIG" -R "$repo" -f markdown 2>/dev/null) || {
      OUTPUT="*Not available*"
    }
    COMMENT_BODY+="<details><summary>${CMD_NAME}</summary>\n\n$OUTPUT\n\n</details>\n\n"
  done

  # 5. Post comment via addDiscussionComment
  FULL_COMMENT="<!-- velocity-showcase:$slug -->\n## $repo\n\n**Status:** $STATUS | **Generated:** $(date -u +%Y-%m-%dT%H:%MZ)\n\n$COMMENT_BODY"

  gh api graphql \
    -f query='mutation($id:ID!,$body:String!){addDiscussionComment(input:{discussionId:$id,body:$body}){comment{url}}}' \
    -f id="$DISC_ID" \
    -f body="$FULL_COMMENT"

  INDEX+="| $repo | $STATUS |\n"

  echo "::endgroup::"
done

# ── Update Discussion body with index ──────────────────────────
FINAL_BODY="# Velocity Showcase — $SHOWCASE_DATE\n\n| Repo | Status |\n|------|--------|\n$INDEX\n\n[Workflow run]($GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID)"

gh api graphql \
  -f query='mutation($id:ID!,$body:String!){updateDiscussion(input:{discussionId:$id,body:$body}){discussion{url}}}' \
  -f id="$DISC_ID" \
  -f body="$FINAL_BODY"

echo "Showcase posted: $DISC_URL"
```

- [x] Each repo's processing is wrapped in continue-on-error so one failure doesn't block others
- [x] Clean `tmp/` at the start to handle workflow re-runs (preflight refuses to overwrite)
- [x] Use `::group::`/`::endgroup::` for readable Actions log folding
- [x] No `GH_VELOCITY_POST_LIVE` needed — posting is done via `gh api`, not via gh-velocity's `--post` flag

### Phase 3: Error Handling

- [x] Preflight failure: post comment with "Preflight failed: <error>" and continue to next repo
- [x] Command failure: capture stderr, include "*Not available*" in the relevant `<details>` section
- [x] Velocity command: expected to fail for repos without iteration strategy — show "*Not available*" gracefully
- [x] Quality release: deferred — let it fail gracefully like other commands
- [x] Comment size: if composed body > 60,000 chars, truncate individual command `<details>` sections
- [x] Rate limit: 5-second sleep between repos to be kind to the API

### Phase 4: Testing

- [ ] Manual trigger via `workflow_dispatch` to validate end-to-end
- [ ] Verify Discussion appears in "Velocity Reports" category with correct title
- [ ] Verify each repo has a comment with config + report + individual commands
- [ ] Verify Discussion body has index table with status per repo
- [ ] Check API rate limit consumption after a full run
- [ ] Verify github/roadmap (issue-only repo) produces sensible output
- [ ] Verify microsoft/ebpf-for-windows gets project board config via `--project-url`

## Alternative Approaches Considered

1. **New Go posting mode (`--post-to-discussion`):** Would add `AddDiscussionComment` to the Go code and a flag to post as a comment on an existing Discussion. More robust and reusable, but YAGNI for now — only one consumer (this workflow). Noted as future work. (see brainstorm: shell orchestration over new Go code)

2. **One Discussion per repo (no comments):** Would use existing `--post` flag with idempotent upsert, creating 9 separate Discussions. Simpler (zero new code), but doesn't provide the single-URL-to-share experience the user wants. (see brainstorm: resolved as "one discussion with comments per repo")

3. **Committed configs instead of fresh preflight:** Would use curated configs from `docs/examples/`. More predictable, but misses the goal of continuously validating preflight detection logic. (see brainstorm: resolved as "fresh preflight each run")

## Acceptance Criteria

- [ ] Workflow runs daily at 06:00 UTC and supports `workflow_dispatch`
- [ ] Creates one new Discussion per run in "Velocity Reports" category on dvhthomas/gh-velocity
- [ ] Each of 9 repos has a comment on the Discussion containing:
  - [ ] The auto-generated config YAML (in collapsible section)
  - [ ] Composite report output (`report --since 30d`)
  - [ ] Individual command outputs (lead-time, cycle-time, throughput, velocity) in collapsible sections
- [ ] Discussion body contains an index table: repo name + status (success/partial/failed)
- [ ] One repo's failure does not block processing of other repos
- [ ] Completes within 90 minutes (well under 6-hour Actions limit)
- [ ] Does not exhaust GitHub API rate limits (stays under 5000 REST calls/hour)
- [ ] No Go code changes required

## Dependencies & Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| API rate limit exhaustion on kubernetes/kubernetes | Medium | Run stops producing useful data | Use `--since 14d` for this repo, or add sleep between repos |
| "Velocity Reports" category not created | Once | Workflow fails entirely | Document as manual prerequisite in Phase 1 |
| `GH_VELOCITY_TOKEN` lacks required scopes | Low | Project board reads fail silently | Verify scopes in Phase 1 |
| Comment size limit exceeded (65K chars) | Low | Comment fails to post | Truncate with note when approaching limit |
| `addDiscussionComment` mutation not available | Very Low | Comments can't be posted | Verified in GitHub GraphQL schema — it exists |
| Preflight regression breaks config generation | Medium | That repo's commands all fail | Graceful failure handling, error in comment |
| github/roadmap has no PRs/releases | Expected | Several commands produce empty output | Document as expected, show graceful degradation |

## Success Metrics

- Workflow runs green daily for 7+ consecutive days
- All 9 repos produce at least a composite report (even if some individual commands fail)
- A Discussion URL can be shared that shows comprehensive output across all repo types
- Preflight regressions are caught within 24 hours (visible as failed repos in the showcase)

## Future Considerations

**Posting configuration in `.gh-velocity.yml`** (see brainstorm: Future Work):
- Target repo for posting (post cli/cli results to dvhthomas/gh-velocity)
- Discussion title template
- Comment mode (new discussion, update body, add/update comment)
- Convention-over-configuration defaults so `--post` "just works"

This showcase workflow is the proving ground for that future feature — it demonstrates exactly the use case that posting configuration would simplify.

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-15-daily-velocity-showcase-brainstorm.md](docs/brainstorms/2026-03-15-daily-velocity-showcase-brainstorm.md)
  - Key decisions: shell orchestration, fresh preflight configs, one discussion per run with comments per repo, 9 target repos, "Velocity Reports" category

### Internal References

- Existing workflow: `.github/workflows/velocity.yaml` — current single-repo weekly report
- E2E config loop pattern: `scripts/e2e-configs.sh:26-32` — array-of-repos iteration
- Posting wiring: `cmd/post.go:22` — `postIfEnabled()` function
- Discussion GraphQL: `internal/github/discussions.go:145` — `CreateDiscussion` mutation
- Marker format: `internal/posting/marker.go:16` — `MarkerKey` function
- Preflight write flag: `cmd/preflight.go:154` — `--write` flag (NoOptDefVal pattern)
- AGENTS.md hard rule: `AGENTS.md:30-35` — `GH_VELOCITY_POST_LIVE` restriction

### Key Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `.github/workflows/showcase.yaml` | **Create** | The daily showcase workflow (thin wrapper) |
| `scripts/showcase.sh` | **Create** | Shell orchestration script for the showcase |
| `Taskfile.yaml` | **Edit** | Add `showcase` task for local runs |
| `.github/workflows/velocity.yaml` | No change | Existing weekly report remains as-is |
