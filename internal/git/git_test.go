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

func TestParseCommitLog_MalformedDate(t *testing.T) {
	// One valid commit, one with a malformed date, one valid commit.
	input := "abc123\t2026-03-01T12:00:00Z\tgood commit\n" +
		"def456\tnot-a-date\tbad commit\n" +
		"ghi789\t2026-03-02T12:00:00Z\tanother good commit\n"

	commits := parseCommitLog(input)

	if len(commits) != 2 {
		t.Fatalf("expected 2 commits (malformed date skipped), got %d", len(commits))
	}

	// Verify the correct commits were kept.
	if commits[0].SHA != "abc123" {
		t.Errorf("expected first commit SHA abc123, got %s", commits[0].SHA)
	}
	if commits[1].SHA != "ghi789" {
		t.Errorf("expected second commit SHA ghi789, got %s", commits[1].SHA)
	}

	// Verify no zero-time commits were produced.
	for _, c := range commits {
		if c.AuthoredAt.IsZero() {
			t.Errorf("commit %s has zero-value AuthoredAt", c.SHA)
		}
	}
}

func TestParseCommitLog_AllMalformedDates(t *testing.T) {
	input := "abc123\tbaddate1\tcommit one\n" +
		"def456\tbaddate2\tcommit two\n"

	commits := parseCommitLog(input)

	if len(commits) != 0 {
		t.Fatalf("expected 0 commits when all dates are malformed, got %d", len(commits))
	}
}

func TestParseCommitLog_ValidDates(t *testing.T) {
	input := "abc123\t2026-03-01T12:00:00Z\tcommit one\n"

	commits := parseCommitLog(input)

	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	expected := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	if !commits[0].AuthoredAt.Equal(expected) {
		t.Errorf("expected AuthoredAt %v, got %v", expected, commits[0].AuthoredAt)
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

func TestValidateRepo(t *testing.T) {
	tests := []struct {
		name    string
		repo    string
		wantErr bool
	}{
		// Valid repos
		{name: "simple", repo: "owner/repo", wantErr: false},
		{name: "with dots", repo: "my.org/my.repo", wantErr: false},
		{name: "with hyphens", repo: "my-org/my-repo", wantErr: false},
		{name: "with underscores", repo: "my_org/my_repo", wantErr: false},

		// Invalid repos
		{name: "empty", repo: "", wantErr: true},
		{name: "no slash", repo: "justrepo", wantErr: true},
		{name: "too many slashes", repo: "a/b/c", wantErr: true},
		{name: "semicolon injection", repo: "owner/repo;rm -rf /", wantErr: true},
		{name: "space injection", repo: "owner/repo --evil", wantErr: true},
		{name: "backtick injection", repo: "owner/`whoami`", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRepo(tt.repo)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRepo(%q) error = %v, wantErr %v", tt.repo, err, tt.wantErr)
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

func TestAllCommits_InjectionRejected(t *testing.T) {
	dir := setupTestRepo(t)
	r := NewRunner(dir)

	_, err := r.AllCommits(context.Background(), "`whoami`")
	if err == nil {
		t.Error("AllCommits with injected ref should fail")
	}
}
