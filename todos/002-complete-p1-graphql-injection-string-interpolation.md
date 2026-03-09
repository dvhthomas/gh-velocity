---
status: complete
priority: p1
issue_id: 002
tags: [code-review, security, graphql, injection]
dependencies: []
---

# GraphQL Injection via String Interpolation

## Problem Statement

The plan uses `go-gh`'s string-based `Do()` method for GraphQL queries. If any user-controlled or config-controlled value is interpolated into query strings (via `fmt.Sprintf`) instead of passed as GraphQL variables, this is a GraphQL injection vulnerability. Config values like `project.id` and `status_field_id` flow directly into Projects v2 queries.

**Raised by:** Security Sentinel (HIGH), Performance Oracle (mentioned)

## Findings

- The plan already notes "Always use GraphQL variables" in the API Strategy Research Insights section
- However, no enforcement mechanism is specified
- `project.id` and `status_field_id` from `.gh-velocity.yml` are attacker-controlled

## Proposed Solutions

### Option A: Lint rule + code convention (Recommended)
- Document in CLAUDE.md: "Never use fmt.Sprintf to build GraphQL queries"
- Add a golangci-lint custom rule or grep-based CI check for `fmt.Sprintf` near GraphQL query strings
- Always use the `variables` map in `client.Do(query, variables, &result)`
- **Effort:** Small
- **Risk:** Low

## Resolution

Implemented Option A:
- CLAUDE.md updated with explicit rule against `fmt.Sprintf` for GraphQL queries
- `scripts/check-graphql-injection.sh` added as grep-based CI check
- `check:graphql-injection` task added to `task quality` pipeline
- Verified: no existing GraphQL code uses string interpolation (no `.Do()` calls exist yet; client is initialized but unused)

## Acceptance Criteria

- [x] All GraphQL queries use variables map, never string interpolation
- [x] CLAUDE.md documents the rule
- [x] CI check catches violations
