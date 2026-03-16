#!/usr/bin/env bash
# Daily velocity showcase — run gh-velocity against multiple OSS repos and
# post results as comments on a single GitHub Discussion.
#
# Requires:
#   - Built ./gh-velocity binary
#   - gh auth with token that has repo, read:project, write:discussion scopes
#   - "Velocity Reports" Discussion category on the showcase repo
#   - jq
#
# Usage: scripts/showcase.sh [--dry-run]
set -uo pipefail

# ── Resolve paths ───────────────────────────────────────────────────
REPO_ROOT="$(git rev-parse --show-toplevel)"
BINARY="${REPO_ROOT}/gh-velocity"

# ── Configuration ───────────────────────────────────────────────────
SHOWCASE_DATE=$(date -u +%Y-%m-%d)
SHOWCASE_TIME=$(date -u +%Y-%m-%dT%H:%MZ)
SHOWCASE_OWNER="dvhthomas"
SHOWCASE_REPO="gh-velocity"
DISCUSSION_CATEGORY="Velocity Reports"
TMP_DIR="${REPO_ROOT}/tmp/showcase"
SINCE="30d"
DRY_RUN=false

if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=true
  echo "DRY RUN — will not create Discussion or post comments"
fi

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

# ── Preflight checks ───────────────────────────────────────────────
if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: $BINARY not found. Run 'task build' first." >&2
  exit 1
fi

if ! command -v jq &>/dev/null; then
  echo "ERROR: jq is required but not installed." >&2
  exit 1
fi

# ── Helpers ─────────────────────────────────────────────────────────
# GitHub Actions log grouping (no-ops when not in CI).
group()    { echo "::group::$1" 2>/dev/null || true; }
endgroup() { echo "::endgroup::" 2>/dev/null || true; }
warn()     { echo "::warning::$1" 2>/dev/null; echo "WARN: $1" >&2; }
err()      { echo "::error::$1"   2>/dev/null; echo "ERROR: $1" >&2; }

# run_cmd LABEL COMMAND [ARGS...] — capture stdout, log stderr, return "" on failure.
run_cmd() {
  local label="$1"; shift
  local output
  if output=$("$@"); then
    echo "$output"
  else
    echo ""
  fi
}

# ── Clean tmp directory ─────────────────────────────────────────────
rm -rf "$TMP_DIR"
mkdir -p "$TMP_DIR"

# ── Resolve IDs ─────────────────────────────────────────────────────
group "Setup: resolve repo and category IDs"

REPO_NODE_ID=$(gh api graphql \
  -f query='query($owner: String!, $repo: String!) {
    repository(owner: $owner, name: $repo) { id }
  }' \
  -f owner="$SHOWCASE_OWNER" \
  -f repo="$SHOWCASE_REPO" \
  --jq '.data.repository.id')

echo "Repository node ID: $REPO_NODE_ID"

CATEGORY_ID=$(gh api graphql \
  -f query='query($owner: String!, $repo: String!) {
    repository(owner: $owner, name: $repo) {
      discussionCategories(first: 50) {
        nodes { id name }
      }
    }
  }' \
  -f owner="$SHOWCASE_OWNER" \
  -f repo="$SHOWCASE_REPO" \
  --jq ".data.repository.discussionCategories.nodes[] | select(.name == \"$DISCUSSION_CATEGORY\") | .id")

if [[ -z "$CATEGORY_ID" ]]; then
  err "Discussion category '$DISCUSSION_CATEGORY' not found. Create it via Settings > Discussions > Categories."
  exit 1
fi
echo "Category ID: $CATEGORY_ID"
endgroup

# ── Create Discussion ───────────────────────────────────────────────
group "Create discussion"

TITLE="Velocity Showcase ($SHOWCASE_DATE)"
INITIAL_BODY="# Velocity Showcase — $SHOWCASE_DATE

Running gh-velocity against ${#REPOS[@]} repos. Results will appear as comments below.

**Started:** $SHOWCASE_TIME"

# Append workflow run link if available.
if [[ -n "${GITHUB_SERVER_URL:-}" ]]; then
  INITIAL_BODY+="
**Workflow run:** $GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID"
fi

DISC_ID=""
DISC_URL=""

if [[ "$DRY_RUN" == "true" ]]; then
  echo "[dry-run] Would create discussion: $TITLE"
  DISC_ID="DRY_RUN_ID"
  DISC_URL="https://github.com/$SHOWCASE_OWNER/$SHOWCASE_REPO/discussions/dry-run"
else
  DISC_RESPONSE=$(gh api graphql \
    -f query='mutation($repoID: ID!, $categoryID: ID!, $title: String!, $body: String!) {
      createDiscussion(input: {
        repositoryId: $repoID
        categoryId: $categoryID
        title: $title
        body: $body
      }) {
        discussion { id url }
      }
    }' \
    -f repoID="$REPO_NODE_ID" \
    -f categoryID="$CATEGORY_ID" \
    -f title="$TITLE" \
    -f body="$INITIAL_BODY")

  DISC_ID=$(echo "$DISC_RESPONSE" | jq -r '.data.createDiscussion.discussion.id')
  DISC_URL=$(echo "$DISC_RESPONSE" | jq -r '.data.createDiscussion.discussion.url')
fi

echo "Discussion: $DISC_URL"
endgroup

# ── Process each repo ───────────────────────────────────────────────
INDEX=""
TOTAL=${#REPOS[@]}
CURRENT=0

for entry in "${REPOS[@]}"; do
  IFS='|' read -r slug repo extra_flags <<< "$entry"
  CURRENT=$((CURRENT + 1))
  CONFIG="$TMP_DIR/$slug.yml"
  STATUS="success"

  group "[$CURRENT/$TOTAL] $repo"

  # ── 1. Preflight ────────────────────────────────────────────────
  PREFLIGHT_ERR=""
  # shellcheck disable=SC2086
  if ! $BINARY config preflight --write="$CONFIG" -R "$repo" --debug $extra_flags 2>"$TMP_DIR/$slug-preflight-stderr.txt"; then
    PREFLIGHT_ERR=$(cat "$TMP_DIR/$slug-preflight-stderr.txt")
    STATUS="preflight-failed"
    warn "Preflight failed for $repo"
  fi

  # ── 2. Build comment ────────────────────────────────────────────
  COMMENT_FILE="$TMP_DIR/$slug-comment.md"

  cat > "$COMMENT_FILE" <<EOF
## $repo

**Status:** \`$STATUS\` | **Generated:** $SHOWCASE_TIME

EOF

  # Include generated config in a collapsible section.
  if [[ -f "$CONFIG" ]]; then
    {
      echo "<details>"
      echo "<summary>Generated Config (.gh-velocity.yml)</summary>"
      echo ""
      echo '```yaml'
      cat "$CONFIG"
      echo '```'
      echo ""
      echo "</details>"
      echo ""
    } >> "$COMMENT_FILE"
  fi

  if [[ "$STATUS" == "preflight-failed" ]]; then
    {
      echo "### Preflight Failed"
      echo ""
      echo '```'
      echo "$PREFLIGHT_ERR"
      echo '```'
      echo ""
    } >> "$COMMENT_FILE"
  fi

  # Skip commands if no config was generated.
  if [[ -f "$CONFIG" ]]; then

    # ── 3. Report ───────────────────────────────────────────────
    REPORT=$(run_cmd "report" $BINARY report --since "$SINCE" --config "$CONFIG" -R "$repo" --debug -f markdown)
    if [[ -n "$REPORT" ]]; then
      {
        echo "### Composite Report"
        echo ""
        echo "$REPORT"
        echo ""
      } >> "$COMMENT_FILE"
    else
      STATUS="partial"
      {
        echo "### Composite Report"
        echo ""
        echo "*Report failed*"
        echo ""
      } >> "$COMMENT_FILE"
    fi

    # ── 4. Individual commands ──────────────────────────────────
    declare -a COMMANDS=(
      "flow lead-time|Lead Time"
      "flow cycle-time|Cycle Time"
      "flow throughput|Throughput"
      "flow velocity|Velocity"
    )

    for cmd_entry in "${COMMANDS[@]}"; do
      IFS='|' read -r cmd cmd_label <<< "$cmd_entry"
      # shellcheck disable=SC2086
      CMD_OUTPUT=$(run_cmd "$cmd_label" $BINARY $cmd --since "$SINCE" --config "$CONFIG" -R "$repo" --debug -f markdown)

      {
        echo "<details>"
        echo "<summary>$cmd_label</summary>"
        echo ""
        if [[ -n "$CMD_OUTPUT" ]]; then
          echo "$CMD_OUTPUT"
        else
          echo "*Not available*"
        fi
        echo ""
        echo "</details>"
        echo ""
      } >> "$COMMENT_FILE"
    done

  fi

  # Update status line if it changed after initial write.
  if [[ "$STATUS" != "success" ]]; then
    sed -i'' -e "s/\`success\`/\`$STATUS\`/" "$COMMENT_FILE"
  fi

  # ── 5. Post comment ──────────────────────────────────────────
  COMMENT_BODY=$(cat "$COMMENT_FILE")

  # Truncate if approaching GitHub's 65536 char limit.
  COMMENT_SIZE=${#COMMENT_BODY}
  if [[ "$COMMENT_SIZE" -gt 60000 ]]; then
    warn "Comment for $repo is ${COMMENT_SIZE} chars. Truncating individual commands."
    # Re-read without <details> sections (keep header + config + report).
    COMMENT_BODY=$(sed '/<details>/,/<\/details>/d' "$COMMENT_FILE")
    COMMENT_BODY+=$'\n\n*Individual command output truncated (comment size limit).*\n'
  fi

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "[dry-run] Would post comment for $repo (${#COMMENT_BODY} chars)"
  else
    if gh api graphql \
      -f query='mutation($id: ID!, $body: String!) {
        addDiscussionComment(input: { discussionId: $id, body: $body }) {
          comment { url }
        }
      }' \
      -f id="$DISC_ID" \
      -f body="$COMMENT_BODY" \
      --silent 2>/dev/null; then
      echo "Posted comment for $repo"
    else
      err "Failed to post comment for $repo"
      STATUS="post-failed"
    fi
  fi

  INDEX+="| $repo | \`$STATUS\` |"$'\n'

  endgroup

  # Brief pause between repos to be kind to the API.
  sleep 5
done

# ── Update Discussion body with index ───────────────────────────────
group "Update discussion index"

FINAL_BODY="# Velocity Showcase — $SHOWCASE_DATE

| Repo | Status |
|------|--------|
${INDEX}
**Completed:** $(date -u +%Y-%m-%dT%H:%MZ)"

if [[ -n "${GITHUB_SERVER_URL:-}" ]]; then
  FINAL_BODY+="
**Workflow run:** $GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID"
fi

if [[ "$DRY_RUN" == "true" ]]; then
  echo "[dry-run] Would update discussion body with index"
  echo "$FINAL_BODY"
else
  gh api graphql \
    -f query='mutation($id: ID!, $body: String!) {
      updateDiscussion(input: { discussionId: $id, body: $body }) {
        discussion { url }
      }
    }' \
    -f id="$DISC_ID" \
    -f body="$FINAL_BODY" \
    --silent
fi

endgroup

echo ""
echo "Showcase complete: $DISC_URL"
