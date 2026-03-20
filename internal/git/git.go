// Package git wraps local git operations via exec.CommandContext.
// Isolated from the GitHub API client — works offline for tag/commit data.
package git

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/log"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

var refPattern = regexp.MustCompile(`^[a-zA-Z0-9._\-/]+$`)

// ValidateRef validates a git ref (tag, branch, or commit reference) against
// a strict allowlist pattern to prevent command injection. Refs starting with
// "-" are rejected to prevent flag injection.
func ValidateRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("ref must not be empty")
	}
	if ref[0] == '-' {
		return fmt.Errorf("invalid ref %q: must not start with '-'", ref)
	}
	if !refPattern.MatchString(ref) {
		return fmt.Errorf("invalid ref %q: must match %s", ref, refPattern.String())
	}
	return nil
}

// Runner executes local git commands.
type Runner struct {
	dir string // working directory for git commands
}

// NewRunner creates a Runner that operates in the given directory.
func NewRunner(dir string) *Runner {
	return &Runner{dir: dir}
}

// Tags returns all tags sorted by creation date (newest first).
func (r *Runner) Tags(ctx context.Context) ([]string, error) {
	out, err := r.run(ctx, "tag", "--sort=-creatordate")
	if err != nil {
		return nil, fmt.Errorf("git tags: %w", err)
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimSpace(out), "\n"), nil
}

// CommitsBetween returns commits between two refs (exclusive base, inclusive head).
func (r *Runner) CommitsBetween(ctx context.Context, base, head string) ([]model.Commit, error) {
	if err := ValidateRef(base); err != nil {
		return nil, fmt.Errorf("git log: invalid base: %w", err)
	}
	if err := ValidateRef(head); err != nil {
		return nil, fmt.Errorf("git log: invalid head: %w", err)
	}
	rangeSpec := fmt.Sprintf("%s..%s", base, head)
	return r.streamCommits(ctx, "log", rangeSpec, "--format=%H\t%aI\t%s", "--")
}

// AllCommits returns all commits in the repo (for first release).
func (r *Runner) AllCommits(ctx context.Context, upToRef string) ([]model.Commit, error) {
	if err := ValidateRef(upToRef); err != nil {
		return nil, fmt.Errorf("git log: invalid ref: %w", err)
	}
	return r.streamCommits(ctx, "log", upToRef, "--format=%H\t%aI\t%s", "--")
}

// CommitsForIssue returns commits whose message references the given issue number
// (e.g. "#123"). It uses git log --grep to let git do the filtering, avoiding a
// full history scan.
func (r *Runner) CommitsForIssue(ctx context.Context, issueNumber int, ref string) ([]model.Commit, error) {
	if err := ValidateRef(ref); err != nil {
		return nil, fmt.Errorf("git log: invalid ref: %w", err)
	}
	grepPattern := "#" + strconv.Itoa(issueNumber)
	return r.streamCommits(ctx, "log", ref, "--grep", grepPattern, "--fixed-strings", "--format=%H\t%aI\t%s", "--")
}

func (r *Runner) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%w: %s", err, string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

// streamCommits runs a git command and streams its output line-by-line through
// a bufio.Scanner, parsing each line into a model.Commit. This avoids
// buffering the entire output into memory, which matters for large repos.
func (r *Runner) streamCommits(ctx context.Context, args ...string) ([]model.Commit, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("git stdout pipe: %w", err)
	}

	// Capture stderr for error reporting.
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("git start: %w", err)
	}

	var commits []model.Commit
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}

		authored, err := time.Parse(time.RFC3339, parts[1])
		if err != nil {
			log.Warn("skipping commit %s: malformed date %q", parts[0], parts[1])
			continue
		}
		commits = append(commits, model.Commit{
			SHA:        parts[0],
			AuthoredAt: authored.UTC(),
			Message:    parts[2],
		})
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("git %v: %w: %s", args, err, stderrBuf.String())
	}
	return commits, nil
}
