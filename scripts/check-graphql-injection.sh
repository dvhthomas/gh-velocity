#!/usr/bin/env bash
# Detect potential GraphQL injection: fmt.Sprintf used near GraphQL query strings.
# Fails if any Go file contains fmt.Sprintf with query/mutation keywords,
# which suggests string interpolation is being used to build GraphQL queries.

set -euo pipefail

# Resolve repo root so this script works from worktrees and subdirectories.
REPO_ROOT="$(git rev-parse --show-toplevel)"

pattern='fmt\.Sprintf\s*\(\s*`[^`]*(query|mutation)'

if grep -rPn "$pattern" --include='*.go' "$REPO_ROOT" 2>/dev/null; then
  echo "ERROR: Found fmt.Sprintf building GraphQL queries."
  echo "Use the variables map in client.Do(query, variables, &result) instead."
  exit 1
fi

echo "✓ No GraphQL injection patterns found"
