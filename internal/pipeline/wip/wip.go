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
	ExcludeUsers    []string // from config, used for bot detection
	Scope           string
	Now             time.Time
	Debug           bool
	WarnFunc        func(string, ...any) // for logging warnings

	// Data injection (report context) — set these instead of calling GatherData via API.
	InjectedIssues []model.Issue
	InjectedPRs    []model.PR

	// Truncated is set when any query returns 1000 results (API cap).
	// Set by GatherData (standalone) or by the caller for report context
	// (from throughput pipeline warnings).
	Truncated bool

	// Internal — OpenIssues exported for enrichment at cmd/ layer.
	OpenIssues []model.Issue
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
		p.OpenIssues = p.InjectedIssues
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
			p.Truncated = true
			p.warn("wip: issue search for label %q returned 1000 results (GitHub cap) — some items may be missing", label)
		}
		for _, issue := range issues {
			if !seenIssues[issue.Number] {
				seenIssues[issue.Number] = true
				p.OpenIssues = append(p.OpenIssues, issue)
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
			p.Truncated = true
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
			p.Truncated = true
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
// When called without GatherData (e.g., from report with injected data),
// populates working fields from InjectedIssues/InjectedPRs.
func (p *Pipeline) ProcessData() error {
	// Populate working fields from injected data when GatherData was not called.
	if p.OpenIssues == nil && p.InjectedIssues != nil {
		p.OpenIssues = p.InjectedIssues
	}
	if p.openPRs == nil && p.InjectedPRs != nil {
		p.openPRs = p.InjectedPRs
	}

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
	for _, issue := range p.OpenIssues {
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
	allAssignees := metrics.ComputeWIPAssignees(items, 0, p.WIPConfig.Bots, p.ExcludeUsers) // no limit yet, partition first
	staleness := metrics.ComputeWIPStaleness(items)

	// Partition assignees into human and bot.
	humanAssignees, botAssignees := metrics.PartitionAssignees(allAssignees, 10)

	// Classify items as human or bot.
	humanItems, botItems := metrics.ClassifyItemsByBot(items, p.WIPConfig.Bots, p.ExcludeUsers)

	// Total effort.
	var totalEffort, humanEffort, botEffort float64
	for _, item := range items {
		totalEffort += item.EffortValue
	}
	for _, item := range humanItems {
		humanEffort += item.EffortValue
	}
	for _, item := range botItems {
		botEffort += item.EffortValue
	}

	// Human/bot staleness.
	humanStaleness := metrics.ComputeWIPStaleness(humanItems)
	botStaleness := metrics.ComputeWIPStaleness(botItems)

	// Check WIP limits — apply only to human effort.
	if p.WIPConfig.TeamLimit != nil && humanEffort > *p.WIPConfig.TeamLimit {
		p.warn("wip: human WIP exceeds team limit (%.0f items, limit %.0f)", humanEffort, *p.WIPConfig.TeamLimit)
	}
	if p.WIPConfig.PersonLimit != nil {
		for i := range humanAssignees {
			if humanAssignees[i].TotalEffort > *p.WIPConfig.PersonLimit {
				humanAssignees[i].OverLimit = true
				p.warn("wip: %s exceeds person WIP limit (%.0f items, limit %.0f)",
					humanAssignees[i].Login, humanAssignees[i].TotalEffort, *p.WIPConfig.PersonLimit)
			}
		}
	}

	// Assemble result.
	p.Result = model.WIPResult{
		Repository:     fmt.Sprintf("%s/%s", p.Owner, p.Repo),
		Items:          items,
		StageCounts:    stageCounts,
		Assignees:      humanAssignees,
		BotAssignees:   botAssignees,
		Staleness:      staleness,
		HumanStaleness: humanStaleness,
		BotStaleness:   botStaleness,
		TotalEffort:    totalEffort,
		HumanEffort:    humanEffort,
		BotEffort:      botEffort,
		HumanItemCount: len(humanItems),
		BotItemCount:   len(botItems),
		TeamLimit:      p.WIPConfig.TeamLimit,
		PersonLimit:    p.WIPConfig.PersonLimit,
		Truncated:      p.Truncated,
		Warnings:       p.Warnings,
	}

	// Generate insights.
	p.Result.Insights = metrics.GenerateWIPInsights(p.Result)

	return nil
}

// Render writes the processed result in the requested format.
func (p *Pipeline) Render(rc format.RenderContext) error {
	switch rc.Format {
	case format.JSON:
		return format.WriteWIPDetailJSON(rc.Writer, p.Result)
	case format.Markdown:
		return format.WriteWIPDetailMarkdown(rc, p.Result)
	default:
		return format.WriteWIPDetailPretty(rc, p.Result)
	}
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
	// PRs use Author as WIP owner (not Assignees, which are rarely set on PRs).
	// Fall-through: Author → Assignees → unassigned.
	var assignees []string
	if pr.Author != "" {
		assignees = []string{pr.Author}
	} else if len(pr.Assignees) > 0 {
		assignees = pr.Assignees
	}
	return model.WIPItem{
		Number:         pr.Number,
		Title:          pr.Title,
		Status:         stage,
		MatchedMatcher: matchedMatcher,
		Age:            now.Sub(pr.CreatedAt),
		Kind:           "PR",
		URL:            pr.URL,
		Labels:         pr.Labels,
		Assignees:      assignees,
		UpdatedAt:      pr.UpdatedAt,
		Staleness:      metrics.ComputeStaleness(pr.UpdatedAt, now),
	}
}
