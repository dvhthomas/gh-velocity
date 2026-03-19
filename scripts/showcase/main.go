// Command showcase runs gh-velocity against multiple OSS repos and posts
// results as comments on a single GitHub Discussion.
//
// It is a pure orchestrator — it shells out to gh-velocity and gh for all
// operations. No library imports for GitHub API work.
//
// The repo list is read from projects.yml (next to this file by default,
// override with --projects).
//
// Usage: go run ./scripts/showcase [--dry-run] [--binary ./gh-velocity] [--since 30d]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// showcaseConfig mirrors one entry in projects.yml.
type showcaseConfig struct {
	Name    string `yaml:"name"`
	Repo    string `yaml:"repo"`
	Project string `yaml:"project"`
}

// slug derives a filesystem-safe name (e.g. "microsoft/ebpf-for-windows" → "microsoft-ebpf-for-windows").
func (sc showcaseConfig) slug() string {
	return strings.ReplaceAll(sc.Name, "/", "-")
}

// preflightFlags builds the gh-velocity flags for the preflight command.
func (sc showcaseConfig) preflightFlags() []string {
	var flags []string
	if sc.Repo != "" {
		flags = append(flags, "-R", sc.Repo)
	}
	if sc.Project != "" {
		flags = append(flags, "--project-url", sc.Project)
	}
	return flags
}

// repoFlags returns only the -R flag (safe for all commands including report).
func (sc showcaseConfig) repoFlags() []string {
	if sc.Repo != "" {
		return []string{"-R", sc.Repo}
	}
	return nil
}

type config struct {
	Binary      string
	DryRun      bool
	Since       string
	Projects    string
	Owner       string
	RepoName    string
	Category    string
	TmpDir      string
	RepoTimeout time.Duration
}

func main() {
	log.SetFlags(0)

	// Default --projects path: projects.yml next to this source file.
	_, thisFile, _, _ := runtime.Caller(0)
	defaultProjects := filepath.Join(filepath.Dir(thisFile), "projects.yml")

	cfg := config{
		Owner:       "dvhthomas",
		RepoName:    "gh-velocity",
		Category:    "Velocity Reports",
		RepoTimeout: 10 * time.Minute,
	}

	flag.StringVar(&cfg.Binary, "binary", "./gh-velocity", "path to gh-velocity binary")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "print what would happen without creating a discussion")
	flag.StringVar(&cfg.Since, "since", "30d", "time range for reports")
	flag.StringVar(&cfg.Projects, "projects", defaultProjects, "path to projects.yml")
	flag.Parse()

	configs, err := loadConfigs(cfg.Projects)
	if err != nil {
		log.Fatalf("failed to load configs: %v", err)
	}
	if len(configs) == 0 {
		log.Fatal("no configs found in " + cfg.Projects)
	}

	repoRoot, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		log.Fatalf("not in a git repo: %v", err)
	}
	root := strings.TrimSpace(string(repoRoot))

	// Resolve binary relative to repo root if it starts with ./
	if strings.HasPrefix(cfg.Binary, "./") {
		cfg.Binary = filepath.Join(root, cfg.Binary[2:])
	}
	cfg.TmpDir = filepath.Join(root, "tmp", "showcase")

	if cfg.DryRun {
		log.Println("DRY RUN — will not create Discussion or post comments")
	}

	ctx := context.Background()

	// ── Preflight checks ─────────────────────────────────────────
	if _, err := os.Stat(cfg.Binary); err != nil {
		log.Fatalf("binary not found: %s — run 'task build' first", cfg.Binary)
	}

	if out, err := execGH(ctx, "auth", "status"); err != nil {
		log.Fatalf("gh auth failed:\n%s", out)
	}

	// ── Validate discussion permissions ──────────────────────────
	var repoNodeID, categoryID string

	if !cfg.DryRun {
		log.Println("Checking discussion write permissions...")
		out, err := graphQL(ctx, `query($owner: String!, $repo: String!) {
			repository(owner: $owner, name: $repo) {
				id
				discussionCategories(first: 50) {
					nodes { id name }
				}
			}
		}`, map[string]string{"owner": cfg.Owner, "repo": cfg.RepoName})
		if err != nil {
			log.Fatalf("Cannot access repository discussions. Check that GH_TOKEN has 'repo' or 'discussion:write' scope.\n%s", out)
		}

		repoNodeID, categoryID, err = parseRepoAndCategory(out, cfg.Category)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Repository node ID: %s", repoNodeID)
		log.Printf("Category ID: %s", categoryID)
		log.Println("Discussion permissions OK")
	}

	// ── Clean tmp directory ──────────────────────────────────────
	os.RemoveAll(cfg.TmpDir)
	if err := os.MkdirAll(cfg.TmpDir, 0o755); err != nil {
		log.Fatalf("failed to create tmp dir: %v", err)
	}

	// ── Create Discussion ────────────────────────────────────────
	showcaseDate := time.Now().UTC().Format("2006-01-02")
	showcaseTime := time.Now().UTC().Format("2006-01-02T15:04Z")

	title := fmt.Sprintf("Velocity Showcase (%s)", showcaseDate)
	initialBody := fmt.Sprintf("# Velocity Showcase — %s\n\nRunning gh-velocity against %d configs. Results will appear as comments below.\n\n**Started:** %s",
		showcaseDate, len(configs), showcaseTime)

	if link := workflowRunLink(); link != "" {
		initialBody += "\n**Workflow run:** " + link
	}

	ghActionsGroup("Create discussion")

	var discID, discURL string

	if cfg.DryRun {
		log.Printf("[dry-run] Would create discussion: %s", title)
		discID = "DRY_RUN_ID"
		discURL = fmt.Sprintf("https://github.com/%s/%s/discussions/dry-run", cfg.Owner, cfg.RepoName)
	} else {
		out, err := graphQL(ctx, `mutation($repoID: ID!, $categoryID: ID!, $title: String!, $body: String!) {
			createDiscussion(input: {
				repositoryId: $repoID
				categoryId: $categoryID
				title: $title
				body: $body
			}) {
				discussion { id url }
			}
		}`, map[string]string{
			"repoID":     repoNodeID,
			"categoryID": categoryID,
			"title":      title,
			"body":       initialBody,
		})
		if err != nil {
			log.Fatalf("Failed to create Discussion:\n%s", out)
		}

		discID, discURL, err = parseDiscussion(out)
		if err != nil {
			log.Fatalf("Failed to parse discussion response: %v\n%s", err, out)
		}
	}

	log.Printf("Discussion: %s", discURL)
	ghActionsEndGroup()

	// ── Process each repo ────────────────────────────────────────
	var index []indexEntry
	successCount := 0

	for i, sc := range configs {
		ghActionsGroup(fmt.Sprintf("[%d/%d] %s", i+1, len(configs), sc.Name))

		status := processConfig(ctx, cfg, sc, showcaseTime, discID)
		index = append(index, indexEntry{Repo: sc.Name, Status: status})
		if status == "success" || status == "partial" {
			successCount++
		}

		ghActionsEndGroup()

		// Brief pause between configs to be kind to the API.
		if i < len(configs)-1 {
			time.Sleep(5 * time.Second)
		}
	}

	// ── Update Discussion body with index ────────────────────────
	ghActionsGroup("Update discussion index")

	var table strings.Builder
	for _, e := range index {
		fmt.Fprintf(&table, "| %s | `%s` |\n", e.Repo, e.Status)
	}

	finalBody := fmt.Sprintf("# Velocity Showcase — %s\n\n| Repo | Status |\n|------|--------|\n%s\n**Completed:** %s",
		showcaseDate, table.String(), time.Now().UTC().Format("2006-01-02T15:04Z"))

	if link := workflowRunLink(); link != "" {
		finalBody += "\n**Workflow run:** " + link
	}

	if cfg.DryRun {
		log.Println("[dry-run] Would update discussion body with index")
		log.Println(finalBody)
	} else {
		out, err := graphQL(ctx, `mutation($id: ID!, $body: String!) {
			updateDiscussion(input: { discussionId: $id, body: $body }) {
				discussion { url }
			}
		}`, map[string]string{"id": discID, "body": finalBody})
		if err != nil {
			log.Fatalf("Failed to update discussion body:\n%s", out)
		}
	}

	ghActionsEndGroup()

	// ── Cleanup: delete discussion if zero repos succeeded ───────
	if successCount == 0 && discID != "" && !cfg.DryRun && discID != "DRY_RUN_ID" {
		log.Println("Zero repos succeeded — deleting empty discussion")
		if _, err := graphQL(ctx, `mutation($id: ID!) {
			deleteDiscussion(input: { id: $id }) { discussion { id } }
		}`, map[string]string{"id": discID}); err != nil {
			log.Printf("Warning: failed to delete discussion: %v", err)
		}
	}

	// ── Print summary with links ────────────────────────────────
	log.Println()
	log.Printf("Showcase complete: %d/%d repos succeeded", successCount, len(configs))
	log.Printf("Discussion: %s", discURL)

	if link := workflowRunLink(); link != "" {
		log.Printf("Artifacts:  %s", link)
	}

	// Write GitHub Actions job summary with clickable links.
	writeJobSummary(discURL, index)
}

// loadConfigs reads the YAML config list.
func loadConfigs(path string) ([]showcaseConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Configs []showcaseConfig `yaml:"configs"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return doc.Configs, nil
}

// processConfig runs preflight + report for a single config and posts the comment.
// Returns the final status string.
func processConfig(ctx context.Context, cfg config, sc showcaseConfig, showcaseTime, discID string) string {
	slug := sc.slug()
	configPath := filepath.Join(cfg.TmpDir, slug+".yml")

	status, preflightErr := runPreflight(ctx, cfg.Binary, configPath, sc)

	var reportMarkdown string
	if status == "success" {
		reportMarkdown, status = runReport(ctx, cfg, configPath, slug, sc)
	}

	comment := buildComment(sc.Name, status, showcaseTime, configPath, preflightErr, reportMarkdown)
	postComment(ctx, cfg.DryRun, sc.Name, discID, comment)

	return status
}

// runPreflight generates a config file via gh-velocity preflight.
// Returns the status and any error message.
func runPreflight(ctx context.Context, binary, configPath string, sc showcaseConfig) (status, errMsg string) {
	args := []string{"config", "preflight", "--write=" + configPath, "--debug"}
	args = append(args, sc.preflightFlags()...)

	if _, err := execBinary(ctx, binary, args...); err != nil {
		ghActionsWarning("Preflight failed for " + sc.Name)
		return "preflight-failed", err.Error()
	}
	return "success", ""
}

// runReport executes the gh-velocity report command. Returns the markdown
// output and updated status.
func runReport(ctx context.Context, cfg config, configPath, slug string, sc showcaseConfig) (markdown, status string) {
	writeToDir := filepath.Join(cfg.TmpDir, slug)
	os.MkdirAll(writeToDir, 0o755)

	repoCtx, cancel := context.WithTimeout(ctx, cfg.RepoTimeout)
	defer cancel()

	args := []string{"report", "--since", cfg.Since, "--config", configPath,
		"--debug", "--results", "md,json,html", "--write-to", writeToDir}
	args = append(args, sc.repoFlags()...)

	_, err := execBinary(repoCtx, cfg.Binary, args...)
	if err != nil {
		return "", "partial"
	}

	// Read the markdown file for posting as a discussion comment.
	mdPath := filepath.Join(writeToDir, "report.md")
	data, readErr := os.ReadFile(mdPath)
	if readErr != nil || len(data) == 0 {
		return "", "partial"
	}
	return string(data), "success"
}

// buildComment assembles the Discussion comment markdown for one config run.
func buildComment(name, status, showcaseTime, configPath, preflightErr, reportMarkdown string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n**Status:** `%s` | **Generated:** %s\n\n", name, status, showcaseTime)

	if configData, err := os.ReadFile(configPath); err == nil {
		fmt.Fprintf(&b, "<details>\n<summary>Generated Config (.gh-velocity.yml)</summary>\n\n```yaml\n%s```\n\n</details>\n\n", configData)
	}

	if preflightErr != "" {
		fmt.Fprintf(&b, "### Preflight Failed\n\n```\n%s\n```\n\n", preflightErr)
	}

	if reportMarkdown != "" {
		fmt.Fprintf(&b, "### Composite Report\n\n%s\n\n", reportMarkdown)
	} else if preflightErr == "" {
		b.WriteString("### Composite Report\n\n*Report failed or timed out*\n\n")
	}

	return b.String()
}

// truncateAtDetailBoundary removes trailing <details> sections from body
// until it fits within maxLen, preserving complete sections rather than
// cutting mid-content. Falls back to byte truncation if no sections remain.
func truncateAtDetailBoundary(body string, maxLen int) string {
	const detailOpen = "<details>"
	const detailClose = "</details>"
	for len(body) > maxLen {
		// Find the last complete <details>...</details> block.
		closeIdx := strings.LastIndex(body, detailClose)
		if closeIdx < 0 {
			break // no detail sections left, fall back to byte truncation
		}
		// Search backward from the close tag to find its opening tag.
		openIdx := strings.LastIndex(body[:closeIdx], detailOpen)
		if openIdx < 0 {
			break
		}
		// Remove this detail section (and any trailing whitespace).
		after := strings.TrimLeft(body[closeIdx+len(detailClose):], "\n\r ")
		body = body[:openIdx] + after
	}
	if len(body) > maxLen {
		body = body[:maxLen]
	}
	return strings.TrimRight(body, "\n\r ") + "\n\n*Output truncated — some detail sections removed (comment size limit).*\n"
}

// postComment posts the comment to the Discussion, truncating if needed.
func postComment(ctx context.Context, dryRun bool, name, discID, body string) {
	// Truncate if approaching GitHub's 65536 char limit.
	if len(body) > 60000 {
		ghActionsWarning(fmt.Sprintf("Comment for %s is %d chars. Truncating at detail boundaries.", name, len(body)))
		body = truncateAtDetailBoundary(body, 60000)
	}

	if dryRun {
		log.Printf("[dry-run] Would post comment for %s (%d chars)", name, len(body))
		return
	}

	out, err := graphQL(ctx, `mutation($id: ID!, $body: String!) {
		addDiscussionComment(input: { discussionId: $id, body: $body }) {
			comment { url }
		}
	}`, map[string]string{"id": discID, "body": body})
	if err != nil {
		log.Fatalf("Failed to post comment for %s:\n%s", name, out)
	}

	log.Printf("Posted comment for %s", name)
}

// ── Exec helpers ─────────────────────────────────────────────────

func execGH(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func execBinary(ctx context.Context, binary string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return string(out), err
}

func graphQL(ctx context.Context, query string, vars map[string]string) (string, error) {
	args := []string{"api", "graphql", "-f", "query=" + query}
	for k, v := range vars {
		args = append(args, "-f", k+"="+v)
	}
	return execGH(ctx, args...)
}

// ── JSON parsing helpers ─────────────────────────────────────────

func parseRepoAndCategory(jsonData, categoryName string) (repoID, catID string, err error) {
	var resp struct {
		Data struct {
			Repository struct {
				ID                   string `json:"id"`
				DiscussionCategories struct {
					Nodes []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"nodes"`
				} `json:"discussionCategories"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		return "", "", fmt.Errorf("failed to parse GraphQL response: %w", err)
	}
	if len(resp.Errors) > 0 {
		var msgs []string
		for _, e := range resp.Errors {
			msgs = append(msgs, e.Message)
		}
		return "", "", fmt.Errorf("GraphQL errors: %s", strings.Join(msgs, "; "))
	}
	repoID = resp.Data.Repository.ID
	if repoID == "" {
		return "", "", fmt.Errorf("repository not found")
	}
	for _, cat := range resp.Data.Repository.DiscussionCategories.Nodes {
		if cat.Name == categoryName {
			return repoID, cat.ID, nil
		}
	}
	return "", "", fmt.Errorf("discussion category %q not found — create it via Settings > Discussions > Categories", categoryName)
}

func parseDiscussion(jsonData string) (id, url string, err error) {
	var resp struct {
		Data struct {
			CreateDiscussion struct {
				Discussion struct {
					ID  string `json:"id"`
					URL string `json:"url"`
				} `json:"discussion"`
			} `json:"createDiscussion"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		return "", "", fmt.Errorf("failed to parse: %w", err)
	}
	d := resp.Data.CreateDiscussion.Discussion
	if d.ID == "" {
		return "", "", fmt.Errorf("discussion ID is empty")
	}
	return d.ID, d.URL, nil
}

// ── GitHub Actions helpers ───────────────────────────────────────

func inGitHubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

func ghActionsGroup(name string) {
	if inGitHubActions() {
		fmt.Printf("::group::%s\n", name)
	}
	log.Printf("── %s ──", name)
}

func ghActionsEndGroup() {
	if inGitHubActions() {
		fmt.Println("::endgroup::")
	}
}

func ghActionsWarning(msg string) {
	if inGitHubActions() {
		fmt.Printf("::warning::%s\n", msg)
	}
	log.Printf("WARN: %s", msg)
}

func ghActionsError(msg string) {
	if inGitHubActions() {
		fmt.Printf("::error::%s\n", msg)
	}
	log.Printf("ERROR: %s", msg)
}

// writeJobSummary appends a markdown summary to $GITHUB_STEP_SUMMARY.
// This shows up as a clickable summary on the Actions run page.
func writeJobSummary(discURL string, index []indexEntry) {
	summaryPath := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryPath == "" {
		return
	}
	f, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("Warning: could not write job summary: %v", err)
		return
	}
	defer f.Close()

	fmt.Fprintln(f, "## Velocity Showcase")
	fmt.Fprintln(f)
	fmt.Fprintf(f, "**Discussion:** %s\n\n", discURL)
	if link := workflowRunLink(); link != "" {
		fmt.Fprintf(f, "**Artifacts:** %s\n\n", link)
	}
	fmt.Fprintln(f, "| Repo | Status |")
	fmt.Fprintln(f, "|------|--------|")
	for _, e := range index {
		fmt.Fprintf(f, "| %s | `%s` |\n", e.Repo, e.Status)
	}
}

type indexEntry struct {
	Repo   string
	Status string
}

func workflowRunLink() string {
	server := os.Getenv("GITHUB_SERVER_URL")
	repo := os.Getenv("GITHUB_REPOSITORY")
	runID := os.Getenv("GITHUB_RUN_ID")
	if server != "" && repo != "" && runID != "" {
		return fmt.Sprintf("%s/%s/actions/runs/%s", server, repo, runID)
	}
	return ""
}
