#!/usr/bin/env bash
# Smoke tests — run real commands against public repos and print output.
# stdout: verbose output showing actual stats (useful in CI logs).
# stderr: only on failure (exit 1).
# Requires: gh auth (valid GitHub token), built ./gh-velocity binary.
set -euo pipefail

BINARY="./gh-velocity"
PASS=0
FAIL=0
ERRORS=""

pass() { PASS=$((PASS + 1)); echo "  ✓ $1"; }
fail() { FAIL=$((FAIL + 1)); ERRORS+="  ✗ $1\n"; echo "  ✗ $1" >&2; }

# Print command output indented for readability.
show() { echo "$1" | sed 's/^/    /'; }

# Check binary exists
if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: $BINARY not found. Run 'task build' first." >&2
  exit 1
fi

echo "Smoke tests"
echo "==========="

# ── version ────────────────────────────────────────────────────────
echo ""
echo "version"

out=$($BINARY version 2>&1)
show "$out"
[[ "$out" == *"gh-velocity"* ]] && pass "version pretty" || fail "version pretty"

out=$($BINARY version --format json 2>&1)
show "$out"
echo "$out" | jq -e '.version' >/dev/null 2>&1 && pass "version json" || fail "version json"

# ── config ─────────────────────────────────────────────────────────
echo ""
echo "config"

out=$($BINARY config show 2>&1)
show "$out"
[[ "$out" == *"workflow"* ]] && pass "config show" || fail "config show"

out=$($BINARY config validate 2>&1)
show "$out"
[[ "$out" == *"valid"* ]] && pass "config validate" || fail "config validate"

# ── config discover ───────────────────────────────────────────────
echo ""
echo "config discover (dvhthomas/gh-velocity)"

out=$($BINARY config discover -R dvhthomas/gh-velocity 2>&1)
show "$out"
[[ "$out" == *"PVT_"* ]] && pass "config discover finds project" || fail "config discover finds project"
[[ "$out" == *"Status"* ]] && pass "config discover shows status field" || fail "config discover shows status field"
[[ "$out" == *"Config snippet"* ]] && pass "config discover shows snippet" || fail "config discover shows snippet"

out=$($BINARY config discover -R dvhthomas/gh-velocity -f json 2>&1)
echo "$out" | jq '.[0].id' 2>/dev/null | sed 's/^/    /'
echo "$out" | jq -e '.[0].id' >/dev/null 2>&1 && pass "config discover json" || fail "config discover json"

out=$($BINARY config discover -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"No Projects"* ]] && pass "config discover no projects" || fail "config discover no projects"

# ── flow lead-time ────────────────────────────────────────────────
echo ""
echo "flow lead-time (cli/cli#2)"

out=$($BINARY flow lead-time 2 -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"Lead Time"* ]] && pass "flow lead-time pretty" || fail "flow lead-time pretty"
[[ "$out" == *"Created:"* ]] && pass "flow lead-time shows created" || fail "flow lead-time shows created"

out=$($BINARY flow lead-time 2 -R cli/cli -f json 2>&1)
show "$out"
echo "$out" | jq -e '.lead_time.duration_seconds' >/dev/null 2>&1 && pass "flow lead-time json" || fail "flow lead-time json"
echo "$out" | jq -e '.lead_time.start.signal' >/dev/null 2>&1 && pass "flow lead-time json start signal" || fail "flow lead-time json start signal"

out=$($BINARY flow lead-time 2 -R cli/cli -f markdown 2>&1)
show "$out"
[[ "$out" == *"|"* ]] && pass "flow lead-time markdown" || fail "flow lead-time markdown"

# ── flow lead-time bulk ──────────────────────────────────────────
echo ""
echo "flow lead-time bulk (cli/cli --since 7d)"

out=$($BINARY flow lead-time --since 7d -R cli/cli -f json 2>/dev/null)
echo "$out" | jq '.stats.count' 2>/dev/null | sed 's/^/    count: /'
echo "$out" | jq -e '.stats' >/dev/null 2>&1 && pass "flow lead-time bulk json" || fail "flow lead-time bulk json"
echo "$out" | jq -e '.window.since' >/dev/null 2>&1 && pass "flow lead-time bulk has window" || fail "flow lead-time bulk has window"

out=$($BINARY flow lead-time --since 7d -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"Lead Time:"* ]] && pass "flow lead-time bulk pretty" || fail "flow lead-time bulk pretty"

# ── flow cycle-time ───────────────────────────────────────────────
echo ""
echo "flow cycle-time (cli/cli#2)"

out=$($BINARY flow cycle-time 2 -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"Cycle Time"* ]] && pass "flow cycle-time pretty" || fail "flow cycle-time pretty"

out=$($BINARY flow cycle-time 2 -R cli/cli -f json 2>&1)
show "$out"
echo "$out" | jq -e '.issue' >/dev/null 2>&1 && pass "flow cycle-time json" || fail "flow cycle-time json"

# ── flow cycle-time --pr ─────────────────────────────────────────
echo ""
echo "flow cycle-time --pr (cli/cli PR#1)"

out=$($BINARY flow cycle-time --pr 1 -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"Cycle Time"* ]] && pass "flow cycle-time --pr pretty" || fail "flow cycle-time --pr pretty"
[[ "$out" == *"Started"* ]] && pass "flow cycle-time --pr shows started" || fail "flow cycle-time --pr shows started"

out=$($BINARY flow cycle-time --pr 1 -R cli/cli -f json 2>&1)
show "$out"
echo "$out" | jq -e '.pr' >/dev/null 2>&1 && pass "flow cycle-time --pr json" || fail "flow cycle-time --pr json"
echo "$out" | jq -e '.cycle_time.start.signal' >/dev/null 2>&1 && pass "flow cycle-time --pr json start signal" || fail "flow cycle-time --pr json start signal"

# ── quality release ────────────────────────────────────────────────
echo ""
echo "quality release (cli/cli v2.65.0)"

out=$($BINARY quality release v2.65.0 -R cli/cli --since v2.64.0 2>&1)
show "$out"
[[ "$out" == *"Release v2.65.0"* ]] && pass "quality release pretty" || fail "quality release pretty"

out=$($BINARY quality release v2.65.0 -R cli/cli --since v2.64.0 -f json 2>/dev/null)
echo "$out" | jq . 2>/dev/null | sed 's/^/    /'
echo "$out" | jq -e '.tag' >/dev/null 2>&1 && pass "quality release json" || fail "quality release json"

out=$($BINARY quality release v2.65.0 -R cli/cli --since v2.64.0 -f markdown 2>/dev/null)
show "$out"
[[ "$out" == *"## Release v2.65.0"* ]] && pass "quality release markdown" || fail "quality release markdown"

# ── deprecated release alias ──────────────────────────────────────
echo ""
echo "deprecated release alias"

out=$($BINARY release v2.65.0 -R cli/cli --since v2.64.0 2>&1)
show "$out"
[[ "$out" == *"Release v2.65.0"* ]] && pass "release alias works" || fail "release alias works"
[[ "$out" == *"quality release"* ]] && pass "release alias shows deprecation" || fail "release alias shows deprecation"

# ── quality release --scope ───────────────────────────────────────
echo ""
echo "quality release --scope (cli/cli v2.65.0)"

out=$($BINARY quality release v2.65.0 -R cli/cli --since v2.64.0 --scope 2>&1)
show "$out"
[[ "$out" == *"Scope: v2.65.0"* ]] && pass "quality release --scope pretty" || fail "quality release --scope pretty"
[[ "$out" == *"Strategy:"* ]] && pass "quality release --scope shows strategies" || fail "quality release --scope shows strategies"

out=$($BINARY quality release v2.65.0 -R cli/cli --since v2.64.0 --scope -f json 2>/dev/null)
echo "$out" | jq . 2>/dev/null | sed 's/^/    /'
echo "$out" | jq -e '.strategies' >/dev/null 2>&1 && pass "quality release --scope json" || fail "quality release --scope json"

out=$($BINARY quality release v2.65.0 -R cli/cli --since v2.64.0 --scope -f markdown 2>/dev/null)
show "$out"
[[ "$out" == *"## Scope:"* ]] && pass "quality release --scope markdown" || fail "quality release --scope markdown"

# ── report ────────────────────────────────────────────────────────
echo ""
echo "report (cli/cli --since 7d)"

out=$($BINARY report --since 7d -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"Report:"* ]] && pass "report pretty" || fail "report pretty"
[[ "$out" == *"Lead Time:"* ]] && pass "report shows lead time" || fail "report shows lead time"
[[ "$out" == *"Throughput:"* ]] && pass "report shows throughput" || fail "report shows throughput"

out=$($BINARY report --since 7d -R cli/cli -f json 2>/dev/null)
echo "$out" | jq '.lead_time.count' 2>/dev/null | sed 's/^/    lead_time count: /'
echo "$out" | jq -e '.lead_time' >/dev/null 2>&1 && pass "report json has lead_time" || fail "report json has lead_time"
echo "$out" | jq -e '.throughput' >/dev/null 2>&1 && pass "report json has throughput" || fail "report json has throughput"
echo "$out" | jq -e '.window.since' >/dev/null 2>&1 && pass "report json has window" || fail "report json has window"

out=$($BINARY report --since 7d -R cli/cli -f markdown 2>/dev/null)
show "$out"
[[ "$out" == *"## Report:"* ]] && pass "report markdown" || fail "report markdown"

# ── group parent help ─────────────────────────────────────────────
echo ""
echo "group parent help"

out=$($BINARY flow --help 2>&1)
show "$out"
[[ "$out" == *"lead-time"* ]] && pass "flow help shows lead-time" || fail "flow help shows lead-time"
[[ "$out" == *"cycle-time"* ]] && pass "flow help shows cycle-time" || fail "flow help shows cycle-time"

out=$($BINARY status --help 2>&1)
show "$out"
[[ "$out" == *"wip"* ]] && pass "status help shows wip" || fail "status help shows wip"

# ── config preflight ──────────────────────────────────────────────
echo ""
echo "config preflight (cli/cli, no project board)"

out=$($BINARY config preflight -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"quality:"* ]] && pass "preflight generates quality config" || fail "preflight generates quality config"
[[ "$out" == *"cycle_time:"* ]] && pass "preflight generates cycle_time config" || fail "preflight generates cycle_time config"
[[ "$out" == *"bug_labels:"* ]] && pass "preflight detects bug labels" || fail "preflight detects bug labels"

out=$($BINARY config preflight -R cli/cli -f json 2>/dev/null)
echo "$out" | jq '.strategy' 2>/dev/null | sed 's/^/    strategy: /'
echo "$out" | jq -e '.repo' >/dev/null 2>&1 && pass "preflight json" || fail "preflight json"

# ── help examples ─────────────────────────────────────────────────
echo ""
echo "help examples"

out=$($BINARY flow lead-time --help 2>&1)
[[ "$out" == *"Examples:"* ]] && pass "lead-time has examples" || fail "lead-time has examples"

out=$($BINARY flow cycle-time --help 2>&1)
[[ "$out" == *"Examples:"* ]] && pass "cycle-time has examples" || fail "cycle-time has examples"

out=$($BINARY report --help 2>&1)
[[ "$out" == *"Examples:"* ]] && pass "report has examples" || fail "report has examples"

out=$($BINARY config preflight --help 2>&1)
[[ "$out" == *"Examples:"* ]] && pass "preflight has examples" || fail "preflight has examples"

# ── error cases ────────────────────────────────────────────────────
echo ""
echo "error handling"

out=$($BINARY flow lead-time abc -R cli/cli 2>&1) && fail "bad issue should fail" || pass "bad issue number rejected"
show "$out"

out=$($BINARY flow lead-time 1 -R cli/cli 2>&1) && fail "PR-as-issue should fail" || pass "PR-as-issue rejected"
show "$out"
[[ "$out" == *"pull request"* ]] && pass "PR-as-issue mentions --pr" || fail "PR-as-issue mentions --pr"

out=$($BINARY flow cycle-time 2 --pr 2 -R cli/cli 2>&1) && fail "issue+pr should fail" || pass "issue+pr conflict rejected"
show "$out"

out=$($BINARY flow lead-time 2 --since 30d -R cli/cli 2>&1) && fail "issue+since should fail" || pass "flow lead-time issue+since conflict rejected"
show "$out"

out=$($BINARY flow cycle-time --pr 1 --since 30d -R cli/cli 2>&1) && fail "pr+since should fail" || pass "flow cycle-time pr+since conflict rejected"
show "$out"

out=$($BINARY flow lead-time abc -R cli/cli -f json 2>&1 || true)
show "$out"
echo "$out" | jq -e '.error.code' >/dev/null 2>&1 && pass "json error envelope" || fail "json error envelope"

# ── old commands removed ──────────────────────────────────────────
echo ""
echo "old commands removed (clean break)"

out=$($BINARY lead-time 2 -R cli/cli 2>&1) && fail "old lead-time should fail" || pass "old lead-time rejected"
out=$($BINARY cycle-time 2 -R cli/cli 2>&1) && fail "old cycle-time should fail" || pass "old cycle-time rejected"
out=$($BINARY scope v2.65.0 -R cli/cli 2>&1) && fail "old scope should fail" || pass "old scope rejected"
out=$($BINARY stats --since 7d -R cli/cli 2>&1) && fail "old stats should fail" || pass "old stats rejected"
out=$($BINARY wip -R dvhthomas/gh-velocity 2>&1) && fail "old wip should fail" || pass "old wip rejected"

# ── summary ────────────────────────────────────────────────────────
echo ""
echo "==========="
echo "Passed: $PASS  Failed: $FAIL"

if [[ $FAIL -gt 0 ]]; then
  echo "" >&2
  echo "Failures:" >&2
  echo -e "$ERRORS" >&2
  exit 1
fi

echo "All smoke tests passed."
