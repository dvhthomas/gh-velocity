package posting

import (
	"context"
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/model"
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
)

// PostOptions configures a single post operation.
type PostOptions struct {
	Command    string // "lead-time", "cycle-time", "report", "release"
	Context    string // "42", "pr-5", "30d", "v1.0", "2026-01-01..2026-02-01"
	Content    string // markdown body (already formatted)
	Target     Target
	Number     int    // issue/PR number (for comment targets)
	ForceNew   bool   // --new-post: skip search, always create
	CategoryID string // Discussion category node ID (required for DiscussionTarget)
	Repo       string // "owner/repo" for discussion titles
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
			Message: "posting to Discussions requires a category_id in config",
		}
	}

	body := WrapWithMarker(opts.Command, opts.Context, opts.Content)
	title := fmt.Sprintf("gh-velocity %s: %s (%s)", opts.Command, opts.Repo, time.Now().UTC().Format("2006-01-02"))

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
