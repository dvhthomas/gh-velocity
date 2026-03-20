package wip

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/config"
	"github.com/dvhthomas/gh-velocity/internal/effort"
	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/metrics"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// searcher is a narrow interface for searching issues and PRs.
type searcher interface {
	SearchIssues(ctx context.Context, query string) ([]model.Issue, error)
	SearchPRs(ctx context.Context, query string) ([]model.PR, error)
}

// Pipeline implements the three-phase pipeline for WIP detail reporting.
type Pipeline struct {
	// Config
	Client          searcher
	Owner, Repo     string
	LifecycleConfig config.LifecycleConfig
	EffortConfig    config.EffortConfig
	WIPConfig       config.WIPConfig
	Scope           string
	Now             time.Time
	Debug           bool
	WarnFunc        func(string, ...any) // for logging warnings

	// Data injection (report context) — set these instead of calling GatherData via API.
	InjectedIssues []model.Issue
	InjectedPRs    []model.PR

	// Internal
	openIssues []model.Issue
	openPRs    []model.PR

	// Output
	Result   model.WIPResult
	Warnings []string
}

// warn appends a warning and calls WarnFunc if set.
func (p *Pipeline) warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	p.Warnings = append(p.Warnings, msg)
	if p.WarnFunc != nil {
		p.WarnFunc("%s", msg)
	}
}

// GatherData fetches open issues and PRs for WIP analysis.
// If InjectedIssues/InjectedPRs are set, uses them directly (report context).
// Otherwise queries the GitHub Search API (standalone context).
func (p *Pipeline) GatherData(ctx context.Context) error {
	if p.InjectedIssues != nil || p.InjectedPRs != nil {
		p.openIssues = p.InjectedIssues
		p.openPRs = p.InjectedPRs
		return nil
	}

	inProgressMatchers := p.LifecycleConfig.InProgress.Match
	inReviewMatchers := p.LifecycleConfig.InReview.Match

	// Extract label names from matchers for search queries.
	var labelNames []string
	for _, matcherStr := range append(inProgressMatchers, inReviewMatchers...) {
		if strings.HasPrefix(matcherStr, "label:") {
			labelNames = append(labelNames, matcherStr[len("label:"):])
		}
	}

	// Build base scope.
	var scopePrefix string
	if p.Scope != "" {
		scopePrefix = p.Scope
	} else if p.Owner != "" && p.Repo != "" {
		scopePrefix = fmt.Sprintf("repo:%s/%s", p.Owner, p.Repo)
	}

	seenIssues := make(map[int]bool)
	seenPRs := make(map[int]bool)

	// Search per label for issues (OR semantics via separate queries).
	for _, label := range labelNames {
		query := buildQuery(scopePrefix, "is:open", "is:issue", fmt.Sprintf("label:%q", label))
		issues, err := p.Client.SearchIssues(ctx, query)
		if err != nil {
			p.warn("wip: search for issue label %q failed: %v", label, err)
			continue
		}
		if len(issues) >= 1000 {
			p.warn("wip: issue search for label %q returned 1000 results (GitHub cap) — some items may be missing", label)
		}
		for _, issue := range issues {
			if !seenIssues[issue.Number] {
				seenIssues[issue.Number] = true
				p.openIssues = append(p.openIssues, issue)
			}
		}
	}

	// Search per label for PRs.
	for _, label := range labelNames {
		query := buildQuery(scopePrefix, "is:open", "is:pr", fmt.Sprintf("label:%q", label))
		prs, err := p.Client.SearchPRs(ctx, query)
		if err != nil {
			p.warn("wip: search for PR label %q failed: %v", label, err)
			continue
		}
		if len(prs) >= 1000 {
			p.warn("wip: PR search for label %q returned 1000 results (GitHub cap) — some items may be missing", label)
		}
		for _, pr := range prs {
			if !seenPRs[pr.Number] {
				seenPRs[pr.Number] = true
				p.openPRs = append(p.openPRs, pr)
			}
		}
	}

	// Extra query for unlabeled open PRs (native signal fallback).
	unlabeledQuery := buildQuery(scopePrefix, "is:open", "is:pr", "no:label")
	unlabeledPRs, err := p.Client.SearchPRs(ctx, unlabeledQuery)
	if err != nil {
		p.warn("wip: search for unlabeled PRs failed: %v", err)
	} else {
		if len(unlabeledPRs) >= 1000 {
			p.warn("wip: unlabeled PR search returned 1000 results (GitHub cap) — some items may be missing")
		}
		for _, pr := range unlabeledPRs {
			if !seenPRs[pr.Number] {
				seenPRs[pr.Number] = true
				p.openPRs = append(p.openPRs, pr)
			}
		}
	}

	return nil
}

// ProcessData classifies items, computes metrics, and generates insights.
func (p *Pipeline) ProcessData() error {
	inProgressMatchers := p.LifecycleConfig.InProgress.Match
	inReviewMatchers := p.LifecycleConfig.InReview.Match

	// Build effort evaluator.
	eval, err := effort.NewEvaluator(p.EffortConfig)
	if err != nil {
		return fmt.Errorf("wip: effort evaluator: %w", err)
	}

	now := p.Now
	var items []model.WIPItem

	// Classify issues.
	for _, issue := range p.openIssues {
		stage, matched, excluded := classifyItem(
			issue.Labels, issue.IssueType, issue.Title,
			false, false,
			inProgressMatchers, inReviewMatchers,
		)
		if excluded {
			continue
		}

		item := toWIPItemFromIssue(issue, stage, matched, now)

		// Evaluate effort.
		effortVal, _ := eval.Evaluate(effort.Item{
			Labels:    issue.Labels,
			IssueType: issue.IssueType,
			Title:     issue.Title,
		})
		item.EffortValue = effortVal

		items = append(items, item)
	}

	// Classify PRs.
	for _, pr := range p.openPRs {
		stage, matched, excluded := classifyItem(
			pr.Labels, "", pr.Title,
			true, pr.Draft,
			inProgressMatchers, inReviewMatchers,
		)
		if excluded {
			continue
		}

		item := toWIPItemFromPR(pr, stage, matched, now)

		// Evaluate effort.
		effortVal, _ := eval.Evaluate(effort.Item{
			Labels: pr.Labels,
			Title:  pr.Title,
		})
		item.EffortValue = effortVal

		items = append(items, item)
	}

	// Compute aggregates.
	stageCounts := metrics.ComputeWIPStageCounts(items, inProgressMatchers, inReviewMatchers)
	assignees := metrics.ComputeWIPAssignees(items, 10)
	staleness := metrics.ComputeWIPStaleness(items)

	// Total effort.
	var totalEffort float64
	for _, item := range items {
		totalEffort += item.EffortValue
	}

	// Check WIP limits and generate limit warnings.
	if p.WIPConfig.TeamLimit != nil && totalEffort > *p.WIPConfig.TeamLimit {
		p.warn("wip: team WIP limit exceeded (%.0f items, limit %.0f)", totalEffort, *p.WIPConfig.TeamLimit)
	}
	if p.WIPConfig.PersonLimit != nil {
		for i := range assignees {
			if assignees[i].TotalEffort > *p.WIPConfig.PersonLimit {
				assignees[i].OverLimit = true
				p.warn("wip: %s exceeds person WIP limit (%.0f items, limit %.0f)",
					assignees[i].Login, assignees[i].TotalEffort, *p.WIPConfig.PersonLimit)
			}
		}
	}

	// Assemble result.
	p.Result = model.WIPResult{
		Repository:  fmt.Sprintf("%s/%s", p.Owner, p.Repo),
		Items:       items,
		StageCounts: stageCounts,
		Assignees:   assignees,
		Staleness:   staleness,
		TotalEffort: totalEffort,
		TeamLimit:   p.WIPConfig.TeamLimit,
		PersonLimit: p.WIPConfig.PersonLimit,
		Warnings:    p.Warnings,
	}

	// Generate insights.
	p.Result.Insights = metrics.GenerateWIPInsights(p.Result)

	return nil
}

// Render writes the processed result. Stub for Phase 4.
func (p *Pipeline) Render(_ format.RenderContext) error {
	return nil
}

// --- helpers ---

func buildQuery(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, " ")
}

func toWIPItemFromIssue(issue model.Issue, stage, matchedMatcher string, now time.Time) model.WIPItem {
	return model.WIPItem{
		Number:         issue.Number,
		Title:          issue.Title,
		Status:         stage,
		MatchedMatcher: matchedMatcher,
		Age:            now.Sub(issue.CreatedAt),
		Kind:           "ISSUE",
		URL:            issue.URL,
		Labels:         issue.Labels,
		Assignees:      issue.Assignees,
		UpdatedAt:      issue.UpdatedAt,
		Staleness:      metrics.ComputeStaleness(issue.UpdatedAt, now),
	}
}

func toWIPItemFromPR(pr model.PR, stage, matchedMatcher string, now time.Time) model.WIPItem {
	return model.WIPItem{
		Number:         pr.Number,
		Title:          pr.Title,
		Status:         stage,
		MatchedMatcher: matchedMatcher,
		Age:            now.Sub(pr.CreatedAt),
		Kind:           "PR",
		URL:            pr.URL,
		Labels:         pr.Labels,
		Assignees:      pr.Assignees,
		UpdatedAt:      pr.UpdatedAt,
		Staleness:      metrics.ComputeStaleness(pr.UpdatedAt, now),
	}
}
