---
title: "Understanding Statistics"
weight: 2
---

# Understanding the Statistics

`gh-velocity` reports several statistical measures for lead time, cycle time, and release lag. This page explains what each one means and why it matters, without assuming a statistics background.

## Why median over mean

Lead times are almost always skewed: most issues close in days or weeks, but a few ancient issues get closed during a release and pull the average way up. The median (the middle value when you sort all durations) resists this distortion.

A concrete example from cli/cli v2.67.0:

| Measure | Value |
|---------|-------|
| Mean lead time | 280 days |
| Median lead time | 60 days |

The mean is 4.6x the median because two issues that had been open for over 4 years were closed in this release. The mean says "the average issue takes 9 months." The median says "the typical issue takes about 2 months." The median is closer to reality for this team.

When you see the median and mean diverge sharply, it tells you there are outliers pulling the average. Both numbers are reported so you can spot this pattern.

## P90 and P95 percentiles

P90 and P95 answer the question: "How long does it take if things go slower than usual?"

- **P90**: 90% of issues shipped within this duration. 1 in 10 took longer.
- **P95**: 95% of issues shipped within this duration. Only 1 in 20 took longer.

These are useful for setting expectations or SLAs. If your P95 lead time is 30 days, you can tell stakeholders "virtually all issues ship within a month" and be right 19 times out of 20.

Percentiles require at least 5 data points. Below that threshold, the values are omitted rather than computed from too little data.

## Outlier detection

The tool flags individual issues that took unusually long compared to their peers. It uses the interquartile range (IQR) method, the same approach used in box-and-whisker plots:

1. Sort all durations and find Q1 (25th percentile) and Q3 (75th percentile)
2. Compute IQR = Q3 - Q1 (the spread of the middle 50%)
3. Set the outlier threshold at Q3 + 1.5 * IQR
4. Flag any value above the threshold

This method adapts to your data. A team with consistently long lead times has a higher threshold than a team that ships fast. An issue is only flagged as an outlier relative to the other issues in the same release.

In practice: if most issues in a release close in 5-15 days, an issue that took 60 days would be flagged. But if your issues typically take 30-90 days, 60 days would not be flagged because it falls within the normal range for your team.

Outlier detection requires at least 4 data points. Flagged issues appear with `OUTLIER` in pretty and markdown output, and `lead_time_outlier: true` in JSON output.

## Standard deviation

Standard deviation measures how spread out your durations are. The tool uses sample standard deviation (N-1 denominator).

The raw number is hard to interpret on its own. The useful signal is the ratio of standard deviation to mean:

- **stddev / mean < 0.5**: Your delivery times are fairly consistent.
- **stddev / mean near 1.0**: Significant variability. Some issues take much longer than others.
- **stddev / mean > 1.0**: Highly variable. Predicting delivery time for any single issue is unreliable.

High variability is not inherently bad -- it often reflects a mix of quick fixes and longer projects. But if you are trying to make delivery more predictable, reducing this ratio is a concrete goal.

Standard deviation requires at least 2 data points.

## See also

- [Interpreting Results]({{< relref "/guides/interpreting-results" >}}) -- what "good" looks like for each metric, and common patterns to watch for
- [Lead Time Reference]({{< relref "/reference/metrics/lead-time" >}}) -- metric definition and aggregation details
- [Cycle Time Reference]({{< relref "/reference/metrics/cycle-time" >}}) -- metric definition and aggregation details
- [Quality Metrics]({{< relref "/reference/metrics/quality" >}}) -- outlier detection in release reports
