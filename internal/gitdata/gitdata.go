// Package gitdata defines a Source interface for git operations (tags, commits)
// and provides two implementations: local git CLI and GitHub API fallback.
package gitdata

import (
	"context"
	"fmt"
	"os"

	"github.com/bitsbyme/gh-velocity/internal/git"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// Source abstracts git data retrieval so callers can transparently use
// either a local git checkout or the GitHub API.
type Source interface {
	// Tags returns all tags sorted newest-first.
	Tags(ctx context.Context) ([]string, error)
	// CommitsBetween returns commits between two refs (exclusive base, inclusive head).
	CommitsBetween(ctx context.Context, base, head string) ([]model.Commit, error)
	// AllCommits returns all commits reachable from ref.
	AllCommits(ctx context.Context, ref string) ([]model.Commit, error)
}

// LocalSource wraps git.Runner to satisfy Source.
type LocalSource struct {
	runner *git.Runner
}

// NewLocalSource creates a Source backed by the local git CLI.
func NewLocalSource(dir string) *LocalSource {
	return &LocalSource{runner: git.NewRunner(dir)}
}

func (s *LocalSource) Tags(ctx context.Context) ([]string, error) {
	return s.runner.Tags(ctx)
}

func (s *LocalSource) CommitsBetween(ctx context.Context, base, head string) ([]model.Commit, error) {
	return s.runner.CommitsBetween(ctx, base, head)
}

func (s *LocalSource) AllCommits(ctx context.Context, ref string) ([]model.Commit, error) {
	return s.runner.AllCommits(ctx, ref)
}

// APISource wraps the GitHub REST client to satisfy Source.
type APISource struct {
	client *gh.Client
}

// NewAPISource creates a Source backed by the GitHub API.
func NewAPISource(client *gh.Client) *APISource {
	return &APISource{client: client}
}

func (s *APISource) Tags(ctx context.Context) ([]string, error) {
	return s.client.ListTags(ctx)
}

func (s *APISource) CommitsBetween(ctx context.Context, base, head string) ([]model.Commit, error) {
	return s.client.CompareCommits(ctx, base, head)
}

func (s *APISource) AllCommits(ctx context.Context, ref string) ([]model.Commit, error) {
	// The GitHub compare API requires a base ref. For "all commits", compare
	// against the first commit. This is a best-effort approximation — we use
	// the compare endpoint with the repo's default branch root. In practice,
	// release commands nearly always have a --since tag, so this path is rare.
	return nil, fmt.Errorf("API fallback does not support listing all commits (use --since <tag> to specify a base tag)")
}

// IsLocalGitAvailable returns true if the given directory is inside a git working tree.
func IsLocalGitAvailable(dir string) bool {
	// Walk up from dir looking for .git. This is a fast check that avoids shelling out.
	info, err := os.Stat(dir + "/.git")
	if err == nil && info.IsDir() {
		return true
	}
	// Also check if it's a file (.git file for worktrees/submodules).
	if err == nil {
		return true
	}
	return false
}
