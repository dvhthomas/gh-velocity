---
title: "Agent Integration"
weight: 5
---

# Agent Integration

Every gh-velocity command supports `--format json` for structured output. This makes the tool composable with LLM agents, CI scripts, and data pipelines. For an overview of what the metrics mean, see [Interpreting Results]({{< relref "/guides/interpreting-results" >}}).

## JSON output structure

Every command produces up to four layers of data (see [Interpreting Results: Output layers]({{< relref "/guides/interpreting-results" >}}#output-layers)):

- **Stats** — aggregate numbers (top-level keys like `lead_time`, `cycle_time`)
- **Detail** — per-item breakdowns (the `issues` array)
- **Insights** — observations with severity (the `insights` array)
- **Provenance** — how the output was produced (the `provenance` object)

The `report` command emits stats only. Standalone commands emit all four layers. Additional details:

- **Durations** are in seconds (divide by 86400 for days)
- **Ratios** are floats between 0 and 1
- **Booleans** flag outlier status, hotfix status, etc.
- **Warnings** are included as a `warnings` array in the JSON object

```bash
gh velocity quality release v1.2.0 --format json | jq 'keys'
```

## Metric states in JSON

Timing fields like `cycle_time` and `lead_time` can be in three states. See [Interpreting Results: Three metric states]({{< relref "/guides/interpreting-results" >}}#three-metric-states) for what each means.

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
gh velocity quality release v1.2.0 --format json | \
  jq '[.issues[] | select(.lead_time_outlier) | {number, title, lead_time_seconds}]'
```

### Bug percentage

```bash
gh velocity quality release v1.2.0 --format json | \
  jq '.composition | "\(.bug_count)/\(.total_issues) bugs (\(.bug_ratio * 100 | round)%)"'
```

### P95 lead time in days

```bash
gh velocity quality release v1.2.0 --format json | \
  jq '.aggregates.lead_time.p95_seconds / 86400 | round | "\(.) days"'
```

### Slowest issues

```bash
gh velocity quality release v1.2.0 --format json | \
  jq -r '.issues | sort_by(-.lead_time_seconds) | .[0:5] | .[] |
    "#\(.number) \(.title[0:40]) -- \(.lead_time_seconds / 86400 | round)d"'
```

### Throughput count

```bash
gh velocity flow throughput --since 30d --format json | \
  jq '{issues_closed, prs_merged}'
```

### Velocity per iteration

```bash
gh velocity flow velocity --format json | \
  jq '.iterations[] | {name, velocity, committed, completion_rate}'
```

## Error handling in JSON mode

When `--format json` is active, errors are emitted as structured `ErrorEnvelope` objects on stderr instead of plain text. This lets agents parse errors programmatically:

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
OUTPUT=$(gh velocity quality release v1.2.0 --format json 2>errors.json)
if [ $? -ne 0 ]; then
  ERROR_TYPE=$(jq -r '.error.type' errors.json)
  echo "Failed: $ERROR_TYPE"
  exit 1
fi
echo "$OUTPUT" | jq '.aggregates'
```

Warnings (non-fatal issues like low label coverage) appear inside the main JSON output as a `warnings` array, not on stderr.

## Claude Code / Copilot agent patterns

If you use an agent that can run shell commands, point it at your repo with instructions like:

```
You have access to `gh velocity`. Use it to analyze our last 3 releases
and identify trends in lead time and bug ratio.

Commands available:
  gh velocity quality release <tag> --format json
  gh velocity quality release <tag> --discover --format json
  gh velocity flow lead-time <issue> --format json
  gh velocity flow lead-time --since 30d --format json
  gh velocity flow cycle-time <issue> --format json
  gh velocity flow throughput --since 30d --format json
  gh velocity flow velocity --format json
  gh velocity report --since 30d --format json

Our recent tags: v2.5.0, v2.4.0, v2.3.0
```

The JSON output includes every field an agent needs: seconds-based durations, ratios as floats, boolean flags, and descriptive warnings.

### Multi-release analysis

An agent can compare releases by running multiple commands:

```bash
for tag in v2.5.0 v2.4.0 v2.3.0; do
  gh velocity quality release "$tag" --format json > "${tag}.json"
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
REPORT=$(gh velocity quality release v1.2.0 -R owner/repo --format json)
echo "$REPORT" | your-agent analyze-release
```

The JSON is self-contained -- the agent does not need to make additional API calls to understand the data.

## CI pipeline integration

### Store metrics as artifacts

```bash
gh velocity quality release "$TAG" --format json > metrics.json
```

Upload as a GitHub Actions artifact for trend tracking:

```yaml
- name: Upload metrics
  uses: actions/upload-artifact@v4
  with:
    name: velocity-metrics
    path: metrics.json
```

### Conditional logic based on metrics

```bash
DEFECT_RATE=$(gh velocity quality release "$TAG" --format json | \
  jq '.composition.bug_ratio')

if (( $(echo "$DEFECT_RATE > 0.5" | bc -l) )); then
  echo "High bug ratio: $DEFECT_RATE"
  # Take action: notify, block release, etc.
fi
```

### Pipe to external systems

```bash
# Post to Slack via webhook
gh velocity report --since 7d --format json | \
  jq '{text: "Weekly velocity: \(.throughput.issues_closed) issues closed"}' | \
  curl -X POST -H 'Content-Type: application/json' -d @- "$SLACK_WEBHOOK_URL"
```

## Next steps

- [Interpreting Results]({{< relref "interpreting-results" >}}) -- understand what the JSON fields mean
- [Recipes]({{< relref "recipes" >}}) -- more jq patterns and data extraction examples
- [CI Setup]({{< relref "/getting-started/ci-setup" >}}) -- automate reports in GitHub Actions
