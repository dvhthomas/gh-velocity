package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/classify"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// ReleaseInput holds the data needed to compute release metrics.
type ReleaseInput struct {
	Tag               string
	PreviousTag       string
	Release           model.Release
	PrevRelease       *model.Release // nil if no previous release
	IssueCommits      map[int][]model.Commit
	Issues            map[int]*model.Issue // successfully fetched issues
	LinkedPRs         map[int]*model.PR    // issue number → linked PR (may be nil)
	FetchErrors       map[int]error        // issues that failed to fetch
	Classifier        *classify.Classifier
	HotfixWindowHours float64
	CycleTimeStrategy CycleTimeStrategy // nil falls back to commit-based default
}

// BuildReleaseMetrics computes all release metrics from the provided input.
// Returns the metrics, a list of warnings, and any error.
func BuildReleaseMetrics(ctx context.Context, input ReleaseInput) (model.ReleaseMetrics, []string, error) {
	var warnings []string

	// Collect fetch errors as warnings (skip-and-warn partial failure strategy)
	for num, fetchErr := range input.FetchErrors {
		warnings = append(warnings, fmt.Sprintf("skipped issue #%d: %v", num, fetchErr))
	}
	if n := len(input.FetchErrors); n > 0 {
		warnings = append(warnings, fmt.Sprintf("%d issue(s) skipped due to fetch errors", n))
	}

	releaseEnd := &model.Event{
		Time:   input.Release.CreatedAt,
		Signal: model.SignalReleasePublished,
		Detail: input.Tag,
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
		im.LeadTime = LeadTime(*issue)
		if im.LeadTime.Duration != nil {
			leadTimes = append(leadTimes, *im.LeadTime.Duration)
		}

		// Cycle time: computed by the configured strategy.
		// Commits enrich output but do not determine start/end signals.
		if input.CycleTimeStrategy != nil {
			ctInput := CycleTimeInput{
				Issue:   issue,
				Commits: issueCommitList,
			}
			if input.LinkedPRs != nil {
				ctInput.PR = input.LinkedPRs[issueNum]
			}
			im.CycleTime = input.CycleTimeStrategy.Compute(ctx, ctInput)
			if im.CycleTime.Duration != nil {
				cycleTimes = append(cycleTimes, *im.CycleTime.Duration)
			}
		}

		// Release lag: closed -> release date
		if issue.ClosedAt != nil {
			closedEvent := &model.Event{
				Time:   *issue.ClosedAt,
				Signal: model.SignalIssueClosed,
			}
			im.ReleaseLag = model.NewMetric(closedEvent, releaseEnd)
			if im.ReleaseLag.Duration != nil {
				releaseLags = append(releaseLags, *im.ReleaseLag.Duration)
			}
		}

		issueMetrics = append(issueMetrics, im)
	}

	// Classify each issue using the flexible classifier
	catNames := append(input.Classifier.CategoryNames(), "other")
	categoryCounts := make(map[string]int, len(catNames))
	for _, name := range catNames {
		categoryCounts[name] = 0
	}

	total := len(issueMetrics)
	for i, im := range issueMetrics {
		ci := classify.Input{
			Labels:    im.Issue.Labels,
			IssueType: im.Issue.IssueType,
			Title:     im.Issue.Title,
		}
		result := input.Classifier.Classify(ci)
		issueMetrics[i].Category = result.Category()
		categoryCounts[result.Category()]++
	}

	categoryRatios := make(map[string]float64, len(catNames))
	if total > 0 {
		ft := float64(total)
		for _, name := range catNames {
			categoryRatios[name] = float64(categoryCounts[name]) / ft
		}

		// Low label coverage warning: "other" means unclassified
		if float64(categoryCounts["other"])/ft > 0.5 {
			warnings = append(warnings, fmt.Sprintf("Low classification coverage: %d/%d issues are unclassified", categoryCounts["other"], total))
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

	// Compute stats first so we can flag outliers on individual issues
	ltStats := ComputeStats(leadTimes)
	ctStats := ComputeStats(cycleTimes)
	rlStats := ComputeStats(releaseLags)

	// Flag outlier issues using IQR method
	for i := range issueMetrics {
		issueMetrics[i].LeadTimeOutlier = IsOutlier(issueMetrics[i].LeadTime, ltStats)
		issueMetrics[i].CycleTimeOutlier = IsOutlier(issueMetrics[i].CycleTime, ctStats)
	}

	rm := model.ReleaseMetrics{
		Tag:             input.Tag,
		PreviousTag:     input.PreviousTag,
		Date:            input.Release.CreatedAt,
		Cadence:         cadence,
		IsHotfix:        isHotfix,
		Issues:          issueMetrics,
		TotalIssues:     total,
		CategoryNames:   catNames,
		CategoryCounts:  categoryCounts,
		CategoryRatios:  categoryRatios,
		LeadTimeStats:   ltStats,
		CycleTimeStats:  ctStats,
		ReleaseLagStats: rlStats,
	}

	return rm, warnings, nil
}
