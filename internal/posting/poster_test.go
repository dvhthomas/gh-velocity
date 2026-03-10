package posting

import (
	"context"
	"errors"
	"testing"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// mockCommentClient is a test double for CommentClient.
type mockCommentClient struct {
	comments       []Comment
	listErr        error
	createErr      error
	updateErr      error
	createdBodies  []string
	createdNumbers []int
	updatedIDs     []int64
	updatedBodies  []string
}

func (m *mockCommentClient) ListComments(_ context.Context, _ int) ([]Comment, error) {
	return m.comments, m.listErr
}

func (m *mockCommentClient) CreateComment(_ context.Context, number int, body string) error {
	m.createdNumbers = append(m.createdNumbers, number)
	m.createdBodies = append(m.createdBodies, body)
	return m.createErr
}

func (m *mockCommentClient) UpdateComment(_ context.Context, id int64, body string) error {
	m.updatedIDs = append(m.updatedIDs, id)
	m.updatedBodies = append(m.updatedBodies, body)
	return m.updateErr
}

func TestCommentPoster_CreateNew(t *testing.T) {
	mock := &mockCommentClient{comments: []Comment{}}
	poster := &CommentPoster{Client: mock}

	err := poster.Post(context.Background(), PostOptions{
		Command: "lead-time",
		Context: "42",
		Content: "| Metric | Value |\n",
		Target:  IssueComment,
		Number:  42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.createdBodies) != 1 {
		t.Fatalf("expected 1 create, got %d", len(mock.createdBodies))
	}
	if !FindMarker(mock.createdBodies[0], "lead-time", "42") {
		t.Error("created body should contain marker")
	}
}

func TestCommentPoster_UpdateExisting(t *testing.T) {
	existing := WrapWithMarker("lead-time", "42", "old content")
	mock := &mockCommentClient{
		comments: []Comment{{ID: 999, Body: existing}},
	}
	poster := &CommentPoster{Client: mock}

	err := poster.Post(context.Background(), PostOptions{
		Command: "lead-time",
		Context: "42",
		Content: "new content",
		Target:  IssueComment,
		Number:  42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.updatedIDs) != 1 || mock.updatedIDs[0] != 999 {
		t.Errorf("expected update on comment 999, got %v", mock.updatedIDs)
	}
	if len(mock.createdBodies) != 0 {
		t.Error("should not create when updating")
	}
}

func TestCommentPoster_ForceNew(t *testing.T) {
	existing := WrapWithMarker("lead-time", "42", "old content")
	mock := &mockCommentClient{
		comments: []Comment{{ID: 999, Body: existing}},
	}
	poster := &CommentPoster{Client: mock}

	err := poster.Post(context.Background(), PostOptions{
		Command:  "lead-time",
		Context:  "42",
		Content:  "new content",
		Target:   IssueComment,
		Number:   42,
		ForceNew: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.createdBodies) != 1 {
		t.Fatal("expected create even though marker exists")
	}
	if len(mock.updatedIDs) != 0 {
		t.Error("should not update when ForceNew is set")
	}
}

func TestCommentPoster_ListError(t *testing.T) {
	mock := &mockCommentClient{listErr: errors.New("network error")}
	poster := &CommentPoster{Client: mock}

	err := poster.Post(context.Background(), PostOptions{
		Command: "lead-time",
		Context: "42",
		Content: "content",
		Number:  42,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != model.ErrPostFailed {
		t.Errorf("expected POST_FAILED, got %s", appErr.Code)
	}
}

func TestCommentPoster_CreateError(t *testing.T) {
	mock := &mockCommentClient{
		comments:  []Comment{},
		createErr: errors.New("forbidden"),
	}
	poster := &CommentPoster{Client: mock}

	err := poster.Post(context.Background(), PostOptions{
		Command: "lead-time",
		Context: "42",
		Content: "content",
		Number:  42,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
}

func TestCommentPoster_DifferentMarkerNotMatched(t *testing.T) {
	// Existing comment has a different marker — should create new, not update.
	existing := WrapWithMarker("cycle-time", "pr-5", "different metric")
	mock := &mockCommentClient{
		comments: []Comment{{ID: 100, Body: existing}},
	}
	poster := &CommentPoster{Client: mock}

	err := poster.Post(context.Background(), PostOptions{
		Command: "lead-time",
		Context: "42",
		Content: "new metric",
		Number:  42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.createdBodies) != 1 {
		t.Fatal("expected create since markers don't match")
	}
	if len(mock.updatedIDs) != 0 {
		t.Error("should not update different marker")
	}
}

// mockDiscussionClient is a test double for DiscussionClient.
type mockDiscussionClient struct {
	discussions    []Discussion
	searchErr      error
	createErr      error
	updateErr      error
	createdTitles  []string
	createdBodies  []string
	updatedIDs     []string
	updatedBodies  []string
	createdURL     string
}

func (m *mockDiscussionClient) SearchDiscussions(_ context.Context, _ string, _ int) ([]Discussion, error) {
	return m.discussions, m.searchErr
}

func (m *mockDiscussionClient) CreateDiscussion(_ context.Context, _ string, title, body string) (string, error) {
	m.createdTitles = append(m.createdTitles, title)
	m.createdBodies = append(m.createdBodies, body)
	url := m.createdURL
	if url == "" {
		url = "https://github.com/owner/repo/discussions/1"
	}
	return url, m.createErr
}

func (m *mockDiscussionClient) UpdateDiscussion(_ context.Context, id, body string) error {
	m.updatedIDs = append(m.updatedIDs, id)
	m.updatedBodies = append(m.updatedBodies, body)
	return m.updateErr
}

func TestDiscussionPoster_CreateNew(t *testing.T) {
	mock := &mockDiscussionClient{discussions: []Discussion{}}
	poster := &DiscussionPoster{Client: mock}

	err := poster.Post(context.Background(), PostOptions{
		Command:    "report",
		Context:    "30d",
		Content:    "report content",
		Target:     DiscussionTarget,
		CategoryID: "DIC_abc",
		Repo:       "owner/repo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.createdBodies) != 1 {
		t.Fatal("expected 1 create")
	}
	if !FindMarker(mock.createdBodies[0], "report", "30d") {
		t.Error("created body should contain marker")
	}
}

func TestDiscussionPoster_UpdateExisting(t *testing.T) {
	existing := WrapWithMarker("report", "30d", "old report")
	mock := &mockDiscussionClient{
		discussions: []Discussion{
			{ID: "D_abc", Body: existing, URL: "https://github.com/owner/repo/discussions/1"},
		},
	}
	poster := &DiscussionPoster{Client: mock}

	err := poster.Post(context.Background(), PostOptions{
		Command:    "report",
		Context:    "30d",
		Content:    "new report",
		Target:     DiscussionTarget,
		CategoryID: "DIC_abc",
		Repo:       "owner/repo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.updatedIDs) != 1 || mock.updatedIDs[0] != "D_abc" {
		t.Errorf("expected update on D_abc, got %v", mock.updatedIDs)
	}
}

func TestDiscussionPoster_ForceNew(t *testing.T) {
	existing := WrapWithMarker("report", "30d", "old")
	mock := &mockDiscussionClient{
		discussions: []Discussion{{ID: "D_abc", Body: existing}},
	}
	poster := &DiscussionPoster{Client: mock}

	err := poster.Post(context.Background(), PostOptions{
		Command:    "report",
		Context:    "30d",
		Content:    "new",
		Target:     DiscussionTarget,
		CategoryID: "DIC_abc",
		Repo:       "owner/repo",
		ForceNew:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.createdBodies) != 1 {
		t.Fatal("expected create even though marker exists")
	}
}

func TestDiscussionPoster_MissingCategoryID(t *testing.T) {
	mock := &mockDiscussionClient{}
	poster := &DiscussionPoster{Client: mock}

	err := poster.Post(context.Background(), PostOptions{
		Command: "report",
		Context: "30d",
		Content: "content",
		Target:  DiscussionTarget,
	})
	if err == nil {
		t.Fatal("expected error for missing category_id")
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != model.ErrPostFailed {
		t.Errorf("expected POST_FAILED, got %s", appErr.Code)
	}
}

func TestDiscussionPoster_SearchError(t *testing.T) {
	mock := &mockDiscussionClient{searchErr: errors.New("graphql error")}
	poster := &DiscussionPoster{Client: mock}

	err := poster.Post(context.Background(), PostOptions{
		Command:    "report",
		Context:    "30d",
		Content:    "content",
		Target:     DiscussionTarget,
		CategoryID: "DIC_abc",
		Repo:       "owner/repo",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
}

// --- Dry-run tests ---

func TestCommentPoster_DryRun_NoCreate(t *testing.T) {
	mock := &mockCommentClient{comments: []Comment{}}
	poster := &CommentPoster{Client: mock, DryRun: true}

	err := poster.Post(context.Background(), PostOptions{
		Command: "lead-time",
		Context: "42",
		Content: "content",
		Number:  42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.createdBodies) != 0 {
		t.Error("dry-run should not create comments")
	}
}

func TestCommentPoster_DryRun_NoUpdate(t *testing.T) {
	existing := WrapWithMarker("lead-time", "42", "old")
	mock := &mockCommentClient{
		comments: []Comment{{ID: 999, Body: existing}},
	}
	poster := &CommentPoster{Client: mock, DryRun: true}

	err := poster.Post(context.Background(), PostOptions{
		Command: "lead-time",
		Context: "42",
		Content: "new",
		Number:  42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.updatedIDs) != 0 {
		t.Error("dry-run should not update comments")
	}
}

func TestCommentPoster_DryRun_ForceNew(t *testing.T) {
	mock := &mockCommentClient{}
	poster := &CommentPoster{Client: mock, DryRun: true}

	err := poster.Post(context.Background(), PostOptions{
		Command:  "lead-time",
		Context:  "42",
		Content:  "content",
		Number:   42,
		ForceNew: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.createdBodies) != 0 {
		t.Error("dry-run should not create even with ForceNew")
	}
}

func TestDiscussionPoster_DryRun_NoCreate(t *testing.T) {
	mock := &mockDiscussionClient{discussions: []Discussion{}}
	poster := &DiscussionPoster{Client: mock, DryRun: true}

	err := poster.Post(context.Background(), PostOptions{
		Command:    "report",
		Context:    "30d",
		Content:    "content",
		Target:     DiscussionTarget,
		CategoryID: "DIC_abc",
		Repo:       "owner/repo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.createdBodies) != 0 {
		t.Error("dry-run should not create discussions")
	}
}

func TestDiscussionPoster_DryRun_NoUpdate(t *testing.T) {
	existing := WrapWithMarker("report", "30d", "old")
	mock := &mockDiscussionClient{
		discussions: []Discussion{{ID: "D_abc", Body: existing, URL: "https://github.com/o/r/discussions/1"}},
	}
	poster := &DiscussionPoster{Client: mock, DryRun: true}

	err := poster.Post(context.Background(), PostOptions{
		Command:    "report",
		Context:    "30d",
		Content:    "new",
		Target:     DiscussionTarget,
		CategoryID: "DIC_abc",
		Repo:       "owner/repo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.updatedIDs) != 0 {
		t.Error("dry-run should not update discussions")
	}
}
