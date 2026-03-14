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

	"github.com/bitsbyme/gh-velocity/internal/log"

	"github.com/bitsbyme/gh-velocity/internal/model"
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

// Contributor represents a person who contributed commits to a path.
type Contributor struct {
	Name    string
	Email   string
	Commits int
}

// PathContributors holds contributor data for a single directory path.
type PathContributors struct {
	Path         string
	Contributors []Contributor
	TotalCommits int
}

// ContributorsByPath runs a single git log with --numstat and aggregates
// contributor data per directory path, truncated to the specified depth.
// This is O(1) process spawns instead of O(D) per-directory invocations.
func (r *Runner) ContributorsByPath(ctx context.Context, since time.Time, depth int, minCommits int) ([]PathContributors, error) {
	sinceStr := since.Format("2006-01-02")
	args := []string{
		"log",
		"--format=%H%x00%aN%x00%aE",
		"--numstat",
		"--no-merges",
		"--since=" + sinceStr,
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("git stdout pipe: %w", err)
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("git start: %w", err)
	}

	// map[dir]map[email]*Contributor
	dirContribs := make(map[string]map[string]*Contributor)

	var currentName, currentEmail string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Commit header: hash\0name\0email
		if parts := strings.SplitN(line, "\x00", 3); len(parts) == 3 {
			currentName = parts[1]
			currentEmail = parts[2]
			continue
		}

		// Numstat line: added\tremoved\tfilepath
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 || currentEmail == "" {
			continue
		}
		// Skip binary files (shown as "-\t-\tpath")
		if parts[0] == "-" {
			continue
		}

		filePath := parts[2]
		dir := truncatePath(filePath, depth)

		contribs, ok := dirContribs[dir]
		if !ok {
			contribs = make(map[string]*Contributor)
			dirContribs[dir] = contribs
		}
		c, ok := contribs[currentEmail]
		if !ok {
			c = &Contributor{Name: currentName, Email: currentEmail}
			contribs[currentEmail] = c
		}
		c.Commits++
		// Keep the most recent name for this email
		c.Name = currentName
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("git log --numstat: %w: %s", err, stderrBuf.String())
	}

	// Convert to sorted result, filtering by minCommits.
	var result []PathContributors
	for dir, contribs := range dirContribs {
		total := 0
		var contributors []Contributor
		for _, c := range contribs {
			total += c.Commits
			contributors = append(contributors, *c)
		}
		if total < minCommits {
			continue
		}
		result = append(result, PathContributors{
			Path:         dir,
			Contributors: contributors,
			TotalCommits: total,
		})
	}

	return result, nil
}

// truncatePath truncates a file path to the given directory depth.
// e.g., "internal/git/git.go" at depth 2 -> "internal/git/"
// Files at root level (no slash) -> "."
func truncatePath(path string, depth int) string {
	parts := strings.Split(path, "/")
	if len(parts) <= depth {
		// File is at or below depth — use its directory
		if len(parts) == 1 {
			return "."
		}
		return strings.Join(parts[:len(parts)-1], "/") + "/"
	}
	return strings.Join(parts[:depth], "/") + "/"
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
