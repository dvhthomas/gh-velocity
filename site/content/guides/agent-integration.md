---
title: "Agent Integration"
weight: 5
---

# Agent Integration

Every gh-velocity command supports `--results json` for structured output, making it composable with LLM agents, CI scripts, and data pipelines.

## JSON output structure

Every command produces up to four layers of data:

- **Stats** — aggregate numbers (top-level keys like `lead_time`, `cycle_time`)
- **Detail** — per-item breakdowns (the `issues` array)
- **Insights** — observations with severity (the `insights` array)
- **Provenance** — how the output was produced (the `provenance` object)

The `report` command emits stats only. Standalone commands emit all four layers. Details:

- **Durations** are in seconds (divide by 86400 for days)
- **Ratios** are floats between 0 and 1
- **Booleans** flag outlier status, hotfix status, etc.
- **Warnings** are included as a `warnings` array in the JSON object

```bash
gh velocity quality release v1.2.0 --results json | jq 'keys'
```

## Metric states in JSON

Timing fields like `cycle_time` and `lead_time` have three possible states:

```json
// Completed — has a duration
{"started_at": "2026-01-20T09:00:00Z", "duration_seconds": 198600}

// In progress — started but no end signal yet
{"started_at": "2026-01-20T09:00:00Z", "duration_seconds": null}

// N/A — no start signal found
{"started_at": null, "duration_seconds": null}
```

Check `started_at` to distinguish "in progress" from "N/A."

## Extracting data with jq

### Outlier issues

```bash
gh velocity quality release v1.2.0 --results json | \
  jq '[.issues[] | select(.lead_time_outlier) | {number, title, lead_time_seconds}]'
```

### Bug percentage

```bash
gh velocity quality release v1.2.0 --results json | \
  jq '.composition | "\(.bug_count)/\(.total_issues) bugs (\(.bug_ratio * 100 | round)%)"'
```

### P95 lead time in days

```bash
gh velocity quality release v1.2.0 --results json | \
  jq '.aggregates.lead_time.p95_seconds / 86400 | round | "\(.) days"'
```

### Slowest issues

```bash
gh velocity quality release v1.2.0 --results json | \
  jq -r '.issues | sort_by(-.lead_time_seconds) | .[0:5] | .[] |
    "#\(.number) \(.title[0:40]) -- \(.lead_time_seconds / 86400 | round)d"'
```

### Throughput count

```bash
gh velocity flow throughput --since 30d --results json | \
  jq '{issues_closed, prs_merged}'
```

### Velocity per iteration

```bash
gh velocity flow velocity --results json | \
  jq '.iterations[] | {name, velocity, committed, completion_rate}'
```

## Error handling in JSON mode

With `--results json`, errors are emitted as structured `ErrorEnvelope` objects on stderr, enabling programmatic parsing:

```json
{
  "error": {
    "code": 4,
    "message": "release v9.9.9 not found",
    "type": "not_found"
  }
}
```

Error codes follow the `model.AppError` convention:

| Code | Type | Meaning |
|------|------|---------|
| 0 | success | Command completed |
| 1 | general | Unexpected error |
| 2 | config | Configuration problem |
| 3 | auth | Authentication failure |
| 4 | not_found | Resource not found |

To handle errors in a script:

```bash
OUTPUT=$(gh velocity quality release v1.2.0 --results json 2>errors.json)
if [ $? -ne 0 ]; then
  ERROR_TYPE=$(jq -r '.error.type' errors.json)
  echo "Failed: $ERROR_TYPE"
  exit 1
fi
echo "$OUTPUT" | jq '.aggregates'
```

Warnings (non-fatal issues like low label coverage) appear inside the main JSON output as a `warnings` array, not on stderr.

## Claude Code / Copilot agent patterns

For agents that run shell commands, provide instructions like:

```
You have access to `gh velocity`. Use it to analyze our last 3 releases
and identify trends in lead time and bug ratio.

Commands available:
  gh velocity quality release <tag> --results json
  gh velocity quality release <tag> --discover --results json
  gh velocity flow lead-time <issue> --results json
  gh velocity flow lead-time --since 30d --results json
  gh velocity flow cycle-time <issue> --results json
  gh velocity flow throughput --since 30d --results json
  gh velocity flow velocity --results json
  gh velocity report --since 30d --results json

Our recent tags: v2.5.0, v2.4.0, v2.3.0
```

JSON output includes every field an agent needs: seconds-based durations, ratios as floats, boolean flags, and descriptive warnings.

### Multi-release analysis

Compare releases by running multiple commands:

```bash
for tag in v2.5.0 v2.4.0 v2.3.0; do
  gh velocity quality release "$tag" --results json > "${tag}.json"
done
```

Then extract trends:

```bash
for tag in v2.5.0 v2.4.0 v2.3.0; do
  echo -n "$tag: "
  jq -r '"median \(.aggregates.lead_time.median_seconds / 86400 | round)d, \(.composition.bug_count) bugs"' "${tag}.json"
done
```

### Feeding output to an LLM

```bash
REPORT=$(gh velocity quality release v1.2.0 -R owner/repo --results json)
echo "$REPORT" | your-agent analyze-release
```

The JSON is self-contained -- no additional API calls needed to interpret the data.

## CI pipeline integration

### Store metrics as artifacts

```bash
gh velocity quality release "$TAG" --results json > metrics.json
```

Upload as a GitHub Actions artifact:

```yaml
- name: Upload metrics
  uses: actions/upload-artifact@v4
  with:
    name: velocity-metrics
    path: metrics.json
```

### Conditional logic based on metrics

```bash
DEFECT_RATE=$(gh velocity quality release "$TAG" --results json | \
  jq '.composition.bug_ratio')

if (( $(echo "$DEFECT_RATE > 0.5" | bc -l) )); then
  echo "High bug ratio: $DEFECT_RATE"
  # Take action: notify, block release, etc.
fi
```

### Pipe to external systems

```bash
# Post to Slack via webhook
gh velocity report --since 7d --results json | \
  jq '{text: "Weekly velocity: \(.throughput.issues_closed) issues closed"}' | \
  curl -X POST -H 'Content-Type: application/json' -d @- "$SLACK_WEBHOOK_URL"
```

## Next steps

- [Interpreting Results]({{< relref "interpreting-results" >}}) -- understand what the JSON fields mean
- [Recipes]({{< relref "recipes" >}}) -- more jq patterns and data extraction examples
- [CI Setup]({{< relref "/getting-started/ci-setup" >}}) -- automate reports in GitHub Actions
