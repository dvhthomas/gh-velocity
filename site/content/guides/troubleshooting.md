---
title: "Troubleshooting"
weight: 7
---

# Troubleshooting

Solutions for common errors, N/A results, and unexpected output from gh-velocity.

## Error messages

### "not a git repository. Use --repo owner/name"

You are not inside a git checkout. Either `cd` into a local clone or use the `-R owner/name` flag:

```bash
gh velocity quality release v1.2.0 -R owner/repo
```

### "no GitHub release for v1.0.0, using current time"

The tag exists but has no corresponding GitHub Release. The tool falls back to the current time for the release date, which makes release lag inaccurate. Fix by creating GitHub Releases for your tags. If you only push tags, the tool resolves dates from the tag's commit -- which works but is less precise.

### "strategy pr-link: pr-link strategy requires tag dates"

Both the current and previous tags need dates for the pr-link strategy to search for merged PRs. This usually means the previous tag has no GitHub Release and the tag date could not be resolved. The other strategies (commit-ref, changelog) still run and produce results.

### "Low label coverage: N/M issues have no bug/feature labels"

More than half the issues in a release lack the labels configured for classification. Fix by:

1. Labeling your issues with bug/feature labels
2. Customizing [`quality.categories`]({{< relref "/reference/config" >}}#qualitycategories) in your config to match your existing labels
3. Running `gh velocity config preflight` to discover available labels and generate matching categories

### "shallow clone detected; commit history is incomplete"

You are running in a git checkout cloned with limited history. This is common in CI. Fix in GitHub Actions:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0    # fetch full history
```

Without full history, the tool cannot find commits between tags or search commit messages for issue references. Lead time (which only uses issue dates) is unaffected.

### "The GitHub search API caps at 1000 results"

The pr-link strategy found more than 1000 merged PRs in the release window. Results are partial. This is rare outside the largest monorepos. The tool warns and returns what it found. The other strategies (commit-ref, changelog) supplement the partial results.

## Cycle time issues

### Cycle time shows N/A for all issues

This is the most common first-run issue. The cause depends on your configured strategy.

**Issue strategy** (`cycle_time.strategy: issue`):

- **Missing `lifecycle.in-progress.match` in config.** The tool has no label-based signal to detect when work started. Fix: add labels like `in-progress` to your repo and configure:
  ```yaml
  lifecycle:
    in-progress:
      match: ["label:in-progress"]
  ```
- **Labels not applied to issues.** The issue strategy requires that matching labels are actually applied to issues. Check that your team is applying the `in-progress` label when work starts. If you use a project board, consider [gh-project-label-sync](https://github.com/dvhthomas/gh-project-label-sync) to automate label application when cards move.

**PR strategy** (`cycle_time.strategy: pr`):

- **No closing PRs found.** Ensure PRs reference issues with "Closes #N" or "Fixes #N" in the PR description, or use GitHub's sidebar linking.
- **Issues were closed without PRs.** The PR strategy requires merged PRs linked to issues. Issues closed manually or by bots will have N/A cycle time.

**Quick fix**: Switch to `strategy: pr` if you do not use lifecycle labels. It works immediately when PRs reference issues.

### Cycle time shows N/A for a single issue

Cycle time is N/A when the configured strategy has no signal for that specific issue:

- **Issue strategy**: The issue has no matching in-progress label applied.
- **PR strategy**: No merged PR references this issue with a closing keyword.

### Negative cycle times

Label-based cycle time should not produce negative durations since label timestamps are immutable. If you see negative cycle times, check that your `lifecycle.in-progress.match` configuration is correct and that the matched labels were applied before issue closure.

## Output issues

### No results / empty output

1. **Check your date range.** `--since 30d` looks at the last 30 days. Try a wider range: `--since 90d`.
2. **Check your scope.** Run with `--debug` to see the GitHub search query being sent. Bulk commands show a "Verify:" link -- open it in GitHub to see what the search returns.
3. **Check for activity.** A repo with no closed issues or merged PRs in the window produces empty results. That is correct behavior.

### Bug ratio shows 0%

The report's bug ratio counts issues classified as "bug". If you name your bug category differently (e.g., "defect", "incident"), rename it to `bug` in your config:

```yaml
quality:
  categories:
    - name: bug        # must be "bug" for bug ratio
      match:
        - "label:defect"
        - "label:incident"
```

### High "other" count in composition

Issues classified as "other" did not match any category in `quality.categories`. This usually means issues lack the expected labels. Fix by:

1. Labeling issues before releasing
2. Adding more matchers to your categories (e.g., `type:Bug` for GitHub Issue Types, `title:/^fix/i` for title patterns). See [Configuration Reference: matcher syntax]({{< relref "/reference/config" >}}#matcher-syntax) for all matcher types.
3. Running `gh velocity config preflight` to discover available labels

### Lead time median is suspiciously low (minutes instead of days)

If the median lead time is in minutes while the mean is in months, the data likely includes spam or duplicate issues that were closed instantly. The "issues closed in under 60 seconds" insight confirms this.

**Fix:** Regenerate your config to auto-detect noise labels:

```bash
gh velocity config preflight -R owner/repo --write
```

The preflight detects labels matching `spam`, `duplicate`, and `invalid` and adds scope exclusions:

```yaml
scope:
  query: "repo:owner/repo -label:duplicate -label:invalid -label:suspected-spam"
```

If your repo has noise labels with different names, add them manually to the scope query.

See [Interpreting Results: Why noise exclusion matters]({{< relref "interpreting-results" >}}#why-noise-exclusion-matters) for a detailed before/after comparison.

### Tag ordering is unexpected

Tags are returned in the order GitHub's API provides, which is usually creation date. The tool picks the tag immediately before your target tag in this list. If your tag history is non-linear (e.g., you tagged a hotfix on an older branch), use `--since` to specify the previous tag explicitly:

```bash
gh velocity quality release v2.0.0 --since v1.9.0
```

## Configuration issues

### "config file required"

All metric commands require a [`.gh-velocity.yml`]({{< relref "/getting-started/configuration" >}}) file. Create one with:

```bash
gh velocity config preflight --write
```

Or for a remote repo:

```bash
gh velocity config preflight -R owner/repo --write
```

### Unknown keys produce warnings

Unknown keys in the config file produce warnings to stderr but do not cause errors. This lets you add comments or future fields without breaking the tool. If you see unexpected warnings, check for typos in field names.

### Validating your config

```bash
gh velocity config validate
```

This checks all fields for correct types, valid ranges, and proper formats. It does not make API calls.

To see the resolved configuration with all defaults applied:

```bash
gh velocity config show
gh velocity config show --results json
```

## CI issues

### --post does nothing in CI

`--post` runs in dry-run mode by default. Set `GH_VELOCITY_POST_LIVE=true` in your workflow environment. See [Posting Reports]({{< relref "/guides/posting-reports" >}}) for all posting patterns:

```yaml
env:
  GH_VELOCITY_POST_LIVE: 'true'
```

### "Resource not accessible by integration"

The `GITHUB_TOKEN` does not have the required permissions. Add explicit permissions to your workflow:

```yaml
permissions:
  contents: read
  issues: write           # for --post to issues
  discussions: write      # for --post to Discussions
```

### Project board commands fail in CI

The default `GITHUB_TOKEN` cannot access Projects v2 boards. Set up `GH_VELOCITY_TOKEN` with `project` scope. See [CI Setup: Setting up GH_VELOCITY_TOKEN]({{< relref "/getting-started/ci-setup" >}}#setting-up-gh_velocity_token).

## Velocity issues

### Velocity shows high not-assessed count

Items without an effort value show as "not assessed" in velocity output. This happens when issues lack the effort labels (e.g., `size/S`, `size/M`) or project board field values that your `velocity.effort` config expects. Fix by applying effort labels to your issues, or if you use a project board number field, switch to `strategy: numeric` and ensure the field is populated. Run `gh velocity config validate --velocity` to see which issues are unmatched.

## Debugging

Use `--debug` to print diagnostic information to stderr:

```bash
gh velocity quality release v1.2.0 --debug
```

This shows:
- The GitHub search queries being sent
- API call details
- Strategy resolution logic
- Timing information

The debug output goes to stderr, so it does not interfere with JSON or markdown output on stdout:

```bash
gh velocity quality release v1.2.0 --results json --debug 2>debug.log | jq '.aggregates'
```

## Next steps

- [Cycle Time Setup]({{< relref "cycle-time-setup" >}}) -- configure cycle time correctly from the start
- [Labels as Lifecycle Signal]({{< relref "/concepts/labels-vs-board" >}}) -- why labels are the sole lifecycle signal
- [Configuration]({{< relref "/getting-started/configuration" >}}) -- full config setup guide
