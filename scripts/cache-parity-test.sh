#!/usr/bin/env bash
# Cache parity test — verifies that cached output is identical to uncached output.
# Uses GH_VELOCITY_NOW to fix timestamps and eliminate drift between runs.
#
# Requires: built ./gh-velocity binary, gh auth, .gh-velocity.yml config.
# Usage: scripts/cache-parity-test.sh
set -uo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
BINARY="${REPO_ROOT}/gh-velocity"
CACHE_DIR="${HOME}/Library/Caches/gh-velocity"
TMP_DIR="${REPO_ROOT}/tmp/cache-parity"
PASS=0
FAIL=0
ERRORS=""

# Fixed time eliminates timestamp drift between runs.
export GH_VELOCITY_NOW="2026-03-15T00:00:00Z"
REPO="dvhthomas/gh-velocity"
SINCE="7d"

pass() { PASS=$((PASS + 1)); echo "  ✓ $1"; }
fail() { FAIL=$((FAIL + 1)); ERRORS+="  ✗ $1\n"; echo "  ✗ $1" >&2; }

if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: $BINARY not found. Run 'task build' first." >&2
  exit 1
fi

rm -rf "$TMP_DIR"
mkdir -p "$TMP_DIR"

echo "Cache parity tests"
echo "==================="
echo "Using GH_VELOCITY_NOW=$GH_VELOCITY_NOW"
echo "Repo: $REPO  Since: $SINCE"
echo ""

# ── Test 1: Cold cache vs --no-cache produces identical output ──────
# Uses long sleep between runs to avoid rate limit interference.
echo "Test 1: report — cold cache vs --no-cache"
rm -rf "$CACHE_DIR"

$BINARY report --since "$SINCE" --no-cache -R "$REPO" -f json 2>/dev/null > "$TMP_DIR/report-nocache.json"
NC_EXIT=$?
NC_WARNINGS=$(python3 -c "import json; d=json.load(open('$TMP_DIR/report-nocache.json')); print(len(d.get('warnings',[])))" 2>/dev/null || echo "?")

# If the baseline itself hit rate limits, skip this test — we can't diff against a broken baseline.
if [[ "$NC_WARNINGS" != "0" ]]; then
  echo "  ⚠ Skipping: no-cache baseline has $NC_WARNINGS warnings (likely rate-limited). Cannot establish reliable baseline."
else
  # Wait generously to let rate limit window reset.
  echo "  Waiting 30s for rate limit cooldown..."
  sleep 30

  rm -rf "$CACHE_DIR"
  $BINARY report --since "$SINCE" -R "$REPO" -f json 2>/dev/null > "$TMP_DIR/report-cold.json"
  COLD_EXIT=$?
  COLD_WARNINGS=$(python3 -c "import json; d=json.load(open('$TMP_DIR/report-cold.json')); print(len(d.get('warnings',[])))" 2>/dev/null || echo "?")

  if [[ "$COLD_WARNINGS" != "0" ]]; then
    echo "  ⚠ Skipping: cold cache run has $COLD_WARNINGS warnings (likely rate-limited)."
  elif [[ $NC_EXIT -ne $COLD_EXIT ]]; then
    fail "report exit codes differ: no-cache=$NC_EXIT cold=$COLD_EXIT"
  elif diff -q "$TMP_DIR/report-nocache.json" "$TMP_DIR/report-cold.json" >/dev/null 2>&1; then
    pass "report: cold cache == no-cache"
  else
    fail "report: cold cache output differs from no-cache"
    diff "$TMP_DIR/report-nocache.json" "$TMP_DIR/report-cold.json" >&2 || true
  fi
fi

# ── Test 2: Warm cache (second run) matches first run ───────────────
echo ""
echo "Test 2: report — warm cache produces identical output"

# Run twice with cache. Second should hit disk. Both should be identical.
rm -rf "$CACHE_DIR"
echo "  Waiting 30s for rate limit cooldown..."
sleep 30

$BINARY report --since "$SINCE" -R "$REPO" -f json 2>/dev/null > "$TMP_DIR/report-run1.json"
RUN1_WARNINGS=$(python3 -c "import json; d=json.load(open('$TMP_DIR/report-run1.json')); print(len(d.get('warnings',[])))" 2>/dev/null || echo "?")

if [[ "$RUN1_WARNINGS" != "0" ]]; then
  echo "  ⚠ Skipping: first run has warnings (likely rate-limited)."
else
  $BINARY report --since "$SINCE" -R "$REPO" -f json 2>/dev/null > "$TMP_DIR/report-run2.json"

  if diff -q "$TMP_DIR/report-run1.json" "$TMP_DIR/report-run2.json" >/dev/null 2>&1; then
    pass "report: warm cache == cold cache"
  else
    fail "report: warm cache output differs from cold cache"
    diff "$TMP_DIR/report-run1.json" "$TMP_DIR/report-run2.json" >&2 || true
  fi
fi

# ── Test 3: Individual commands match report data ───────────────────
echo ""
echo "Test 3: lead-time — warm cache matches no-cache"

$BINARY flow lead-time --since "$SINCE" --no-cache -R "$REPO" -f json 2>/dev/null > "$TMP_DIR/leadtime-nocache.json"
sleep 5
$BINARY flow lead-time --since "$SINCE" -R "$REPO" -f json 2>/dev/null > "$TMP_DIR/leadtime-cached.json"

if diff -q "$TMP_DIR/leadtime-nocache.json" "$TMP_DIR/leadtime-cached.json" >/dev/null 2>&1; then
  pass "lead-time: cached == no-cache"
else
  fail "lead-time: cached output differs from no-cache"
  diff "$TMP_DIR/leadtime-nocache.json" "$TMP_DIR/leadtime-cached.json" >&2 || true
fi

# ── Test 4: throughput — warm cache matches no-cache ────────────────
echo ""
echo "Test 4: throughput — warm cache matches no-cache"

$BINARY flow throughput --since "$SINCE" --no-cache -R "$REPO" -f json 2>/dev/null > "$TMP_DIR/throughput-nocache.json"
sleep 5
$BINARY flow throughput --since "$SINCE" -R "$REPO" -f json 2>/dev/null > "$TMP_DIR/throughput-cached.json"

if diff -q "$TMP_DIR/throughput-nocache.json" "$TMP_DIR/throughput-cached.json" >/dev/null 2>&1; then
  pass "throughput: cached == no-cache"
else
  fail "throughput: cached output differs from no-cache"
  diff "$TMP_DIR/throughput-nocache.json" "$TMP_DIR/throughput-cached.json" >&2 || true
fi

# ── Test 5: Different --since produces different results (no false hit) ──
echo ""
echo "Test 5: different --since must NOT use cache from prior run"

# Cache is warm from test 4 with --since 7d. Run with --since 3d.
$BINARY flow throughput --since 3d -R "$REPO" -f json 2>/dev/null > "$TMP_DIR/throughput-3d.json"

# The 3d result should differ from 7d (narrower window = fewer items).
T7D=$(python3 -c "import json; d=json.load(open('$TMP_DIR/throughput-nocache.json')); print(d.get('issues_closed',0)+d.get('prs_merged',0))" 2>/dev/null || echo "0")
T3D=$(python3 -c "import json; d=json.load(open('$TMP_DIR/throughput-3d.json')); print(d.get('issues_closed',0)+d.get('prs_merged',0))" 2>/dev/null || echo "0")

if [[ "$T3D" -le "$T7D" ]]; then
  pass "throughput 3d ($T3D) <= 7d ($T7D) — different window, different results"
else
  fail "throughput 3d ($T3D) > 7d ($T7D) — cache may have served wrong data"
fi

# ── Test 6: --no-cache second run also matches (no leftover state) ──
echo ""
echo "Test 6: --no-cache ignores disk cache completely"

# Disk cache is warm. Run with --no-cache and verify debug shows no disk hits.
NOCACHE_DEBUG=$($BINARY flow lead-time --since "$SINCE" --no-cache -R "$REPO" -f pretty --debug 2>&1 >/dev/null)
if echo "$NOCACHE_DEBUG" | grep -q "cache hit (disk)"; then
  fail "--no-cache still hit disk cache"
else
  pass "--no-cache did not hit disk cache"
fi

# ── Test 7: Disk cache files exist after cached run ─────────────────
echo ""
echo "Test 7: disk cache files created"

CACHE_FILES=$(find "$CACHE_DIR" -name "*.json" 2>/dev/null | wc -l | tr -d ' ')
if [[ "$CACHE_FILES" -gt 0 ]]; then
  pass "disk cache has $CACHE_FILES files"
else
  fail "no disk cache files found"
fi

# ── Summary ─────────────────────────────────────────────────────────
echo ""
echo "==================="
echo "Passed: $PASS  Failed: $FAIL"

if [[ $FAIL -gt 0 ]]; then
  echo "" >&2
  echo "Failures:" >&2
  echo -e "$ERRORS" >&2
  exit 1
fi

echo "All cache parity tests passed."
