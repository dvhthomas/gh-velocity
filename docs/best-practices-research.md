# Best Practices Research: Go-Based GitHub CLI Extension Development

> Researched March 2026. Sources include official GitHub documentation, go-gh and go-github
> library docs, Cobra framework docs, community guides, and real-world extension examples.

---

## Table of Contents

1. [Building GitHub CLI Extensions in Go](#1-building-github-cli-extensions-in-go)
2. [Using the go-github Library](#2-using-the-go-github-library)
3. [Go CLI Frameworks and Cobra](#3-go-cli-frameworks-and-cobra)
4. [Table-Driven Testing Patterns](#4-table-driven-testing-patterns)
5. [Multi-Format Output (JSON, CSV, Markdown, Pretty-Print)](#5-multi-format-output)
6. [GitHub Projects v2 GraphQL API](#6-github-projects-v2-graphql-api)

---

## 1. Building GitHub CLI Extensions in Go

### 1.1 Scaffolding

```bash
gh extension create --precompiled=go my-extension
```

This generates a repository named `gh-my-extension` with:

```
gh-my-extension/
  main.go                    # Entry point, uses go-gh
  go.mod                     # Module: github.com/<user>/gh-my-extension
  go.sum
  .github/
    workflows/
      release.yml            # Uses gh-extension-precompile action
```

The default template is minimal -- a single `main.go` that imports `github.com/cli/go-gh/v2`.
It does NOT include Cobra (the gh team intentionally avoids favoring a specific CLI framework).
For anything beyond a trivial extension, you should restructure to a standard Go layout.

### 1.2 Recommended Project Structure (for non-trivial extensions)

```
gh-my-extension/
  main.go                          # Minimal: calls cmd.Execute()
  cmd/
    root.go                        # Root cobra.Command
    list.go                        # Subcommand: list
    update.go                      # Subcommand: update
  internal/
    github/
      client.go                    # API client wrapper (REST + GraphQL)
      queries.go                   # GraphQL query definitions
      types.go                     # API response types
    output/
      formatter.go                 # Output format interface + implementations
      table.go                     # Table/pretty-print formatter
      json.go                      # JSON formatter
      csv.go                       # CSV formatter
      markdown.go                  # Markdown formatter
  go.mod
  go.sum
  .github/
    workflows/
      release.yml                  # Cross-compilation via gh-extension-precompile
      test.yml                     # CI testing
```

### 1.3 Authentication

**This is the most important architectural decision for gh extensions.**

gh extensions automatically inherit the user's `gh` authentication. The `go-gh` library
handles this transparently:

```go
import "github.com/cli/go-gh/v2/pkg/api"

// Automatically uses the user's gh auth token
// Respects GH_TOKEN and GH_HOST environment variables
// Falls back to the user's stored OAuth token from `gh auth login`
client, err := api.DefaultRESTClient()
client, err := api.DefaultGraphQLClient()
```

If you need to use google/go-github instead of go-gh's built-in clients:

```go
import (
    "os/exec"
    "strings"
    "github.com/google/go-github/v68/github"
)

func getGHToken() (string, error) {
    // Option 1: Use GH_TOKEN env var (set by gh when running extensions)
    if token := os.Getenv("GH_TOKEN"); token != "" {
        return token, nil
    }

    // Option 2: Shell out to gh auth token
    out, err := exec.Command("gh", "auth", "token").Output()
    if err != nil {
        return "", fmt.Errorf("gh auth token failed: %w", err)
    }
    return strings.TrimSpace(string(out)), nil
}

func newGitHubClient() (*github.Client, error) {
    token, err := getGHToken()
    if err != nil {
        return nil, err
    }
    return github.NewClient(nil).WithAuthToken(token), nil
}
```

**Better approach** -- use go-gh's auth package directly:

```go
import (
    "github.com/cli/go-gh/v2/pkg/auth"
    "github.com/google/go-github/v68/github"
)

func newGitHubClient() (*github.Client, error) {
    token, _ := auth.TokenForHost("github.com")
    if token == "" {
        return nil, fmt.Errorf("not authenticated; run 'gh auth login'")
    }
    return github.NewClient(nil).WithAuthToken(token), nil
}
```

**Important**: For Projects v2 API access, users must have the `project` scope:
```bash
gh auth refresh --scopes project
# or during initial login:
gh auth login --scopes project
```

### 1.4 Build and Release

The scaffolded `.github/workflows/release.yml` uses the `gh-extension-precompile` action
which automatically produces cross-compiled binaries following the naming convention:

```
gh-EXTENSION-NAME-OS-ARCHITECTURE[.exe]
```

Examples:
- `gh-velocity-linux-amd64`
- `gh-velocity-darwin-arm64`
- `gh-velocity-windows-amd64.exe`

To release:
```bash
git tag v1.0.0
git push origin v1.0.0
# The GitHub Action builds and attaches binaries to the release
```

Users install with:
```bash
gh extension install owner/gh-velocity
```

---

## 2. Using the go-github Library (google/go-github)

### 2.1 When to Use go-github vs go-gh

| Feature | go-gh (`cli/go-gh/v2`) | go-github (`google/go-github`) |
|---------|------------------------|-------------------------------|
| REST API | `pkg/api.RESTClient` | Full typed service layer |
| GraphQL API | `pkg/api.GraphQLClient` (raw queries) | Not included (REST only) |
| Auth | Automatic from gh CLI | Manual token management |
| Type safety | Responses are `interface{}` or custom structs | Fully typed request/response structs |
| GitHub Projects v2 | Via GraphQL client (raw queries) | Not supported (REST-only library) |
| Terminal output | `pkg/tableprinter`, `pkg/template` | Not included |
| Repository detection | `pkg/repository.Current()` | Not included |

**Recommendation**: Use go-gh for:
- Authentication (always)
- GraphQL queries (especially Projects v2)
- Terminal-aware output (tableprinter)
- Repository context detection

Use go-github for:
- Complex REST API operations with full type safety
- When you need typed request/response structures
- Operations well-served by REST (issues, PRs, repos)

**They work well together** -- use go-gh for auth and plumbing, go-github for typed API calls.

### 2.2 Client Creation with go-github

```go
import "github.com/google/go-github/v68/github"

// Simple: personal access token
client := github.NewClient(nil).WithAuthToken("your-token")

// From gh CLI token (recommended for extensions)
token, _ := auth.TokenForHost("github.com")
client := github.NewClient(nil).WithAuthToken(token)

// Unauthenticated (very low rate limits)
client := github.NewClient(nil)
```

### 2.3 GraphQL vs REST Considerations

**GitHub Projects v2 is GraphQL-only.** There is no REST API for Projects v2.

For a tool that needs Projects v2 data, you MUST use GraphQL via either:
- `go-gh/v2/pkg/api.GraphQLClient` (recommended for gh extensions)
- `shurcooL/graphql` (standalone GraphQL client)
- Raw HTTP POST to `https://api.github.com/graphql`

The go-gh GraphQL client supports both struct-based queries and raw query strings:

```go
// Struct-based (shurcooL-style)
var query struct {
    Organization struct {
        ProjectV2 struct {
            Items struct {
                Nodes []struct {
                    Id string
                }
            } `graphql:"items(first: 100)"`
        } `graphql:"projectV2(number: $number)"`
    } `graphql:"organization(login: $org)"`
}

variables := map[string]interface{}{
    "org":    graphql.String("myorg"),
    "number": graphql.Int(1),
}

err := client.Query("ProjectItems", &query, variables)

// Raw query string (simpler for complex ProjectV2 queries)
var result map[string]interface{}
err := client.Do(`
    query($org: String!, $number: Int!) {
        organization(login: $org) {
            projectV2(number: $number) {
                items(first: 100) {
                    nodes { id }
                }
            }
        }
    }
`, map[string]interface{}{
    "org":    "myorg",
    "number": 1,
}, &result)
```

### 2.4 Testing with go-github-mock

The `migueleliasweb/go-github-mock` library provides clean mocking for go-github:

```go
import (
    "github.com/google/go-github/v68/github"
    "github.com/migueleliasweb/go-github-mock/src/mock"
)

func TestListRepos(t *testing.T) {
    mockedHTTPClient := mock.NewMockedHTTPClient(
        mock.WithRequestMatch(
            mock.GetOrgsReposByOrg,
            []github.Repository{
                {Name: github.Ptr("repo-one")},
                {Name: github.Ptr("repo-two")},
            },
        ),
    )

    client := github.NewClient(mockedHTTPClient)
    repos, _, err := client.Repositories.ListByOrg(
        context.Background(), "myorg", nil,
    )

    assert.NoError(t, err)
    assert.Len(t, repos, 2)
    assert.Equal(t, "repo-one", repos[0].GetName())
}
```

**Mocking errors:**
```go
mockedHTTPClient := mock.NewMockedHTTPClient(
    mock.WithRequestMatchHandler(
        mock.GetUsersByUsername,
        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            mock.WriteError(w, http.StatusInternalServerError, "server error")
        }),
    ),
)
```

**Mocking pagination:**
```go
mockedHTTPClient := mock.NewMockedHTTPClient(
    mock.WithRequestMatchPages(
        mock.GetOrgsReposByOrg,
        []github.Repository{{Name: github.Ptr("page1-repo")}},
        []github.Repository{{Name: github.Ptr("page2-repo")}},
    ),
)
```

### 2.5 Testing GraphQL (httptest approach)

For GraphQL queries (not covered by go-github-mock), use `net/http/httptest`:

```go
func TestProjectQuery(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "data": map[string]interface{}{
                "organization": map[string]interface{}{
                    "projectV2": map[string]interface{}{
                        "items": map[string]interface{}{
                            "nodes": []map[string]interface{}{
                                {"id": "PVTI_123"},
                            },
                        },
                    },
                },
            },
        })
    }))
    defer server.Close()

    // Create go-gh client pointed at test server
    client, _ := api.NewGraphQLClient(api.ClientOptions{
        Host:      server.URL,
        AuthToken: "fake-token",
        Transport: server.Client().Transport,
    })

    // Run your query function under test
    items, err := fetchProjectItems(client, "myorg", 1)
    assert.NoError(t, err)
    assert.Len(t, items, 1)
}
```

---

## 3. Go CLI Frameworks and Cobra

### 3.1 Cobra Is the De Facto Standard

Cobra is used by the `gh` CLI itself, Kubernetes (`kubectl`), Hugo, Docker CLI, and hundreds
of other Go CLI tools. It is the standard choice for Go CLI applications.

### 3.2 Cobra with gh Extensions

The default `gh extension create --precompiled=go` template does NOT include Cobra.
The gh team has intentionally avoided bundling it (see cli/cli#7774). However, most
non-trivial gh extensions adopt Cobra for subcommand support.

**Recommended pattern for a gh extension with Cobra:**

```go
// main.go
package main

import "github.com/yourname/gh-velocity/cmd"

func main() {
    cmd.Execute()
}
```

```go
// cmd/root.go
package cmd

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
)

var (
    outputFormat string
)

var rootCmd = &cobra.Command{
    Use:   "velocity",
    Short: "Track velocity across GitHub Projects",
    Long:  `gh-velocity provides insights into team velocity using GitHub Projects v2.`,
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}

func init() {
    rootCmd.PersistentFlags().StringVarP(&outputFormat, "format", "f", "table",
        "Output format: table, json, csv, markdown")
}
```

```go
// cmd/list.go
package cmd

import (
    "github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
    Use:   "list",
    Short: "List project items with velocity data",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Implementation here
        return nil
    },
}

func init() {
    rootCmd.AddCommand(listCmd)
    listCmd.Flags().StringP("project", "p", "", "Project number")
    listCmd.Flags().StringP("org", "o", "", "Organization name")
}
```

### 3.3 Key Cobra Patterns

- Use `RunE` (not `Run`) to return errors instead of calling `os.Exit`
- Use `PersistentFlags` on root for flags shared across all subcommands (like `--format`)
- Use `Flags` on individual commands for command-specific flags
- Use `cobra.MinimumNArgs(1)` or `cobra.ExactArgs(2)` for argument validation
- Use `SilenceUsage: true` on root to avoid printing usage on runtime errors
- Use `SilenceErrors: true` if you handle error display yourself

---

## 4. Table-Driven Testing Patterns

### 4.1 Standard Go Table-Driven Test

This is THE idiomatic Go testing pattern:

```go
func TestFormatDuration(t *testing.T) {
    tests := []struct {
        name     string
        input    time.Duration
        expected string
    }{
        {
            name:     "zero duration",
            input:    0,
            expected: "0s",
        },
        {
            name:     "hours and minutes",
            input:    2*time.Hour + 30*time.Minute,
            expected: "2h30m",
        },
        {
            name:     "negative duration",
            input:    -5 * time.Minute,
            expected: "-5m",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := FormatDuration(tt.input)
            if result != tt.expected {
                t.Errorf("FormatDuration(%v) = %q, want %q",
                    tt.input, result, tt.expected)
            }
        })
    }
}
```

### 4.2 Table-Driven Tests for CLI Commands

For testing cobra commands in a gh extension:

```go
func TestListCommand(t *testing.T) {
    tests := []struct {
        name           string
        args           []string
        setupMock      func(*mockGitHubClient)
        expectedOutput string
        expectedErr    string
    }{
        {
            name: "list items in table format",
            args: []string{"list", "--org", "myorg", "--project", "1", "--format", "json"},
            setupMock: func(m *mockGitHubClient) {
                m.items = []ProjectItem{
                    {Title: "Issue 1", Status: "In Progress"},
                    {Title: "Issue 2", Status: "Done"},
                }
            },
            expectedOutput: `[{"title":"Issue 1","status":"In Progress"},{"title":"Issue 2","status":"Done"}]`,
            expectedErr:    "",
        },
        {
            name:        "missing required org flag",
            args:        []string{"list", "--project", "1"},
            setupMock:   func(m *mockGitHubClient) {},
            expectedErr: "required flag \"org\" not set",
        },
        {
            name: "API error",
            args: []string{"list", "--org", "myorg", "--project", "1"},
            setupMock: func(m *mockGitHubClient) {
                m.err = fmt.Errorf("API rate limit exceeded")
            },
            expectedErr: "API rate limit exceeded",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mock := &mockGitHubClient{}
            tt.setupMock(mock)

            buf := new(bytes.Buffer)
            cmd := NewListCmd(mock, buf)
            cmd.SetArgs(tt.args)
            cmd.SetOut(buf)
            cmd.SetErr(buf)

            err := cmd.Execute()

            if tt.expectedErr != "" {
                assert.ErrorContains(t, err, tt.expectedErr)
            } else {
                assert.NoError(t, err)
                assert.Contains(t, buf.String(), tt.expectedOutput)
            }
        })
    }
}
```

### 4.3 Testing Pattern: Dependency Injection for Testability

The key to testable CLI code is injecting dependencies rather than creating them in
the command handler:

```go
// Define an interface for your API operations
type ProjectClient interface {
    ListItems(ctx context.Context, org string, projectNum int) ([]ProjectItem, error)
    UpdateItemStatus(ctx context.Context, itemID, statusOptionID string) error
}

// Command factory accepts the interface
func NewListCmd(client ProjectClient, out io.Writer) *cobra.Command {
    return &cobra.Command{
        Use:   "list",
        Short: "List project items",
        RunE: func(cmd *cobra.Command, args []string) error {
            org, _ := cmd.Flags().GetString("org")
            project, _ := cmd.Flags().GetInt("project")
            items, err := client.ListItems(cmd.Context(), org, project)
            if err != nil {
                return err
            }
            return renderItems(out, items, outputFormat)
        },
    }
}

// In tests, use a mock implementation
type mockProjectClient struct {
    items []ProjectItem
    err   error
}

func (m *mockProjectClient) ListItems(ctx context.Context, org string, num int) ([]ProjectItem, error) {
    return m.items, m.err
}
```

---

## 5. Multi-Format Output

### 5.1 Architecture: Format Interface Pattern

```go
// internal/output/formatter.go
package output

import "io"

type Format string

const (
    FormatTable    Format = "table"
    FormatJSON     Format = "json"
    FormatCSV      Format = "csv"
    FormatMarkdown Format = "markdown"
)

type Formatter interface {
    Format(w io.Writer, data interface{}) error
}

func NewFormatter(format Format, isTTY bool, width int) Formatter {
    switch format {
    case FormatJSON:
        return &JSONFormatter{Pretty: isTTY}
    case FormatCSV:
        return &CSVFormatter{}
    case FormatMarkdown:
        return &MarkdownFormatter{}
    default:
        return &TableFormatter{IsTTY: isTTY, Width: width}
    }
}
```

### 5.2 JSON Formatter

```go
// internal/output/json.go
package output

import (
    "encoding/json"
    "io"
)

type JSONFormatter struct {
    Pretty bool
}

func (f *JSONFormatter) Format(w io.Writer, data interface{}) error {
    enc := json.NewEncoder(w)
    if f.Pretty {
        enc.SetIndent("", "  ")
    }
    return enc.Encode(data)
}
```

### 5.3 CSV Formatter

```go
// internal/output/csv.go
package output

import (
    "encoding/csv"
    "io"
)

type CSVFormatter struct{}

type CSVable interface {
    CSVHeaders() []string
    CSVRows() [][]string
}

func (f *CSVFormatter) Format(w io.Writer, data interface{}) error {
    csvData, ok := data.(CSVable)
    if !ok {
        return fmt.Errorf("data does not implement CSVable")
    }
    writer := csv.NewWriter(w)
    defer writer.Flush()

    if err := writer.Write(csvData.CSVHeaders()); err != nil {
        return err
    }
    return writer.WriteAll(csvData.CSVRows())
}
```

### 5.4 Table Formatter (using go-gh tableprinter)

```go
// internal/output/table.go
package output

import (
    "io"
    "github.com/cli/go-gh/v2/pkg/tableprinter"
)

type TableFormatter struct {
    IsTTY bool
    Width int
}

type Tabular interface {
    Headers() []string
    Rows() [][]string
}

func (f *TableFormatter) Format(w io.Writer, data interface{}) error {
    tabData, ok := data.(Tabular)
    if !ok {
        return fmt.Errorf("data does not implement Tabular")
    }

    tp := tableprinter.New(w, f.IsTTY, f.Width)

    // Add headers
    headers := tabData.Headers()
    tp.AddHeader(headers)

    // Add rows
    for _, row := range tabData.Rows() {
        for _, cell := range row {
            tp.AddField(cell)
        }
        tp.EndRow()
    }

    return tp.Render()
}
```

### 5.5 TTY Detection

```go
import "github.com/cli/go-gh/v2/pkg/term"

terminal := term.FromEnv()
isTTY := terminal.IsTerminalOutput()
width, _, _ := terminal.Size()
```

### 5.6 How gh Itself Handles Output

The gh CLI pattern is:
- **TTY**: Pretty table with colors, truncated to terminal width
- **Non-TTY (piped)**: TSV format (tab-separated, no colors, no truncation)
- **--json flag**: JSON output with optional `--jq` filtering and `--template` formatting

This is the convention users expect from gh extensions.

---

## 6. GitHub Projects v2 GraphQL API

### 6.1 Important Prerequisites

**Scope requirement**: Users must authenticate with the `project` scope:
```bash
gh auth refresh --scopes project
```

Without this scope, all Projects v2 queries will fail with permission errors.

### 6.2 Finding a Project

```graphql
# By organization
query($org: String!, $number: Int!) {
  organization(login: $org) {
    projectV2(number: $number) {
      id
      title
    }
  }
}

# By user
query($login: String!, $number: Int!) {
  user(login: $login) {
    projectV2(number: $number) {
      id
      title
    }
  }
}
```

### 6.3 Querying Project Fields (including custom fields)

This is essential -- you need field IDs before you can read or write field values:

```graphql
query($projectId: ID!) {
  node(id: $projectId) {
    ... on ProjectV2 {
      fields(first: 20) {
        nodes {
          ... on ProjectV2Field {
            id
            name
            dataType          # TEXT, NUMBER, DATE, SINGLE_SELECT, ITERATION
          }
          ... on ProjectV2IterationField {
            id
            name
            configuration {
              iterations {
                startDate
                id
              }
            }
          }
          ... on ProjectV2SingleSelectField {
            id
            name
            options {
              id
              name              # e.g., "Todo", "In Progress", "Done"
            }
          }
        }
      }
    }
  }
}
```

**Field types you will encounter:**
- `ProjectV2Field` -- generic fields (Text, Number, Date)
- `ProjectV2SingleSelectField` -- Status and other dropdowns (has `options`)
- `ProjectV2IterationField` -- Sprint/iteration fields (has `configuration.iterations`)

### 6.4 Querying Items with All Field Values

```graphql
query($org: String!, $number: Int!) {
  organization(login: $org) {
    projectV2(number: $number) {
      items(first: 100, after: $cursor) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          id
          content {
            ... on Issue {
              title
              url
              number
              state
              assignees(first: 10) {
                nodes { login }
              }
              labels(first: 10) {
                nodes { name }
              }
            }
            ... on PullRequest {
              title
              url
              number
              state
              assignees(first: 10) {
                nodes { login }
              }
            }
            ... on DraftIssue {
              title
              body
            }
          }
          status: fieldValueByName(name: "Status") {
            ... on ProjectV2ItemFieldSingleSelectValue {
              name
              optionId
            }
          }
          startDate: fieldValueByName(name: "Start Date") {
            ... on ProjectV2ItemFieldDateValue {
              date
            }
          }
          targetDate: fieldValueByName(name: "Target Date") {
            ... on ProjectV2ItemFieldDateValue {
              date
            }
          }
          sprint: fieldValueByName(name: "Sprint") {
            ... on ProjectV2ItemFieldIterationValue {
              title
              startDate
              duration
            }
          }
          estimate: fieldValueByName(name: "Estimate") {
            ... on ProjectV2ItemFieldNumberValue {
              number
            }
          }
        }
      }
    }
  }
}
```

**Key technique**: Use `fieldValueByName` with aliases (e.g., `status:`, `startDate:`)
to query custom fields by their display name. This avoids needing to know field IDs
upfront for read operations.

### 6.5 Updating Field Values

**Update Status (Single Select):**
```graphql
mutation($projectId: ID!, $itemId: ID!, $fieldId: ID!, $optionId: String!) {
  updateProjectV2ItemFieldValue(
    input: {
      projectId: $projectId
      itemId: $itemId
      fieldId: $fieldId
      value: { singleSelectOptionId: $optionId }
    }
  ) {
    projectV2Item { id }
  }
}
```

**Update Date fields (Start Date, Target Date):**
```graphql
mutation($projectId: ID!, $itemId: ID!, $fieldId: ID!, $date: Date!) {
  updateProjectV2ItemFieldValue(
    input: {
      projectId: $projectId
      itemId: $itemId
      fieldId: $fieldId
      value: { date: $date }
    }
  ) {
    projectV2Item { id }
  }
}
```

**Update Iteration field:**
```graphql
mutation($projectId: ID!, $itemId: ID!, $fieldId: ID!, $iterationId: String!) {
  updateProjectV2ItemFieldValue(
    input: {
      projectId: $projectId
      itemId: $itemId
      fieldId: $fieldId
      value: { iterationId: $iterationId }
    }
  ) {
    projectV2Item { id }
  }
}
```

**Important constraint**: You cannot add an item and update its fields in the same
mutation call. You must first `addProjectV2ItemById`, then `updateProjectV2ItemFieldValue`.

### 6.6 Pagination

Projects can have hundreds of items. Always implement pagination:

```go
func fetchAllItems(client *api.GraphQLClient, org string, projectNum int) ([]Item, error) {
    var allItems []Item
    var cursor *string

    for {
        var result struct {
            Organization struct {
                ProjectV2 struct {
                    Items struct {
                        PageInfo struct {
                            HasNextPage bool
                            EndCursor   string
                        }
                        Nodes []Item
                    }
                }
            }
        }

        variables := map[string]interface{}{
            "org":    org,
            "number": projectNum,
            "cursor": cursor,
        }

        err := client.Do(projectItemsQuery, variables, &result)
        if err != nil {
            return nil, err
        }

        items := result.Organization.ProjectV2.Items
        allItems = append(allItems, items.Nodes...)

        if !items.PageInfo.HasNextPage {
            break
        }
        cursor = &items.PageInfo.EndCursor
    }

    return allItems, nil
}
```

### 6.7 Go Types for ProjectV2 Responses

```go
type ProjectItem struct {
    ID      string
    Content ItemContent
    Status  *SingleSelectValue `json:"status"`
    StartDate   *DateValue     `json:"startDate"`
    TargetDate  *DateValue     `json:"targetDate"`
    Sprint      *IterationValue `json:"sprint"`
    Estimate    *NumberValue    `json:"estimate"`
}

type ItemContent struct {
    TypeName string `json:"__typename"`
    Title    string
    URL      string
    Number   int
    State    string
    Assignees struct {
        Nodes []struct{ Login string }
    }
    Labels struct {
        Nodes []struct{ Name string }
    }
}

type SingleSelectValue struct {
    Name     string
    OptionId string
}

type DateValue struct {
    Date string  // ISO 8601 format: "2026-03-15"
}

type IterationValue struct {
    Title     string
    StartDate string
    Duration  int
}

type NumberValue struct {
    Number float64
}

type ProjectField struct {
    ID       string
    Name     string
    DataType string
    Options  []FieldOption  // Only for SingleSelect
}

type FieldOption struct {
    ID   string
    Name string
}
```

---

## Summary of Key Recommendations

| Decision | Recommendation | Source |
|----------|---------------|--------|
| CLI framework | Cobra (same as gh itself) | Community consensus |
| Auth in extensions | go-gh `pkg/auth` or `DefaultGraphQLClient()` | Official go-gh docs |
| REST API client | go-github for typed access, go-gh for simple calls | Library docs |
| GraphQL client | go-gh `pkg/api.GraphQLClient` with raw queries | go-gh docs |
| Projects v2 API | GraphQL only (no REST support) | GitHub docs |
| Testing | Table-driven + dependency injection + go-github-mock | Go community standard |
| Output formatting | Format interface with go-gh tableprinter for tables | go-gh docs, gh patterns |
| TTY detection | go-gh `pkg/term.FromEnv()` | go-gh docs |
| Cross-compilation | `gh-extension-precompile` GitHub Action | GitHub docs |

---

## Sources

- [Creating GitHub CLI Extensions - GitHub Docs](https://docs.github.com/en/github-cli/github-cli/creating-github-cli-extensions)
- [cli/go-gh - GitHub](https://github.com/cli/go-gh)
- [go-gh v2 API package - pkg.go.dev](https://pkg.go.dev/github.com/cli/go-gh/v2/pkg/api)
- [go-gh v2 tableprinter - pkg.go.dev](https://pkg.go.dev/github.com/cli/go-gh/v2/pkg/tableprinter)
- [google/go-github - GitHub](https://github.com/google/go-github)
- [go-github-mock - GitHub](https://github.com/migueleliasweb/go-github-mock)
- [spf13/cobra - GitHub](https://github.com/spf13/cobra)
- [Using the API to manage Projects - GitHub Docs](https://docs.github.com/en/issues/planning-and-tracking-with-projects/automating-your-project/using-the-api-to-manage-projects)
- [Intro to GraphQL with GitHub Projects - Some Natalie](https://some-natalie.dev/blog/graphql-intro/)
- [GitHub Projects GraphQL examples - DevOps Journal](https://devopsjournal.io/blog/2022/11/28/github-graphql-queries)
- [Extending the gh CLI with Go - Mike Ball](https://mikeball.info/blog/extending-the-gh-cli-with-go/)
- [Cobra CLI issue for gh extension templates - cli/cli#7774](https://github.com/cli/cli/issues/7774)
- [Go Unit Testing Best Practices - Rost Glukhov](https://www.glukhov.org/post/2025/11/unit-tests-in-go/)
- [go-pretty table library - GitHub](https://github.com/jedib0t/go-pretty)
