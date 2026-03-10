# gh-velocity

A GitHub CLI extension that measures how fast your team ships.

`gh velocity` computes lead time, cycle time, release lag, and quality metrics from your existing GitHub data — issues, pull requests, releases, and commits. No external services, no tracking pixels, no configuration databases. Just your repo.

## Install

```
gh extension install dvhthomas/gh-velocity
```

Requires [GitHub CLI](https://cli.github.com/) 2.0+.

## Quick start

Most commands work against any repo via `-R`:

```bash
# How did the last release go?
gh velocity release v2.67.0 -R cli/cli

# What's in a release? Which strategy found each issue?
gh velocity scope v2.67.0 -R cli/cli

# How long did a specific issue take?
gh velocity lead-time 42 -R cli/cli
```

Cycle time works remotely too — it uses PR creation dates and assignment events from the API:

```bash
gh velocity cycle-time 42 -R cli/cli
```

From inside any local repo, you can omit `-R`:

```bash
cd your-repo
gh velocity release v1.2.0
```

## Output formats

Every command supports three formats:

```bash
gh velocity release v1.2.0                    # human-readable table
gh velocity release v1.2.0 -f json            # structured JSON
gh velocity release v1.2.0 -f markdown        # paste into an issue or PR
```

## Commands

| Command | What it measures | Needs local clone? |
| --- | --- | --- |
| `release <tag>` | Full release report: per-issue metrics, composition, aggregates, outliers | No (but improves accuracy) |
| `scope <tag>` | What a release contains, broken down by discovery strategy | No |
| `lead-time <issue>` | Time from issue creation to close | No |
| `cycle-time <issue>` | Time from work started to close | No (local clone adds commit data) |
| `config show` | Display resolved configuration | No |
| `config create` | Generate a default `.gh-velocity.yml` | No |
| `config validate` | Check your `.gh-velocity.yml` for errors | No |

### Common flags

| Flag | Short | Description |
| --- | --- | --- |
| `--format` | `-f` | Output: `pretty` (default), `json`, `markdown` |
| `--repo` | `-R` | Target repo as `owner/name` |
| `--since` | | Previous tag override (on `release` and `scope`) |

## What gets measured

The `release` command computes these metrics for every issue in a release:

- **Lead time** — issue created to issue closed
- **Cycle time** — work started (board status change, label, PR created, assigned, or first commit) to issue closed
- **Release lag** — issue closed to release published

Aggregates include mean, median, standard deviation, P90, P95, and IQR-based outlier detection. Individual issues are flagged when they exceed the outlier threshold.

## How issues are discovered

Three strategies run in parallel to find which issues belong to a release:

1. **pr-link** — finds merged PRs in the release window, then follows GitHub's "closing references" to linked issues
2. **commit-ref** — scans commit messages for closing keywords (`fixes #N`, `closes #N`, `resolves #N`)
3. **changelog** — parses the release body for `#N` references

Results are merged with priority (pr-link > commit-ref > changelog). Use `scope` to see what each strategy finds.

## Configuration

Create `.gh-velocity.yml` in your repo root. All fields are optional:

```yaml
# Issue classification
quality:
  bug_labels: ["bug", "defect"]
  feature_labels: ["enhancement", "feature"]
  hotfix_window_hours: 48

# Commit message patterns
commit_ref:
  patterns: ["closes"]          # default: closing keywords only
  # patterns: ["closes", "refs"]  # also match bare #N references
```

See [docs/guide.md](docs/guide.md) for the full configuration reference and detailed examples.

## License

MIT
