package gitdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsLocalGitAvailable_WithGitDir(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsLocalGitAvailable(dir) {
		t.Error("expected IsLocalGitAvailable to return true for dir with .git")
	}
}

func TestIsLocalGitAvailable_WithoutGitDir(t *testing.T) {
	dir := t.TempDir()
	if IsLocalGitAvailable(dir) {
		t.Error("expected IsLocalGitAvailable to return false for dir without .git")
	}
}

func TestIsLocalGitAvailable_WithGitFile(t *testing.T) {
	// Simulate a git worktree/submodule where .git is a file
	dir := t.TempDir()
	gitFile := filepath.Join(dir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /some/path"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsLocalGitAvailable(dir) {
		t.Error("expected IsLocalGitAvailable to return true for dir with .git file")
	}
}

func TestAPISource_AllCommits_ReturnsError(t *testing.T) {
	// APISource.AllCommits should return an error since it can't enumerate all commits
	source := NewAPISource(nil)
	_, err := source.AllCommits(nil, "HEAD")
	if err == nil {
		t.Error("expected APISource.AllCommits to return an error")
	}
}
