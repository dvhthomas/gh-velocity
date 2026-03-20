package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	gh "github.com/dvhthomas/gh-velocity/internal/github"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/dvhthomas/gh-velocity/internal/posting"
	"github.com/spf13/cobra"
)

// postCapture holds the independent post buffer and the post function.
// The buffer captures markdown output separately from stdout and --write-to.
type postCapture struct {
	buf  bytes.Buffer
	deps *Deps
}

// postWriter returns a writer for the post buffer. When --write-to is set,
// this is the only way to capture content for posting (stdout is silent).
// When --write-to is not set, content is teed to both stdout and the buffer.
func (pc *postCapture) postWriter(stdout io.Writer) io.Writer {
	if pc.deps.Output.WriteTo != "" {
		// --write-to mode: post buffer is independent, stdout is silent.
		return &pc.buf
	}
	// Stdout mode: tee to both stdout and the buffer.
	return io.MultiWriter(stdout, &pc.buf)
}

// setupPost returns a postCapture and a post function. When --post is not set,
// both are nil/no-op. When --post is set, the caller must render markdown to
// the postCapture's writer, then call the returned function to post.
func setupPost(cmd *cobra.Command, deps *Deps, client *gh.Client, opts posting.PostOptions) (*postCapture, func() error) {
	if !deps.Post {
		return nil, func() error { return nil }
	}

	pc := &postCapture{deps: deps}

	return pc, func() error {
		opts.Content = pc.buf.String()
		opts.ForceNew = deps.NewPost
		opts.Repo = deps.Owner + "/" + deps.Repo

		var poster posting.Poster
		switch opts.Target {
		case posting.DiscussionTarget:
			targetStr := deps.Config.Discussions.Category
			if targetStr == "" {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "posting to Discussions requires discussions.category in config (owner/repo/category)",
				}
			}
			target, err := posting.ParseTarget(targetStr)
			if err != nil {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: fmt.Sprintf("discussions.category: %v", err),
				}
			}
			categoryID, err := client.ResolveDiscussionCategoryIDForRepo(cmd.Context(), target.Owner, target.Repo, target.Category)
			if err != nil {
				return &model.AppError{
					Code:    model.ErrPostFailed,
					Message: fmt.Sprintf("resolve discussion category %q on %s/%s: %v", target.Category, target.Owner, target.Repo, err),
				}
			}
			opts.CategoryID = categoryID
			if titleTemplate := deps.Config.Discussions.Title; titleTemplate != "" {
				opts.Title = posting.RenderTitle(titleTemplate, posting.TitleVars{
					Date:    time.Now().UTC(),
					Repo:    opts.Repo,
					Owner:   deps.Owner,
					Command: opts.Command,
				})
			}
			poster = &posting.DiscussionPoster{
				Client: &discussionAdapter{
					client: client,
					owner:  target.Owner,
					repo:   target.Repo,
				},
				DryRun: deps.DryRun,
			}
		case posting.IssueBody:
			poster = &posting.BodyPoster{
				Client: &bodyAdapter{client: client},
				DryRun: deps.DryRun,
			}
		default:
			poster = &posting.CommentPoster{
				Client: &commentAdapter{client: client},
				DryRun: deps.DryRun,
			}
		}

		return poster.Post(cmd.Context(), opts)
	}
}

// commentAdapter adapts github.Client to posting.CommentClient.
type commentAdapter struct {
	client *gh.Client
}

func (a *commentAdapter) ListComments(ctx context.Context, number int) ([]posting.Comment, error) {
	comments, err := a.client.ListComments(ctx, number)
	if err != nil {
		return nil, err
	}
	result := make([]posting.Comment, len(comments))
	for i, c := range comments {
		result[i] = posting.Comment{ID: c.ID, Body: c.Body}
	}
	return result, nil
}

func (a *commentAdapter) CreateComment(ctx context.Context, number int, body string) error {
	return a.client.CreateComment(ctx, number, body)
}

func (a *commentAdapter) UpdateComment(ctx context.Context, commentID int64, body string) error {
	return a.client.UpdateComment(ctx, commentID, body)
}

// bodyAdapter adapts github.Client to posting.BodyClient.
type bodyAdapter struct {
	client *gh.Client
}

func (a *bodyAdapter) GetBody(ctx context.Context, number int) (string, error) {
	return a.client.GetIssueBody(ctx, number)
}

func (a *bodyAdapter) UpdateBody(ctx context.Context, number int, body string) error {
	return a.client.UpdateIssueBody(ctx, number, body)
}

// discussionAdapter adapts github.Client to posting.DiscussionClient.
// owner/repo specify the target repository for cross-repo posting.
type discussionAdapter struct {
	client *gh.Client
	owner  string
	repo   string
}

func (a *discussionAdapter) SearchDiscussions(ctx context.Context, categoryID string, limit int) ([]posting.Discussion, error) {
	discussions, err := a.client.SearchDiscussionsForRepo(ctx, a.owner, a.repo, categoryID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]posting.Discussion, len(discussions))
	for i, d := range discussions {
		result[i] = posting.Discussion{ID: d.ID, Title: d.Title, Body: d.Body, URL: d.URL}
	}
	return result, nil
}

func (a *discussionAdapter) CreateDiscussion(ctx context.Context, categoryID, title, body string) (string, error) {
	return a.client.CreateDiscussionForRepo(ctx, a.owner, a.repo, categoryID, title, body)
}

func (a *discussionAdapter) UpdateDiscussion(ctx context.Context, discussionID, body string) error {
	return a.client.UpdateDiscussion(ctx, discussionID, body)
}
