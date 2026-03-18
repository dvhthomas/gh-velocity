package cmd

import (
	"bytes"
	"context"
	"io"

	"fmt"

	"github.com/dvhthomas/gh-velocity/internal/format"
	gh "github.com/dvhthomas/gh-velocity/internal/github"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/dvhthomas/gh-velocity/internal/posting"
	"github.com/spf13/cobra"
)

// postIfEnabled returns a writer and a post function. When --post is not set,
// the writer is cmd.OutOrStdout() and the post function is a no-op.
// When --post is set, the writer tees to both stdout and an internal buffer.
// After formatting completes, call the returned function to post the captured
// output to GitHub. The poster respects DryRun from deps.
func postIfEnabled(cmd *cobra.Command, deps *Deps, client *gh.Client, opts posting.PostOptions) (io.Writer, func() error) {
	if !deps.Post {
		return cmd.OutOrStdout(), func() error { return nil }
	}

	var buf bytes.Buffer
	w := io.MultiWriter(cmd.OutOrStdout(), &buf)

	return w, func() error {
		opts.Content = wrapForPost(buf.String(), deps.Format)
		opts.ForceNew = deps.NewPost
		opts.Repo = deps.Owner + "/" + deps.Repo

		var poster posting.Poster
		switch opts.Target {
		case posting.DiscussionTarget:
			categoryName := deps.Config.Discussions.Category
			if categoryName == "" {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "posting to Discussions requires discussions.category in config",
				}
			}
			categoryID, err := client.ResolveDiscussionCategoryID(cmd.Context(), categoryName)
			if err != nil {
				return &model.AppError{
					Code:    model.ErrPostFailed,
					Message: fmt.Sprintf("resolve discussion category %q: %v", categoryName, err),
				}
			}
			opts.CategoryID = categoryID
			poster = &posting.DiscussionPoster{
				Client: &discussionAdapter{client: client},
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

// wrapForPost prepares content for posting: JSON is wrapped in a code fence,
// markdown and pretty are used as-is.
func wrapForPost(content string, f format.Format) string {
	if f == format.JSON {
		return "```json\n" + content + "```\n"
	}
	return content
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

// discussionAdapter adapts github.Client to posting.DiscussionClient.
type discussionAdapter struct {
	client *gh.Client
}

func (a *discussionAdapter) SearchDiscussions(ctx context.Context, categoryID string, limit int) ([]posting.Discussion, error) {
	discussions, err := a.client.SearchDiscussions(ctx, categoryID, limit)
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
	return a.client.CreateDiscussion(ctx, categoryID, title, body)
}

func (a *discussionAdapter) UpdateDiscussion(ctx context.Context, discussionID, body string) error {
	return a.client.UpdateDiscussion(ctx, discussionID, body)
}
