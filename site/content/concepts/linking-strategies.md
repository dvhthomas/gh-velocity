---
title: "Linking Strategies"
weight: 3
---

# Linking Strategies

The `quality release` command needs to determine which issues belong to a release. This is harder than it sounds -- different teams use different workflows, and no single method works everywhere. `gh-velocity` uses three strategies and merges the results.

## Connecting PRs to issues

Before diving into the strategies, it helps to understand how the tool finds PR-to-issue connections. GitHub tracks these through timeline events. A PR becomes linked to an issue when you:

- Write `Fixes #42`, `Closes #42`, or `Resolves #42` in the PR description
- Use GitHub's sidebar "Development" section to link a PR to an issue
- Mention `#42` anywhere in the PR (creates a cross-reference event)
- Use any casing variation: `fix #42`, `close #42`, `resolve #42`

The PR does **not** need to be merged, closed, or even out of draft. Opening a draft PR that mentions an issue is enough for the tool to find the connection.

You do **not** need to:

- Add special labels or tags
- Use a specific branch naming convention
- Configure webhooks or integrations
- Follow any commit message format (unless you want commit-based enrichment)

## The three strategies

### pr-link

The highest-fidelity strategy. It works by:

1. Searching for PRs merged between the previous tag date and the target tag date
2. Querying each PR's `closingIssuesReferences` via GraphQL
3. Returning issues with full metadata (title, labels, dates)

This works well for teams that use "Fixes #N" in PR descriptions or GitHub's sidebar linking. It requires that your tags correspond to GitHub Releases (or at least that the tag's commit has a resolvable date).

**Limitation**: The GitHub search API returns at most 1000 results per query. If your release window contains more than 1000 merged PRs, results are partial. The tool warns when this happens.

### commit-ref

Scans commit messages between two tags for issue references. By default, it only matches closing keywords:

```
fixes #42
Closes #10
RESOLVED #99
```

With `patterns: ["closes", "refs"]` in your config, it also matches bare references:

```
implement #42
update #7
```

Commits are grouped by issue number. If three commits all reference `#42`, the tool returns one item with three associated commits.

The `commit-ref` strategy is especially useful when PRs are squash-merged and the commit message contains the issue reference, or when developers commit directly to the main branch.

### changelog

Parses the GitHub Release body (the release notes text) for `#N` references. This catches issues mentioned in release notes that are not linked via PRs or commit messages.

This strategy is low-fidelity -- it only finds the issue number from the release body, not full issue data. The tool fetches issue details separately after extracting the references.

## How merge works

Results from all three strategies are combined using priority-based deduplication:

1. **pr-link** has highest priority (most data, highest confidence)
2. **commit-ref** is next
3. **changelog** is lowest

When the same issue number appears in multiple strategies, the highest-priority version wins. This means pr-link's rich data (PR reference, full issue metadata) is preferred over commit-ref's issue-number-only data.

Use the `--discover` flag to see this merge in action:

```bash
gh velocity quality release v1.2.0 --discover
```

The output lists what each strategy found independently, then shows the merged result. Items that appear in multiple strategies are annotated with "(also: commit-ref)" or similar markers, so you can see which strategies overlap.

## When to use each strategy

Most teams do not need to think about strategies -- all three run automatically and results are merged. But understanding them helps when debugging gaps in your release reports.

| Scenario | Primary strategy | Notes |
|----------|-----------------|-------|
| PRs close issues with "Fixes #N" | pr-link | Highest fidelity; works out of the box |
| Squash merges with issue refs in commit messages | commit-ref | Set `commit_ref.patterns` if you use bare `#N` refs |
| Hand-written release notes mention issues | changelog | Low fidelity but catches issues missed by other strategies |
| Direct commits to main (no PRs) | commit-ref | Only strategy that works without PRs |

## Configuring commit-ref patterns

The `commit_ref.patterns` config controls how aggressively the `commit-ref` strategy matches:

```yaml
# Conservative (default): only closing keywords
commit_ref:
  patterns: ["closes"]

# Broader: also match bare #N references
commit_ref:
  patterns: ["closes", "refs"]
```

The broader setting catches commits like "implement #42" but can produce false positives from messages like "update step #1." Use it when your team consistently references issues in commits without closing keywords.
