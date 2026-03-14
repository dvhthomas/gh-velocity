// Package velocity implements the velocity metric pipeline.
// It measures effort completed per iteration (velocity) and completion rate.
package velocity

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/config"
	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/scope"
)

// Client is the narrow interface used by the velocity pipeline.
type Client interface {
	SearchIssues(ctx context.Context, query string) ([]model.Issue, error)
	SearchPRs(ctx context.Context, query string) ([]model.PR, error)
	ListIterationField(ctx context.Context, projectID, fieldName string) (*model.IterationFieldConfig, error)
	ListProjectItemsWithFields(ctx context.Context, projectID, iterFieldName, numFieldName string, singleSelectFields []string) ([]model.VelocityItem, error)
}

// Pipeline implements the velocity metric pipeline.
type Pipeline struct {
	Client         Client
	Owner, Repo    string
	Config         config.VelocityConfig
	ProjectConfig  config.ProjectConfig
	Scope          string
	ExcludeUsers   string
	Now            time.Time
	ShowCurrent    bool
	ShowHistory    bool
	IterationCount int
	Since, Until   *time.Time
	Verbose        bool

	// Internal state
	items     []model.VelocityItem
	periods   PeriodStrategy
	evaluator EffortEvaluator
	Result    model.VelocityResult
}

// GatherData fetches project items and resolves iteration boundaries.
func (p *Pipeline) GatherData(ctx context.Context) error {
	// Build effort evaluator.
	var err error
	p.evaluator, err = NewEffortEvaluator(p.Config.Effort)
	if err != nil {
		return fmt.Errorf("velocity: %w", err)
	}

	needsBoard := p.Config.Effort.Strategy == "numeric" ||
		p.Config.Iteration.Strategy == "project-field" ||
		HasFieldMatchers(p.Config.Effort)

	if needsBoard {
		return p.gatherFromBoard(ctx)
	}
	return p.gatherFromSearch(ctx)
}

func (p *Pipeline) gatherFromBoard(ctx context.Context) error {
	// Resolve project.
	projInfo, err := p.resolveProject(ctx)
	if err != nil {
		return err
	}

	// Resolve period strategy.
	if p.Config.Iteration.Strategy == "project-field" {
		iterCfg, err := p.Client.ListIterationField(ctx, projInfo.ProjectID, p.Config.Iteration.ProjectField)
		if err != nil {
			return fmt.Errorf("velocity: %w", err)
		}
		p.periods = &ProjectFieldPeriod{
			Active:    iterCfg.Iterations,
			Completed: iterCfg.CompletedIterations,
			Now:       p.Now,
		}
	} else {
		fp, err := NewFixedPeriod(p.Config.Iteration.Fixed, p.Now)
		if err != nil {
			return fmt.Errorf("velocity: %w", err)
		}
		p.periods = fp
	}

	// Fetch items with iteration and/or number fields.
	iterField := ""
	if p.Config.Iteration.Strategy == "project-field" {
		iterField = p.Config.Iteration.ProjectField
	}
	numField := ""
	if p.Config.Effort.Strategy == "numeric" {
		numField = p.Config.Effort.Numeric.ProjectField
	}

	ssFields := ExtractFieldMatcherNames(p.Config.Effort)

	items, err := p.Client.ListProjectItemsWithFields(ctx, projInfo.ProjectID, iterField, numField, ssFields)
	if err != nil {
		return fmt.Errorf("velocity: %w", err)
	}

	if len(items) > MaxBoardItems {
		log.Warn("board has %d items (limit: %d), truncating", len(items), MaxBoardItems)
		p.Result.Warnings = append(p.Result.Warnings,
			fmt.Sprintf("Board has %d items (limit: %d). Results may be incomplete. Consider: tighter --scope, shorter time range, or switch to label-based attribute strategy.",
				len(items), MaxBoardItems))
		items = items[:MaxBoardItems]
	}

	// Board is the scope — include all items, don't filter by repo.
	p.items = items

	return nil
}

func (p *Pipeline) gatherFromSearch(ctx context.Context) error {
	// Fixed period + count/attribute effort = search API.
	fp, err := NewFixedPeriod(p.Config.Iteration.Fixed, p.Now)
	if err != nil {
		return fmt.Errorf("velocity: %w", err)
	}
	p.periods = fp

	// Only fetch iterations we actually need based on flags.
	var allIters []model.Iteration

	if !p.ShowHistory {
		current, err := fp.Current()
		if err != nil {
			return fmt.Errorf("velocity: %w", err)
		}
		allIters = append(allIters, *current)
	}

	if !p.ShowCurrent {
		history, err := fp.Iterations(p.IterationCount)
		if err != nil {
			return fmt.Errorf("velocity: %w", err)
		}
		allIters = append(allIters, history...)
	}

	for _, iter := range allIters {
		items, err := p.fetchItemsForPeriod(ctx, iter.StartDate, iter.EndDate)
		if err != nil {
			return err
		}
		p.items = append(p.items, items...)
	}

	return nil
}

func (p *Pipeline) fetchItemsForPeriod(ctx context.Context, start, end time.Time) ([]model.VelocityItem, error) {
	if p.Config.Unit == "prs" {
		q := scope.MergedPRQuery(p.Scope, start, end)
		q.ExcludeUsers = p.ExcludeUsers
		prs, err := p.Client.SearchPRs(ctx, q.Build())
		if err != nil {
			return nil, fmt.Errorf("velocity: search PRs: %w", err)
		}
		return prsToVelocityItems(prs, start, end), nil
	}

	// Issues — use reason:completed filter in search.
	q := scope.ClosedIssueQuery(p.Scope, start, end)
	q.ExcludeUsers = p.ExcludeUsers
	issues, err := p.Client.SearchIssues(ctx, q.Build())
	if err != nil {
		return nil, fmt.Errorf("velocity: search issues: %w", err)
	}
	return issuesToVelocityItems(issues, start, end), nil
}

func issuesToVelocityItems(issues []model.Issue, start, end time.Time) []model.VelocityItem {
	items := make([]model.VelocityItem, 0, len(issues))
	for _, iss := range issues {
		item := model.VelocityItem{
			ContentType: "Issue",
			Number:      iss.Number,
			Title:       iss.Title,
			State:       iss.State,
			StateReason: iss.StateReason,
			ClosedAt:    iss.ClosedAt,
			CreatedAt:   iss.CreatedAt,
			Labels:      iss.Labels,
			IssueType:   iss.IssueType,
		}
		items = append(items, item)
	}
	return items
}

func prsToVelocityItems(prs []model.PR, start, end time.Time) []model.VelocityItem {
	items := make([]model.VelocityItem, 0, len(prs))
	for _, pr := range prs {
		item := model.VelocityItem{
			ContentType: "PullRequest",
			Number:      pr.Number,
			Title:       pr.Title,
			State:       pr.State,
			MergedAt:    pr.MergedAt,
			CreatedAt:   pr.CreatedAt,
			Labels:      pr.Labels,
		}
		items = append(items, item)
	}
	return items
}

func (p *Pipeline) resolveProject(ctx context.Context) (*gh.ProjectInfo, error) {
	ghClient, ok := p.Client.(*gh.Client)
	if !ok {
		return nil, fmt.Errorf("velocity: board features require a GitHub client")
	}
	info, err := ghClient.ResolveProject(ctx, p.ProjectConfig.URL, "")
	if err != nil {
		return nil, fmt.Errorf("velocity: %w", err)
	}
	return info, nil
}

// ProcessData computes velocity metrics for each iteration.
func (p *Pipeline) ProcessData() error {
	count := p.IterationCount

	var current *model.Iteration
	if !p.ShowHistory {
		c, err := p.periods.Current()
		if err != nil {
			log.Warn("velocity: %v", err)
		} else {
			current = c
		}
	}

	var history []model.Iteration
	if !p.ShowCurrent {
		h, err := p.periods.Iterations(count)
		if err != nil {
			return fmt.Errorf("velocity: %w", err)
		}
		history = h
	}

	// Filter iterations by --since/--until if specified.
	if p.Since != nil || p.Until != nil {
		history = filterIterations(history, p.Since, p.Until)
		if current != nil && !iterationOverlaps(*current, p.Since, p.Until) {
			current = nil
		}
	}

	// Determine effort unit label.
	effortUnit := "items"
	switch p.Config.Effort.Strategy {
	case "attribute", "numeric":
		effortUnit = "pts"
	}

	// Build effort detail for output.
	detail := model.EffortDetail{Strategy: p.Config.Effort.Strategy}
	switch p.Config.Effort.Strategy {
	case "attribute":
		for _, m := range p.Config.Effort.Attribute {
			detail.Matchers = append(detail.Matchers, model.EffortMatch{Query: m.Query, Value: m.Value})
		}
	case "numeric":
		detail.NumericField = p.Config.Effort.Numeric.ProjectField
	}

	p.Result = model.VelocityResult{
		Repository:   p.Owner + "/" + p.Repo,
		Unit:         p.Config.Unit,
		EffortUnit:   effortUnit,
		EffortDetail: detail,
	}

	if current != nil {
		iv := p.computeIteration(*current, nil)
		// Compute cycle position for current iteration.
		totalDays := int(current.EndDate.Sub(current.StartDate).Hours() / 24)
		dayOfCycle := int(p.Now.Sub(current.StartDate).Hours()/24) + 1 // 1-indexed: day 1 = first day
		if dayOfCycle < 1 {
			dayOfCycle = 1
		}
		if dayOfCycle > totalDays {
			dayOfCycle = totalDays
		}
		iv.DayOfCycle = dayOfCycle
		iv.TotalDays = totalDays
		p.Result.Current = &iv
	}

	var prevIter *model.Iteration
	for i := len(history) - 1; i >= 0; i-- {
		iv := p.computeIteration(history[i], prevIter)
		p.Result.History = append([]model.IterationVelocity{iv}, p.Result.History...)
		iter := history[i]
		prevIter = &iter
	}

	// Compute aggregate stats from history.
	if len(p.Result.History) > 0 {
		var sum, sumSq float64
		var compSum float64
		for _, h := range p.Result.History {
			sum += h.Velocity
			sumSq += h.Velocity * h.Velocity
			compSum += h.CompletionPct
		}
		n := float64(len(p.Result.History))
		p.Result.AvgVelocity = sum / n
		p.Result.AvgCompletion = compSum / n
		if n > 1 {
			variance := (sumSq - sum*sum/n) / (n - 1)
			if variance > 0 {
				p.Result.StdDev = math.Sqrt(variance)
			}
		}
	}

	// Generate insights from the computed data.
	p.generateInsights()

	return nil
}

// generateInsights derives human-readable observations from the velocity result.
func (p *Pipeline) generateInsights() {
	r := &p.Result

	// Check for not-assessed items.
	var totalNotAssessed int
	var totalItems int
	if r.Current != nil {
		totalNotAssessed += r.Current.NotAssessed
		totalItems += r.Current.ItemsTotal
	}
	for _, h := range r.History {
		totalNotAssessed += h.NotAssessed
		totalItems += h.ItemsTotal
	}
	if totalNotAssessed > 0 && totalItems > 0 {
		pct := float64(totalNotAssessed) / float64(totalItems) * 100
		if pct >= 100 {
			r.Insights = append(r.Insights, model.Insight{Message: fmt.Sprintf("All %d items lack effort estimates — velocity will be 0 until estimates are added.", totalNotAssessed)})
		} else if pct >= 50 {
			r.Insights = append(r.Insights, model.Insight{Message: fmt.Sprintf("%.0f%% of items (%d/%d) lack effort estimates — velocity may be understated.", pct, totalNotAssessed, totalItems)})
		}
	}

	// High completion rate.
	if r.Current != nil && r.Current.CompletionPct >= 100 && r.Current.ItemsTotal > 0 {
		r.Insights = append(r.Insights, model.Insight{Message: "Current iteration is 100% complete — all committed work is done."})
	}

	// Zero velocity across all history.
	if len(r.History) > 0 && r.AvgVelocity == 0 {
		r.Insights = append(r.Insights, model.Insight{Message: fmt.Sprintf("Zero velocity across %d iteration(s) — check effort strategy or date range.", len(r.History))})
	}

	// High variability.
	if r.AvgVelocity > 0 && r.StdDev > 0 {
		cv := r.StdDev / r.AvgVelocity
		if cv > 0.5 {
			r.Insights = append(r.Insights, model.Insight{Message: fmt.Sprintf("High velocity variability (CV=%.1f) — sprint commitments may be inconsistent.", cv)})
		}
	}
}

// computeIteration computes velocity metrics for a single iteration.
func (p *Pipeline) computeIteration(iter model.Iteration, prevIter *model.Iteration) model.IterationVelocity {
	iv := model.IterationVelocity{
		Name:  iter.Title,
		Start: iter.StartDate,
		End:   iter.EndDate,
	}

	// Find items for this iteration.
	iterItems := p.itemsForIteration(iter)

	var doneEffort, committedEffort float64
	var itemsDone, itemsTotal int
	var notAssessed int
	var notAssessedItems []int

	for _, item := range iterItems {
		itemsTotal++
		effort, assessed := p.evaluator.Evaluate(item)
		if !assessed {
			notAssessed++
			notAssessedItems = append(notAssessedItems, item.Number)
		}

		committedEffort += effort

		if p.isDone(item) {
			itemsDone++
			doneEffort += effort
		}
	}

	iv.Velocity = doneEffort
	iv.Committed = committedEffort
	iv.ItemsDone = itemsDone
	iv.ItemsTotal = itemsTotal
	iv.NotAssessed = notAssessed
	iv.NotAssessedItems = notAssessedItems

	if committedEffort > 0 {
		iv.CompletionPct = (doneEffort / committedEffort) * 100
	}

	// Trend.
	if prevIter != nil {
		prevIV := p.computeIterationVelocityOnly(*prevIter)
		switch {
		case iv.Velocity > prevIV:
			iv.Trend = "▲"
		case iv.Velocity < prevIV:
			iv.Trend = "▼"
		default:
			iv.Trend = "─"
		}
	}

	return iv
}

// computeIterationVelocityOnly returns just the velocity for trend comparison.
func (p *Pipeline) computeIterationVelocityOnly(iter model.Iteration) float64 {
	var total float64
	for _, item := range p.itemsForIteration(iter) {
		if p.isDone(item) {
			effort, _ := p.evaluator.Evaluate(item)
			total += effort
		}
	}
	return total
}

// itemsForIteration returns items belonging to the given iteration.
func (p *Pipeline) itemsForIteration(iter model.Iteration) []model.VelocityItem {
	var result []model.VelocityItem
	for _, item := range p.items {
		if p.Config.Iteration.Strategy == "project-field" {
			// Project-field: match by iteration ID.
			if item.IterationID == iter.ID {
				result = append(result, item)
			}
		} else {
			// Fixed: items closed/merged within the iteration window,
			// OR created before end and still open (committed).
			if p.itemInFixedPeriod(item, iter) {
				result = append(result, item)
			}
		}
	}
	return result
}

// itemInFixedPeriod checks if an item belongs to a fixed period.
// For search-based data, items were already fetched per-period, so we check
// by close/merge date falling within the iteration window.
func (p *Pipeline) itemInFixedPeriod(item model.VelocityItem, iter model.Iteration) bool {
	closeDate := item.ClosedAt
	if item.ContentType == "PullRequest" && item.MergedAt != nil {
		closeDate = item.MergedAt
	}
	if closeDate == nil {
		return false
	}
	return !closeDate.Before(iter.StartDate) && closeDate.Before(iter.EndDate)
}

// isDone checks if an item is "done" based on the work unit type.
func (p *Pipeline) isDone(item model.VelocityItem) bool {
	if p.Config.Unit == "prs" {
		return item.MergedAt != nil
	}
	// Issues: closed with reason:completed (or any closed for search-based
	// where reason:completed is already filtered by the search query).
	if item.ContentType == "Issue" {
		if item.StateReason != "" {
			return item.StateReason == "completed" || item.StateReason == "COMPLETED"
		}
		// If no state reason available (e.g., from search API which may not return it),
		// treat all closed issues as done since the search already filters by reason:completed.
		return item.State == "closed" || item.State == "CLOSED"
	}
	// PRs in issue mode: merged PRs count.
	return item.MergedAt != nil
}

func filterIterations(iters []model.Iteration, since, until *time.Time) []model.Iteration {
	var result []model.Iteration
	for _, iter := range iters {
		if iterationOverlaps(iter, since, until) {
			result = append(result, iter)
		}
	}
	return result
}

func iterationOverlaps(iter model.Iteration, since, until *time.Time) bool {
	if since != nil && iter.EndDate.Before(*since) {
		return false
	}
	if until != nil && iter.StartDate.After(*until) {
		return false
	}
	return true
}

// Render writes the velocity result in the requested format.
func (p *Pipeline) Render(rc format.RenderContext) error {
	switch rc.Format {
	case format.JSON:
		return WriteJSON(rc.Writer, p.Result)
	case format.Markdown:
		return WriteMarkdown(rc.Writer, p.Result)
	default:
		return WritePretty(rc.Writer, p.Result, p.Verbose)
	}
}
