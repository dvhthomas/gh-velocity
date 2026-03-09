package metrics

import (
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// ReleaseInput holds the data needed to compute release metrics.
type ReleaseInput struct {
	Tag          string
	PreviousTag  string
	Release      model.Release
	PrevRelease  *model.Release // nil if no previous release
	IssueCommits map[int][]model.Commit
	Issues       map[int]*model.Issue // successfully fetched issues
	FetchErrors  map[int]error        // issues that failed to fetch
	BugLabels    []string
	FeatureLabels []string
	HotfixWindowHours float64
}

// BuildReleaseMetrics computes all release metrics from the provided input.
// Returns the metrics, a list of warnings, and any error.
func BuildReleaseMetrics(input ReleaseInput) (model.ReleaseMetrics, []string, error) {
	var warnings []string

	// Collect fetch errors as warnings (skip-and-warn partial failure strategy)
	for num, fetchErr := range input.FetchErrors {
		warnings = append(warnings, fmt.Sprintf("skipped issue #%d: %v", num, fetchErr))
	}
	if n := len(input.FetchErrors); n > 0 {
		warnings = append(warnings, fmt.Sprintf("%d issue(s) skipped due to fetch errors", n))
	}

	// Build per-issue metrics and collect durations for aggregation
	var issueMetrics []model.IssueMetrics
	var leadTimes, cycleTimes, releaseLags []time.Duration

	for issueNum, issueCommitList := range input.IssueCommits {
		issue, ok := input.Issues[issueNum]
		if !ok {
			continue // already recorded as fetch error
		}

		im := model.IssueMetrics{
			Issue:       *issue,
			CommitCount: len(issueCommitList),
		}

		// Lead time: created -> closed
		lt := LeadTime(*issue)
		im.LeadTime = lt
		if lt != nil {
			leadTimes = append(leadTimes, *lt)
		}

		// Cycle time: first commit -> closed
		if len(issueCommitList) > 0 {
			firstCommit := issueCommitList[len(issueCommitList)-1].AuthoredAt // commits are newest-first
			var endTime time.Time
			if issue.ClosedAt != nil {
				endTime = *issue.ClosedAt
			}
			ct := CycleTime(firstCommit, endTime)
			im.CycleTime = ct
			if ct != nil {
				cycleTimes = append(cycleTimes, *ct)
			}
		}

		// Release lag: closed -> release date
		if issue.ClosedAt != nil {
			lag := input.Release.CreatedAt.Sub(*issue.ClosedAt)
			im.ReleaseLag = &lag
			releaseLags = append(releaseLags, lag)
		}

		issueMetrics = append(issueMetrics, im)
	}

	// Single-pass label classification: counts + ratios + low-label-coverage warning
	var bugCount, featureCount, otherCount int
	for _, im := range issueMetrics {
		if hasAnyLabel(im.Issue.Labels, input.BugLabels) {
			bugCount++
		} else if hasAnyLabel(im.Issue.Labels, input.FeatureLabels) {
			featureCount++
		} else {
			otherCount++
		}
	}

	total := len(issueMetrics)
	var bugRatio, featureRatio, otherRatio float64
	if total > 0 {
		ft := float64(total)
		bugRatio = float64(bugCount) / ft
		featureRatio = float64(featureCount) / ft
		otherRatio = float64(otherCount) / ft

		// Low label coverage warning: "other" means unlabeled (no bug/feature label)
		if float64(otherCount)/ft > 0.5 {
			warnings = append(warnings, fmt.Sprintf("Low label coverage: %d/%d issues have no bug/feature labels", otherCount, total))
		}
	}

	// Hotfix detection
	var isHotfix bool
	var cadence *time.Duration
	if input.PrevRelease != nil && input.PrevRelease.TagName != "" {
		c := input.Release.CreatedAt.Sub(input.PrevRelease.CreatedAt)
		cadence = &c
		isHotfix = IsHotfix(input.Release, *input.PrevRelease, input.HotfixWindowHours)
	}

	rm := model.ReleaseMetrics{
		Tag:             input.Tag,
		PreviousTag:     input.PreviousTag,
		Date:            input.Release.CreatedAt,
		Cadence:         cadence,
		IsHotfix:        isHotfix,
		Issues:          issueMetrics,
		TotalIssues:     total,
		BugCount:        bugCount,
		FeatureCount:    featureCount,
		OtherCount:      otherCount,
		BugRatio:        bugRatio,
		FeatureRatio:    featureRatio,
		OtherRatio:      otherRatio,
		LeadTimeStats:   ComputeStats(leadTimes),
		CycleTimeStats:  ComputeStats(cycleTimes),
		ReleaseLagStats: ComputeStats(releaseLags),
	}

	return rm, warnings, nil
}
