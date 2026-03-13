#!/usr/bin/env bash
# Generate a preflight config for a repo into output/configs/.
# Usage: generate-config.sh <owner/repo> [--project-url <url>]
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
BINARY="${REPO_ROOT}/gh-velocity"

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <owner/repo> [--project-url <url>]" >&2
  exit 1
fi

repo="$1"
shift
slug=$(echo "$repo" | tr '/' '-')
outdir="${REPO_ROOT}/output/configs"
mkdir -p "$outdir"
outfile="${outdir}/${slug}.yml"

# Remove existing so --write doesn't refuse to overwrite.
rm -f "$outfile"

echo "→ preflight ${repo} → ${outfile}"
"$BINARY" config preflight -R "$repo" --write="${outfile}" "$@"
