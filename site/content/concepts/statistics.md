---
title: "Understanding Statistics"
weight: 3
---

# Understanding the Statistics

gh-velocity reports several statistical measures for lead time, cycle time, and release lag. This page explains what each means and why it matters, without assuming a statistics background.

## Why median over mean

Lead times are almost always skewed: most issues close in days or weeks, but a few ancient issues closed during a release pull the average up. The median (the middle value when sorted) resists this distortion.

A concrete example from cli/cli v2.67.0:

| Measure | Value |
|---------|-------|
| Mean lead time | 280 days |
| Median lead time | 60 days |

The mean is 4.6x the median because two issues open for over 4 years were closed in this release. The mean says "the average issue takes 9 months." The median says "the typical issue takes about 2 months." The median is closer to reality.

When median and mean diverge sharply, outliers are pulling the average. Both numbers are reported so you can spot this pattern.

## P90 and P95 percentiles

P90 and P95 answer the question: "How long does it take if things go slower than usual?"

- **P90**: 90% of issues shipped within this duration. 1 in 10 took longer.
- **P95**: 95% of issues shipped within this duration. Only 1 in 20 took longer.

These are useful for setting expectations or SLAs. If your P95 lead time is 30 days, "virtually all issues ship within a month" is accurate 19 times out of 20.

Percentiles require at least 5 data points. Below that threshold, values are omitted.

## Outlier detection

The tool flags issues that took unusually long compared to their peers using the interquartile range (IQR) method (the same approach used in box-and-whisker plots):

1. Sort all durations and find Q1 (25th percentile) and Q3 (75th percentile)
2. Compute IQR = Q3 - Q1 (the spread of the middle 50%)
3. Set the outlier threshold at Q3 + 1.5 * IQR
4. Flag any value above the threshold

This method adapts to your data. A team with consistently long lead times has a higher threshold than a team that ships fast. An issue is only flagged as an outlier relative to the other issues in the same release.

In practice: if most issues in a release close in 5--15 days, a 60-day issue would be flagged. But if your issues typically take 30--90 days, 60 days would not be flagged because it falls within the normal range.

Outlier detection requires at least 4 data points. Flagged issues appear with `OUTLIER` in pretty and markdown output, and `lead_time_outlier: true` in JSON output.

## Standard deviation and predictability (CV)

Standard deviation measures how spread out durations are -- "on average, how far is each item from the mean?" A small standard deviation means items cluster tightly; a large one means they are all over the map. The tool uses sample standard deviation (divides by N-1, not N).

Raw standard deviation is hard to interpret alone. "The standard deviation of our lead times is 20 days" prompts the question "20 days compared to what?" A 20-day spread when your mean is 5 days is chaotic. A 20-day spread when your mean is 200 days is rock-solid.

### Coefficient of variation (CV)

The **coefficient of variation** answers "compared to what?" by dividing:

```
CV = standard deviation / mean
```

The result is a dimensionless ratio. A CV of 0.3 means the standard deviation is 30% of the mean. A CV of 2.0 means the standard deviation is twice the mean.

**Why CV instead of raw standard deviation?** Issue durations naturally span a huge range. A backlog might contain a 20-minute typo fix and a 3-month platform migration, both closing in the same sprint. Standard deviation alone cannot tell you whether the spread is "a lot" or "normal for the scale." CV normalizes the spread so you can compare across time windows, teams, and repositories -- even when absolute durations differ greatly.

An analogy: two pizza delivery services. Service A delivers in 25--35 minutes (mean 30, stddev 5, CV 0.17). Service B delivers in 10--60 minutes (mean 30, stddev 18, CV 0.60). Both have the same mean, but Service B is far less predictable. CV captures this; raw standard deviation alone might mislead if the means differed.

### How to read the predictability label

gh-velocity translates CV into a plain-language **predictability** label so you do not have to remember thresholds:

| CV | Label | What it tells you |
|----|-------|-------------------|
| < 0.5 | _(not shown)_ | Delivery times are consistent. The median is a solid estimate. |
| 0.5 -- 1.0 | **moderate** | Noticeable variation. Most items land near the median, but some take 2--3x longer. Investigate the slow ones. |
| > 1.0 | **low** | The spread exceeds the average. Predicting any single item's duration is unreliable. The median is a better anchor than the mean, but expect surprises. |

### When is high variability OK?

High variability is not automatically bad. Consider two scenarios:

1. **Mixed-size backlog** (bugs + features + epics): A team closing 1-hour hotfixes and 3-week features in the same window naturally has a high CV. This is expected. Takeaway: don't quote the mean as a delivery estimate.

2. **Uniform sprint work** (all items roughly the same effort): If items typically take 2--5 days, a CV of 1.5 means something is off -- some items are ballooning. Takeaway: investigate the slow outliers.

The predictability label helps you spot the second scenario without doing arithmetic.

### Worked example

From a real repository (the astral-sh/uv report above):

| Measure | Value |
|---------|-------|
| Mean lead time | 5h 56m |
| Median lead time | 2h 35m |
| Standard deviation | 8h 33m |
| CV | 1.4 |
| Predictability | low |

A CV of 1.4 means the spread is 1.4 times the mean. The "typical" issue takes about 2.5 hours (median), but variation is so large that some items take 10x longer. The tool surfaces this as:

> Delivery times vary widely (CV 1.4) -- the median is a more reliable estimate than the mean.

If you looked only at the mean (6 hours), you might set expectations that most items take half a day. The median (2.5 hours) is closer to what people actually experience. The CV tells you the mean is distorted.

### A more extreme example

From cli/cli v2.67.0:

| Measure | Value |
|---------|-------|
| Mean lead time | 280 days |
| Median lead time | 60 days |
| Standard deviation | ~1,300 days |
| CV | ~4.6 |
| Predictability | low |

A CV of 4.6 means the standard deviation is nearly five times the mean. Two issues open for 4+ years were closed alongside recent work. The mean (280 days) is wildly misleading; the median (60 days) is the number to quote. The "low" predictability label tells you this at a glance.

### CV in JSON output

JSON output includes both the raw CV value and the label:

```json
{
  "cv": 1.4,
  "predictability": "low"
}
```

Use these in scripts or dashboards. For example, to alert when predictability drops:

```bash
gh velocity flow lead-time --since 30d --results json | \
  jq -r 'if .stats.cv > 1.0 then "WARNING: low predictability (CV \(.stats.cv))" else "OK" end'
```

Standard deviation and CV require at least 2 data points.

## See also

- [Interpreting Results]({{< relref "/guides/interpreting-results" >}}) -- what "good" looks like for each metric
- [Lead Time Reference]({{< relref "/reference/metrics/lead-time" >}}) -- metric definition and aggregation details
- [Quality Metrics]({{< relref "/reference/metrics/quality" >}}) -- outlier detection in release reports
