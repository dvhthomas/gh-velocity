package posting

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// Target identifies where to post metrics output.
type Target int

const (
	// IssueComment posts as a comment on an issue.
	IssueComment Target = iota
	// PRComment posts as a comment on a PR.
	PRComment
	// DiscussionTarget posts as a GitHub Discussion.
	DiscussionTarget
	// IssueBody appends/updates a marked section in the issue/PR body.
	IssueBody
)

// PostOptions configures a single post operation.
type PostOptions struct {
	Command       string // "lead-time", "cycle-time", "report", "release"
	Context       string // "42", "pr-5", "30d", "v1.0", "2026-01-01..2026-02-01"
	Content       string // markdown body (already formatted)
	Target        Target
	Number        int    // issue/PR number (for comment targets)
	ForceNew      bool   // --new-post: skip search, always create
	CategoryID    string // Discussion category node ID (required for DiscussionTarget)
	Repo          string // "owner/repo" for discussion titles
	TitleTemplate string // Go template for discussion title (optional; uses default if empty)
}

// TitleData is the data available to discussion title templates.
type TitleData struct {
	Command string // e.g., "report", "release", "throughput"
	Repo    string // e.g., "cli/cli"
	Date    string // e.g., "2026-03-21"
}

// CommentClient abstracts the GitHub API calls needed by CommentPoster.
type CommentClient interface {
	ListComments(ctx context.Context, number int) ([]Comment, error)
	CreateComment(ctx context.Context, number int, body string) error
	UpdateComment(ctx context.Context, commentID int64, body string) error
}

// Comment mirrors github.Comment for the posting package.
type Comment struct {
	ID   int64
	Body string
}

// DiscussionClient abstracts the GitHub API calls needed by DiscussionPoster.
type DiscussionClient interface {
	SearchDiscussions(ctx context.Context, categoryID string, limit int) ([]Discussion, error)
	CreateDiscussion(ctx context.Context, categoryID, title, body string) (string, error)
	UpdateDiscussion(ctx context.Context, discussionID, body string) error
}

// Discussion mirrors github.Discussion for the posting package.
type Discussion struct {
	ID    string
	Title string
	Body  string
	URL   string
}

// Poster posts metric output to GitHub.
type Poster interface {
	Post(ctx context.Context, opts PostOptions) error
}

// CommentPoster posts metrics as issue/PR comments.
// DryRun prevents all writes — reads are still performed to show
// what would happen, but no mutations are executed.
type CommentPoster struct {
	Client CommentClient
	DryRun bool
}

// Post creates or updates a comment on the issue/PR identified by opts.Number.
// In dry-run mode, logs the intended action without executing it.
func (p *CommentPoster) Post(ctx context.Context, opts PostOptions) error {
	body := WrapWithMarker(opts.Command, opts.Context, opts.Content)

	if opts.ForceNew {
		if p.DryRun {
			log.Notice("[dry-run] Would create new comment on #%d (%d bytes)", opts.Number, len(body))
			return nil
		}
		if err := p.Client.CreateComment(ctx, opts.Number, body); err != nil {
			return &model.AppError{
				Code:    model.ErrPostFailed,
				Message: fmt.Sprintf("create comment on #%d: %v", opts.Number, err),
			}
		}
		log.Notice("Posted to #%d (new comment)", opts.Number)
		return nil
	}

	// Search existing comments for our marker.
	comments, err := p.Client.ListComments(ctx, opts.Number)
	if err != nil {
		return &model.AppError{
			Code:    model.ErrPostFailed,
			Message: fmt.Sprintf("list comments on #%d: %v", opts.Number, err),
		}
	}

	for _, c := range comments {
		if FindMarker(c.Body, opts.Command, opts.Context) {
			if p.DryRun {
				log.Notice("[dry-run] Would update existing comment %d on #%d (%d bytes)", c.ID, opts.Number, len(body))
				return nil
			}
			if err := p.Client.UpdateComment(ctx, c.ID, body); err != nil {
				return &model.AppError{
					Code:    model.ErrPostFailed,
					Message: fmt.Sprintf("update comment on #%d: %v", opts.Number, err),
				}
			}
			log.Notice("Posted to #%d (updated)", opts.Number)
			return nil
		}
	}

	// No existing marker found — create new.
	if p.DryRun {
		log.Notice("[dry-run] Would create new comment on #%d (%d bytes)", opts.Number, len(body))
		return nil
	}
	if err := p.Client.CreateComment(ctx, opts.Number, body); err != nil {
		return &model.AppError{
			Code:    model.ErrPostFailed,
			Message: fmt.Sprintf("create comment on #%d: %v", opts.Number, err),
		}
	}
	log.Notice("Posted to #%d (new comment)", opts.Number)
	return nil
}

// BodyClient abstracts the GitHub API calls needed by BodyPoster.
type BodyClient interface {
	GetBody(ctx context.Context, number int) (string, error)
	UpdateBody(ctx context.Context, number int, body string) error
}

// BodyPoster injects metrics into the body of an issue or PR.
// It appends a marked section at the end, or replaces an existing one.
type BodyPoster struct {
	Client BodyClient
	DryRun bool
}

// Post reads the current body, injects the marked section, and updates the body.
func (p *BodyPoster) Post(ctx context.Context, opts PostOptions) error {
	markedContent := WrapWithMarker(opts.Command, opts.Context, opts.Content)

	currentBody, err := p.Client.GetBody(ctx, opts.Number)
	if err != nil {
		return &model.AppError{
			Code:    model.ErrPostFailed,
			Message: fmt.Sprintf("read body of #%d: %v", opts.Number, err),
		}
	}

	newBody := InjectMarkedSection(currentBody, opts.Command, opts.Context, markedContent)
	itemURL := itemURL(opts)

	if p.DryRun {
		if currentBody == newBody {
			log.Notice("[dry-run] Body of #%d already up to date — %s", opts.Number, itemURL)
		} else if FindMarker(currentBody, opts.Command, opts.Context) {
			log.Notice("[dry-run] Would update metrics section in body of #%d — %s", opts.Number, itemURL)
		} else {
			log.Notice("[dry-run] Would append metrics section to body of #%d — %s", opts.Number, itemURL)
		}
		return nil
	}

	if err := p.Client.UpdateBody(ctx, opts.Number, newBody); err != nil {
		return &model.AppError{
			Code:    model.ErrPostFailed,
			Message: fmt.Sprintf("update body of #%d: %v", opts.Number, err),
		}
	}

	if FindMarker(currentBody, opts.Command, opts.Context) {
		log.Notice("Updated metrics in body of #%d — %s", opts.Number, itemURL)
	} else {
		log.Notice("Appended metrics to body of #%d — %s", opts.Number, itemURL)
	}
	return nil
}

// itemURL constructs a clickable GitHub URL for a posted item.
func itemURL(opts PostOptions) string {
	if opts.Repo == "" {
		return fmt.Sprintf("#%d", opts.Number)
	}
	kind := "issues"
	if opts.Command == "pr" {
		kind = "pull"
	}
	return fmt.Sprintf("https://github.com/%s/%s/%d", opts.Repo, kind, opts.Number)
}

// DiscussionPoster posts metrics as GitHub Discussions.
// DryRun prevents all writes — reads are still performed to show
// what would happen, but no mutations are executed.
type DiscussionPoster struct {
	Client DiscussionClient
	DryRun bool
}

// Post creates or updates a Discussion in the configured category.
// In dry-run mode, logs the intended action without executing it.
func (p *DiscussionPoster) Post(ctx context.Context, opts PostOptions) error {
	if opts.CategoryID == "" {
		return &model.AppError{
			Code:    model.ErrPostFailed,
			Message: "posting to Discussions requires discussions.category in config",
		}
	}

	body := WrapWithMarker(opts.Command, opts.Context, opts.Content)
	title, err := renderTitle(opts)
	if err != nil {
		return &model.AppError{
			Code:    model.ErrPostFailed,
			Message: fmt.Sprintf("render discussion title: %v", err),
		}
	}

	if opts.ForceNew {
		if p.DryRun {
			log.Notice("[dry-run] Would create new discussion %q (%d bytes)", title, len(body))
			return nil
		}
		url, err := p.Client.CreateDiscussion(ctx, opts.CategoryID, title, body)
		if err != nil {
			return &model.AppError{
				Code:    model.ErrPostFailed,
				Message: fmt.Sprintf("create discussion: %v", err),
			}
		}
		log.Notice("Posted to %s (new discussion)", url)
		return nil
	}

	// Search existing discussions for our marker.
	discussions, err := p.Client.SearchDiscussions(ctx, opts.CategoryID, 50)
	if err != nil {
		return &model.AppError{
			Code:    model.ErrPostFailed,
			Message: fmt.Sprintf("search discussions: %v", err),
		}
	}

	for _, d := range discussions {
		if FindMarker(d.Body, opts.Command, opts.Context) {
			if p.DryRun {
				log.Notice("[dry-run] Would update existing discussion %s (%d bytes)", d.URL, len(body))
				return nil
			}
			if err := p.Client.UpdateDiscussion(ctx, d.ID, body); err != nil {
				return &model.AppError{
					Code:    model.ErrPostFailed,
					Message: fmt.Sprintf("update discussion: %v", err),
				}
			}
			log.Notice("Posted to %s (updated)", d.URL)
			return nil
		}
	}

	// No existing marker found — create new.
	if p.DryRun {
		log.Notice("[dry-run] Would create new discussion %q (%d bytes)", title, len(body))
		return nil
	}
	url, err := p.Client.CreateDiscussion(ctx, opts.CategoryID, title, body)
	if err != nil {
		return &model.AppError{
			Code:    model.ErrPostFailed,
			Message: fmt.Sprintf("create discussion: %v", err),
		}
	}
	log.Notice("Posted to %s (new discussion)", url)
	return nil
}

// DefaultTitleTemplate is the default Go template for discussion titles.
const DefaultTitleTemplate = "gh-velocity {{.Command}}: {{.Repo}} ({{.Date}})"

// renderTitle renders the discussion title from the template or default.
func renderTitle(opts PostOptions) (string, error) {
	tmplStr := opts.TitleTemplate
	if tmplStr == "" {
		tmplStr = DefaultTitleTemplate
	}
	tmpl, err := template.New("title").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse title template: %w", err)
	}
	data := TitleData{
		Command: opts.Command,
		Repo:    opts.Repo,
		Date:    time.Now().UTC().Format("2006-01-02"),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute title template: %w", err)
	}
	return buf.String(), nil
}
