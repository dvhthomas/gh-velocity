package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// setupTestRepo creates a temp git repo with some commits and tags for testing.
func setupTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	commands := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}

	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", args, err, out)
		}
	}

	// Create commits
	for i, msg := range []string{"initial commit", "add feature #1", "fix bug #2"} {
		path := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(path, []byte(msg), 0644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "add", ".")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git add: %v\n%s", err, out)
		}

		cmd = exec.Command("git", "commit", "-m", msg)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_COMMITTER_DATE=2026-03-0"+string(rune('1'+i))+"T12:00:00Z",
			"GIT_AUTHOR_DATE=2026-03-0"+string(rune('1'+i))+"T12:00:00Z",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit %q: %v\n%s", msg, err, out)
		}
	}

	// Create tags
	for _, tag := range []string{"v1.0.0", "v1.1.0"} {
		cmd := exec.Command("git", "tag", tag)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git tag %s: %v\n%s", tag, err, out)
		}

		// Add another commit after first tag
		if tag == "v1.0.0" {
			path := filepath.Join(dir, "file2.txt")
			if err := os.WriteFile(path, []byte("post-tag"), 0644); err != nil {
				t.Fatal(err)
			}
			cmd = exec.Command("git", "add", ".")
			cmd.Dir = dir
			cmd.CombinedOutput()
			cmd = exec.Command("git", "commit", "-m", "post v1.0.0 work closes #3")
			cmd.Dir = dir
			cmd.CombinedOutput()
		}
	}

	return dir
}

func TestTags(t *testing.T) {
	dir := setupTestRepo(t)
	r := NewRunner(dir)

	tags, err := r.Tags(context.Background())
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(tags), tags)
	}
}

func TestCommitsBetween(t *testing.T) {
	dir := setupTestRepo(t)
	r := NewRunner(dir)

	commits, err := r.CommitsBetween(context.Background(), "v1.0.0", "v1.1.0")
	if err != nil {
		t.Fatalf("CommitsBetween: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("expected commits between tags")
	}
}

func TestTags_EmptyRepo(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	cmd.CombinedOutput()

	r := NewRunner(dir)
	tags, err := r.Tags(context.Background())
	if err != nil {
		t.Fatalf("Tags on empty repo: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
}

func TestValidateRef(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		// Valid refs
		{name: "simple tag", ref: "v1.0.0", wantErr: false},
		{name: "tag with slash", ref: "release/v1.0.0", wantErr: false},
		{name: "branch name", ref: "main", wantErr: false},
		{name: "SHA-like", ref: "abc123def456", wantErr: false},
		{name: "tag with hyphen", ref: "v1.0.0-rc1", wantErr: false},
		{name: "tag with underscore", ref: "my_tag", wantErr: false},
		{name: "tag with dot", ref: "v1.0.0", wantErr: false},

		// Injection attempts
		{name: "empty", ref: "", wantErr: true},
		{name: "semicolon injection", ref: "v1.0.0;rm -rf /", wantErr: true},
		{name: "backtick injection", ref: "`whoami`", wantErr: true},
		{name: "pipe injection", ref: "v1.0.0|cat /etc/passwd", wantErr: true},
		{name: "dollar substitution", ref: "$(whoami)", wantErr: true},
		{name: "newline injection", ref: "v1.0.0\nmalicious", wantErr: true},
		{name: "space injection", ref: "v1.0.0 --upload-pack=evil", wantErr: true},
		{name: "dash-prefixed flag", ref: "--upload-pack=evil", wantErr: true},
		{name: "ampersand injection", ref: "v1.0.0&echo pwned", wantErr: true},
		{name: "single quote injection", ref: "v1.0.0'$(id)", wantErr: true},
		{name: "double quote injection", ref: `v1.0.0"$(id)`, wantErr: true},
		{name: "null byte", ref: "v1.0.0\x00evil", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRef(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRef(%q) error = %v, wantErr %v", tt.ref, err, tt.wantErr)
			}
		})
	}
}

func TestCommitsBetween_InjectionRejected(t *testing.T) {
	dir := setupTestRepo(t)
	r := NewRunner(dir)

	tests := []struct {
		name string
		base string
		head string
	}{
		{name: "semicolon in base", base: "v1.0.0;echo pwned", head: "v1.1.0"},
		{name: "semicolon in head", base: "v1.0.0", head: "v1.1.0;echo pwned"},
		{name: "backtick in base", base: "`whoami`", head: "v1.1.0"},
		{name: "pipe in head", base: "v1.0.0", head: "v1.1.0|cat /etc/passwd"},
		{name: "flag injection in base", base: "--upload-pack=evil", head: "v1.1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.CommitsBetween(context.Background(), tt.base, tt.head)
			if err == nil {
				t.Errorf("CommitsBetween(%q, %q) expected error for injection attempt", tt.base, tt.head)
			}
		})
	}
}

func TestCommitsForIssue(t *testing.T) {
	dir := setupTestRepo(t)
	r := NewRunner(dir)

	// Issue #1 is referenced in "add feature #1"
	commits, err := r.CommitsForIssue(context.Background(), 1, "HEAD")
	if err != nil {
		t.Fatalf("CommitsForIssue(1): %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit for issue #1, got %d", len(commits))
	}
	if commits[0].Message != "add feature #1" {
		t.Errorf("expected message %q, got %q", "add feature #1", commits[0].Message)
	}

	// Issue #3 is referenced in "post v1.0.0 work closes #3"
	commits, err = r.CommitsForIssue(context.Background(), 3, "HEAD")
	if err != nil {
		t.Fatalf("CommitsForIssue(3): %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit for issue #3, got %d", len(commits))
	}

	// Issue #999 should have no commits
	commits, err = r.CommitsForIssue(context.Background(), 999, "HEAD")
	if err != nil {
		t.Fatalf("CommitsForIssue(999): %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("expected 0 commits for issue #999, got %d", len(commits))
	}
}

func TestCommitsForIssue_InjectionRejected(t *testing.T) {
	dir := setupTestRepo(t)
	r := NewRunner(dir)

	_, err := r.CommitsForIssue(context.Background(), 1, "`whoami`")
	if err == nil {
		t.Error("CommitsForIssue with injected ref should fail")
	}

	_, err = r.CommitsForIssue(context.Background(), 1, "--evil-flag")
	if err == nil {
		t.Error("CommitsForIssue with flag injection should fail")
	}
}

func TestTruncatePath(t *testing.T) {
	tests := []struct {
		path  string
		depth int
		want  string
	}{
		{"internal/git/git.go", 2, "internal/git/"},
		{"internal/git/git.go", 1, "internal/"},
		{"cmd/root.go", 2, "cmd/"},
		{"cmd/root.go", 1, "cmd/"},
		{"main.go", 2, "."},
		{"main.go", 1, "."},
		{"a/b/c/d/e.go", 2, "a/b/"},
		{"a/b/c/d/e.go", 3, "a/b/c/"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := truncatePath(tt.path, tt.depth)
			if got != tt.want {
				t.Errorf("truncatePath(%q, %d) = %q, want %q", tt.path, tt.depth, got, tt.want)
			}
		})
	}
}

func TestContributorsByPath(t *testing.T) {
	dir := setupBusFactorRepo(t)
	r := NewRunner(dir)

	paths, err := r.ContributorsByPath(context.Background(), parseDate(t, "2020-01-01"), 2, 1)
	if err != nil {
		t.Fatalf("ContributorsByPath: %v", err)
	}

	if len(paths) == 0 {
		t.Fatal("expected at least 1 path")
	}

	// Check that we have the expected directories.
	pathMap := make(map[string]PathContributors)
	for _, p := range paths {
		pathMap[p.Path] = p
	}

	// internal/git/ should have alice and bob
	if p, ok := pathMap["internal/git/"]; ok {
		if p.TotalCommits < 2 {
			t.Errorf("internal/git/ total commits = %d, want >= 2", p.TotalCommits)
		}
	}
}

func TestContributorsByPath_MinCommitsFilter(t *testing.T) {
	dir := setupBusFactorRepo(t)
	r := NewRunner(dir)

	// With high min-commits, no paths should qualify.
	paths, err := r.ContributorsByPath(context.Background(), parseDate(t, "2020-01-01"), 2, 1000)
	if err != nil {
		t.Fatalf("ContributorsByPath: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 paths with min-commits=1000, got %d", len(paths))
	}
}

// setupBusFactorRepo creates a test repo with multiple authors and directories.
func setupBusFactorRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", args, err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.email", "alice@test.com")
	run("git", "config", "user.name", "Alice")

	// Alice commits to internal/git/
	if err := os.MkdirAll(filepath.Join(dir, "internal", "git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "git", "runner.go"), []byte("package git\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "alice: add git runner")

	// Alice again
	if err := os.WriteFile(filepath.Join(dir, "internal", "git", "runner.go"), []byte("package git\n// updated\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "alice: update git runner")

	// Bob commits to internal/git/
	run("git", "config", "user.email", "bob@test.com")
	run("git", "config", "user.name", "Bob")
	if err := os.WriteFile(filepath.Join(dir, "internal", "git", "util.go"), []byte("package git\n// util\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "bob: add git util")

	// Alice commits to cmd/
	run("git", "config", "user.email", "alice@test.com")
	run("git", "config", "user.name", "Alice")
	if err := os.MkdirAll(filepath.Join(dir, "cmd"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", "root.go"), []byte("package cmd\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "alice: add cmd root")

	return dir
}

func parseDate(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatal(err)
	}
	return tm
}

func TestAllCommits_InjectionRejected(t *testing.T) {
	dir := setupTestRepo(t)
	r := NewRunner(dir)

	_, err := r.AllCommits(context.Background(), "`whoami`")
	if err == nil {
		t.Error("AllCommits with injected ref should fail")
	}
}
