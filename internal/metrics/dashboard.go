package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/cycletime"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"golang.org/x/sync/errgroup"
)

// DashboardClient defines the API methods needed for dashboard computation.
type DashboardClient interface {
	SearchClosedIssues(ctx context.Context, since, until time.Time) ([]model.Issue, error)
	SearchMergedPRs(ctx context.Context, since, until time.Time) ([]model.PR, error)
	ListProjectItems(ctx context.Context, projectID, statusFieldID string) ([]model.ProjectItem, error)
	SearchOpenIssuesWithLabels(ctx context.Context, labels []string) ([]model.Issue, error)
	FetchPRLinkedIssues(ctx context.Context, prNumbers []int) (map[int][]model.Issue, error)
}

// DashboardInput holds configuration for dashboard computation.
type DashboardInput struct {
	Repo              string
	Since             time.Time
	Until             time.Time
	Now               time.Time
	CycleTimeStrategy cycletime.Strategy
	CycleTimeLabel    string // "issue", "pr", or "project-board"
	ProjectID         string
	StatusFieldID     string
	BacklogStatus     string
	DoneStatus        string
	ActiveLabels      []string
	BacklogLabels     []string
	BugLabels         []string
}

// ComputeDashboard gathers all dashboard sections concurrently with graceful degradation.
// Independent API calls run in parallel; compute phases share fetched data.
func ComputeDashboard(ctx context.Context, client DashboardClient, input DashboardInput) model.StatsResult {
	result := model.StatsResult{
		Repository: input.Repo,
		Since:      input.Since,
		Until:      input.Until,
	}

	// --- Phase 1: Parallel API fetches ---
	var (
		closedIssues []model.Issue
		mergedPRs    []model.PR
		closingPRs   map[int]*model.PR
		wipCount     *int
		warnings     []string
		mu           sync.Mutex
	)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	// Fetch closed issues
	g.Go(func() error {
		issues, err := client.SearchClosedIssues(gctx, input.Since, input.Until)
		if err != nil {
			mu.Lock()
			warnings = append(warnings, fmt.Sprintf("could not fetch closed issues: %v", err))
			mu.Unlock()
			return nil
		}
		mu.Lock()
		closedIssues = issues
		mu.Unlock()
		return nil
	})

	// Fetch merged PRs and build closing PR map.
	g.Go(func() error {
		prs, err := client.SearchMergedPRs(gctx, input.Since, input.Until)
		if err != nil {
			mu.Lock()
			warnings = append(warnings, fmt.Sprintf("could not fetch merged PRs: %v", err))
			mu.Unlock()
			return nil
		}
		prMap := buildClosingPRMap(gctx, client, prs)
		mu.Lock()
		mergedPRs = prs
		closingPRs = prMap
		mu.Unlock()
		return nil
	})

	// Fetch WIP (optional)
	if input.ProjectID != "" {
		g.Go(func() error {
			projectItems, err := client.ListProjectItems(gctx, input.ProjectID, input.StatusFieldID)
			if err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("could not fetch WIP from project board: %v", err))
				mu.Unlock()
				return nil
			}
			count := 0
			for _, pi := range projectItems {
				if pi.Status != input.BacklogStatus && pi.Status != input.DoneStatus {
					count++
				}
			}
			mu.Lock()
			wipCount = &count
			mu.Unlock()
			return nil
		})
	} else if len(input.ActiveLabels) > 0 {
		g.Go(func() error {
			activeIssues, err := client.SearchOpenIssuesWithLabels(gctx, input.ActiveLabels)
			if err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("could not fetch WIP from labels: %v", err))
				mu.Unlock()
				return nil
			}
			backlogSet := make(map[string]bool)
			for _, l := range input.BacklogLabels {
				backlogSet[l] = true
			}
			count := 0
			for _, issue := range activeIssues {
				hasBacklog := false
				for _, l := range issue.Labels {
					if backlogSet[l] {
						hasBacklog = true
						break
					}
				}
				if !hasBacklog {
					count++
				}
			}
			mu.Lock()
			wipCount = &count
			mu.Unlock()
			return nil
		})
	}

	if waitErr := g.Wait(); waitErr != nil {
		warnings = append(warnings, fmt.Sprintf("stats fetch error: %v", waitErr))
	}
	result.Warnings = warnings

	// --- Phase 2: Compute metrics from fetched data ---
	if closedIssues == nil {
		return result
	}

	// Lead Time
	var leadDurations []time.Duration
	for _, issue := range closedIssues {
		m := LeadTime(issue)
		if m.Duration != nil {
			leadDurations = append(leadDurations, *m.Duration)
		}
	}
	leadStats := ComputeStats(leadDurations)
	result.LeadTime = &leadStats

	// Cycle Time
	if closingPRs == nil {
		closingPRs = make(map[int]*model.PR)
	}

	var cycleDurations []time.Duration
	for _, issue := range closedIssues {
		ctInput := cycletime.Input{Issue: &issue}
		if pr, ok := closingPRs[issue.Number]; ok {
			ctInput.PR = pr
		}
		ct := input.CycleTimeStrategy.Compute(ctx, ctInput)
		if ct.Duration != nil {
			cycleDurations = append(cycleDurations, *ct.Duration)
		}
	}
	cycleStats := ComputeStats(cycleDurations)
	result.CycleTime = &cycleStats
	result.CycleTimeStrategy = input.CycleTimeLabel

	// Throughput
	result.Throughput = &model.StatsThroughput{
		IssuesClosed: len(closedIssues),
		PRsMerged:    len(mergedPRs),
	}

	// WIP
	result.WIPCount = wipCount

	// Quality: defect rate from bug labels
	bugLabels := make(map[string]bool)
	for _, l := range input.BugLabels {
		bugLabels[l] = true
	}
	bugCount := 0
	for _, issue := range closedIssues {
		for _, l := range issue.Labels {
			if bugLabels[l] {
				bugCount++
				break
			}
		}
	}
	if len(closedIssues) > 0 {
		defectRate := float64(bugCount) / float64(len(closedIssues))
		result.Quality = &model.StatsQuality{
			BugCount:    bugCount,
			TotalIssues: len(closedIssues),
			DefectRate:  defectRate,
		}
	}

	return result
}

// buildClosingPRMap builds a map from issue number to closing PR by fetching linked issues.
func buildClosingPRMap(ctx context.Context, client DashboardClient, mergedPRs []model.PR) map[int]*model.PR {
	closingPRs := make(map[int]*model.PR)
	if len(mergedPRs) == 0 {
		return closingPRs
	}

	prNumbers := make([]int, len(mergedPRs))
	prMap := make(map[int]*model.PR)
	for i, pr := range mergedPRs {
		prNumbers[i] = pr.Number
		prCopy := pr
		prMap[pr.Number] = &prCopy
	}

	linkedIssues, err := client.FetchPRLinkedIssues(ctx, prNumbers)
	if err != nil {
		// Graceful: return empty map, caller will proceed without PR data.
		return closingPRs
	}

	for prNum, issues := range linkedIssues {
		for _, issue := range issues {
			closingPRs[issue.Number] = prMap[prNum]
		}
	}
	return closingPRs
}
