#!/usr/bin/env bash
# Smoke tests — run real commands against public repos and print output.
# stdout: verbose output showing actual stats (useful in CI logs).
# stderr: only on failure (exit 1).
# Requires: gh auth (valid GitHub token), built ./gh-velocity binary.
set -euo pipefail

# Resolve repo root so this script works from worktrees and subdirectories.
REPO_ROOT="$(git rev-parse --show-toplevel)"
BINARY="${REPO_ROOT}/gh-velocity"
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

# ── quality release --discover ────────────────────────────────────
echo ""
echo "quality release --discover (cli/cli v2.65.0)"

out=$($BINARY quality release v2.65.0 -R cli/cli --since v2.64.0 --discover 2>&1)
show "$out"
[[ "$out" == *"Scope: v2.65.0"* ]] && pass "quality release --discover pretty" || fail "quality release --discover pretty"
[[ "$out" == *"Strategy:"* ]] && pass "quality release --discover shows strategies" || fail "quality release --discover shows strategies"

out=$($BINARY quality release v2.65.0 -R cli/cli --since v2.64.0 --discover -f json 2>/dev/null)
echo "$out" | jq . 2>/dev/null | sed 's/^/    /'
echo "$out" | jq -e '.strategies' >/dev/null 2>&1 && pass "quality release --discover json" || fail "quality release --discover json"

out=$($BINARY quality release v2.65.0 -R cli/cli --since v2.64.0 --discover -f markdown 2>/dev/null)
show "$out"
[[ "$out" == *"## Scope:"* ]] && pass "quality release --discover markdown" || fail "quality release --discover markdown"

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
[[ "$out" == *"throughput"* ]] && pass "flow help shows throughput" || fail "flow help shows throughput"

out=$($BINARY status --help 2>&1)
show "$out"
[[ "$out" == *"wip"* ]] && pass "status help shows wip" || fail "status help shows wip"
[[ "$out" == *"my-week"* ]] && pass "status help shows my-week" || fail "status help shows my-week"
[[ "$out" == *"reviews"* ]] && pass "status help shows reviews" || fail "status help shows reviews"

out=$($BINARY risk --help 2>&1)
show "$out"
[[ "$out" == *"bus-factor"* ]] && pass "risk help shows bus-factor" || fail "risk help shows bus-factor"

# ── flow throughput ───────────────────────────────────────────────
echo ""
echo "flow throughput (cli/cli --since 7d)"

out=$($BINARY flow throughput --since 7d -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"Throughput:"* ]] && pass "flow throughput pretty" || fail "flow throughput pretty"
[[ "$out" == *"Issues closed:"* ]] && pass "flow throughput shows issues" || fail "flow throughput shows issues"

out=$($BINARY flow throughput --since 7d -R cli/cli -f json 2>/dev/null)
echo "$out" | jq '.total' 2>/dev/null | sed 's/^/    total: /'
echo "$out" | jq -e '.issues_closed' >/dev/null 2>&1 && pass "flow throughput json" || fail "flow throughput json"

out=$($BINARY flow throughput --since 7d -R cli/cli -f markdown 2>/dev/null)
show "$out"
[[ "$out" == *"## Throughput:"* ]] && pass "flow throughput markdown" || fail "flow throughput markdown"

# ── risk bus-factor ───────────────────────────────────────────────
echo ""
echo "risk bus-factor (local repo)"

out=$($BINARY risk bus-factor 2>&1)
show "$out"
[[ "$out" == *"Knowledge Risk"* ]] && pass "risk bus-factor pretty" || fail "risk bus-factor pretty"

out=$($BINARY risk bus-factor -f json 2>/dev/null)
echo "$out" | jq '.paths | length' 2>/dev/null | sed 's/^/    paths: /'
echo "$out" | jq -e '.paths' >/dev/null 2>&1 && pass "risk bus-factor json" || fail "risk bus-factor json"

out=$($BINARY risk bus-factor -f markdown 2>/dev/null)
show "$out"
[[ "$out" == *"## Knowledge Risk"* ]] && pass "risk bus-factor markdown" || fail "risk bus-factor markdown"

# ── status reviews ───────────────────────────────────────────────
echo ""
echo "status reviews (dvhthomas/gh-velocity)"

out=$($BINARY status reviews -R dvhthomas/gh-velocity 2>&1)
show "$out"
[[ "$out" == *"Review Queue"* ]] && pass "status reviews pretty" || fail "status reviews pretty"

out=$($BINARY status reviews -R dvhthomas/gh-velocity -f json 2>/dev/null)
echo "$out" | jq '.count' 2>/dev/null | sed 's/^/    count: /'
echo "$out" | jq -e '.count >= 0' >/dev/null 2>&1 && pass "status reviews json" || fail "status reviews json"

out=$($BINARY status reviews -R dvhthomas/gh-velocity -f markdown 2>/dev/null)
show "$out"
[[ "$out" == *"## Review Queue"* ]] && pass "status reviews markdown" || fail "status reviews markdown"

# ── debug flag ────────────────────────────────────────────────────
echo ""
echo "debug flag"

out=$($BINARY flow lead-time 2 -R cli/cli --debug 2>&1)
show "$out"
[[ "$out" == *"[debug] repo:"* ]] && pass "debug shows repo" || fail "debug shows repo"
[[ "$out" == *"[debug] config:"* ]] && pass "debug shows config" || fail "debug shows config"

# ── config preflight ──────────────────────────────────────────────
echo ""
echo "config preflight (cli/cli, no project board)"

out=$($BINARY config preflight -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"quality:"* ]] && pass "preflight generates quality config" || fail "preflight generates quality config"
[[ "$out" == *"cycle_time:"* ]] && pass "preflight generates cycle_time config" || fail "preflight generates cycle_time config"
[[ "$out" == *"categories:"* ]] && pass "preflight generates categories" || fail "preflight generates categories"

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

out=$($BINARY flow throughput --help 2>&1)
[[ "$out" == *"Examples:"* ]] && pass "throughput has examples" || fail "throughput has examples"

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

# ── --post dry-run ────────────────────────────────────────────────
echo ""
echo "posting (dry-run by default)"

out=$($BINARY flow lead-time 2 -R cli/cli --post 2>&1)
show "$out"
[[ "$out" == *"dry-run"* ]] && pass "post defaults to dry-run" || fail "post defaults to dry-run"

out=$($BINARY flow lead-time 2 -R cli/cli --new-post 2>&1)
show "$out"
[[ "$out" == *"dry-run"* ]] && pass "new-post defaults to dry-run" || fail "new-post defaults to dry-run"

# ── preflight posting readiness ──────────────────────────────────
echo ""
echo "preflight posting readiness"

out=$($BINARY config preflight -R cli/cli -f json 2>/dev/null)
echo "$out" | jq '.posting_readiness.discussions_enabled' 2>/dev/null | sed 's/^/    discussions: /'
echo "$out" | jq -e '.posting_readiness' >/dev/null 2>&1 && pass "preflight json has posting_readiness" || fail "preflight json has posting_readiness"
echo "$out" | jq -e '.verification.config_parses' >/dev/null 2>&1 && pass "preflight json has verification" || fail "preflight json has verification"

out=$($BINARY config preflight -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"Discussions"* ]] && pass "preflight pretty shows discussions" || fail "preflight pretty shows discussions"

# ── CI logging format ────────────────────────────────────────────
echo ""
echo "CI logging format"

out=$(GITHUB_ACTIONS=true $BINARY flow lead-time 2 -R cli/cli --post 2>&1)
show "$out"
[[ "$out" == *"::notice::"* ]] && pass "CI mode emits ::notice::" || fail "CI mode emits ::notice::"

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
