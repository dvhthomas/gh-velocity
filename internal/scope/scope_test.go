package scope

import (
	"strings"
	"testing"
	"time"
)

func TestQuery_Build(t *testing.T) {
	tests := []struct {
		name  string
		query Query
		want  string
	}{
		{
			name: "all parts",
			query: Query{
				Scope:     "repo:myorg/myrepo label:bug",
				Type:      "is:issue",
				Lifecycle: "is:closed closed:2026-01-01..2026-02-01",
			},
			want: "repo:myorg/myrepo label:bug is:issue is:closed closed:2026-01-01..2026-02-01",
		},
		{
			name: "scope only",
			query: Query{
				Scope: "repo:myorg/myrepo",
			},
			want: "repo:myorg/myrepo",
		},
		{
			name: "lifecycle only",
			query: Query{
				Lifecycle: "is:closed",
			},
			want: "is:closed",
		},
		{
			name:  "empty",
			query: Query{},
			want:  "",
		},
		{
			name: "scope and type only",
			query: Query{
				Scope: "repo:myorg/myrepo",
				Type:  "is:pr",
			},
			want: "repo:myorg/myrepo is:pr",
		},
		{
			name: "whitespace trimmed",
			query: Query{
				Scope:     "  repo:myorg/myrepo  ",
				Lifecycle: "  is:closed  ",
			},
			want: "repo:myorg/myrepo is:closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.query.Build()
			if got != tt.want {
				t.Errorf("Build() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQuery_URL(t *testing.T) {
	q := Query{
		Scope:     "repo:myorg/myrepo",
		Type:      "is:issue",
		Lifecycle: "is:closed",
	}

	u := q.URL()
	if !strings.HasPrefix(u, "https://github.com/issues?q=") {
		t.Errorf("URL() should start with GitHub issues URL, got %q", u)
	}
	if !strings.Contains(u, "repo") {
		t.Errorf("URL() should contain encoded query, got %q", u)
	}

	// Empty query returns empty URL.
	empty := Query{}
	if got := empty.URL(); got != "" {
		t.Errorf("empty URL() = %q, want empty", got)
	}
}

func TestQuery_Verbose(t *testing.T) {
	q := Query{
		Scope:     "repo:myorg/myrepo",
		Type:      "is:issue",
		Lifecycle: "is:closed",
	}

	v := q.Verbose()
	if !strings.Contains(v, "[scope]") {
		t.Error("Verbose() should contain [scope]")
	}
	if !strings.Contains(v, "[type]") {
		t.Error("Verbose() should contain [type]")
	}
	if !strings.Contains(v, "[lifecycle]") {
		t.Error("Verbose() should contain [lifecycle]")
	}
	if !strings.Contains(v, "[query]") {
		t.Error("Verbose() should contain [query]")
	}
	if !strings.Contains(v, "[url]") {
		t.Error("Verbose() should contain [url]")
	}

	// Empty parts should be omitted.
	partial := Query{Lifecycle: "is:closed"}
	pv := partial.Verbose()
	if strings.Contains(pv, "[scope]") {
		t.Error("Verbose() should omit empty [scope]")
	}
	if strings.Contains(pv, "[type]") {
		t.Error("Verbose() should omit empty [type]")
	}
}

func TestClosedIssueQuery(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	q := ClosedIssueQuery("repo:myorg/myrepo", since, until)

	got := q.Build()
	want := "repo:myorg/myrepo is:issue is:closed closed:2026-01-01T00:00:00Z..2026-02-01T00:00:00Z"
	if got != want {
		t.Errorf("ClosedIssueQuery().Build() = %q, want %q", got, want)
	}
}

func TestMergedPRQuery(t *testing.T) {
	since := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	q := MergedPRQuery("repo:myorg/myrepo label:bug", since, until)

	got := q.Build()
	want := "repo:myorg/myrepo label:bug is:pr is:merged merged:2026-03-01T00:00:00Z..2026-03-15T00:00:00Z"
	if got != want {
		t.Errorf("MergedPRQuery().Build() = %q, want %q", got, want)
	}
}

func TestClosedIssuesByAuthorQuery(t *testing.T) {
	since := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	q := ClosedIssuesByAuthorQuery("repo:owner/repo", "testuser", since, until)
	built := q.Build()

	for _, want := range []string{"repo:owner/repo", "is:issue", "is:closed", "author:testuser", "closed:"} {
		if !strings.Contains(built, want) {
			t.Errorf("query %q missing %q", built, want)
		}
	}
}

func TestMergedPRsByAuthorQuery(t *testing.T) {
	since := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	q := MergedPRsByAuthorQuery("repo:owner/repo", "testuser", since, until)
	built := q.Build()

	for _, want := range []string{"repo:owner/repo", "is:pr", "is:merged", "author:testuser", "merged:"} {
		if !strings.Contains(built, want) {
			t.Errorf("query %q missing %q", built, want)
		}
	}
}

func TestReviewedPRsByAuthorQuery(t *testing.T) {
	since := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	q := ReviewedPRsByAuthorQuery("repo:owner/repo", "testuser", since, until)
	built := q.Build()

	for _, want := range []string{"repo:owner/repo", "is:pr", "reviewed-by:testuser", "updated:"} {
		if !strings.Contains(built, want) {
			t.Errorf("query %q missing %q", built, want)
		}
	}
}

func TestAuthorQueryWithExcludeUsers(t *testing.T) {
	since := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	q := ClosedIssuesByAuthorQuery("repo:owner/repo", "testuser", since, until)
	q.ExcludeUsers = BuildExclusions([]string{"dependabot[bot]"})
	built := q.Build()

	if !strings.Contains(built, "-author:dependabot[bot]") {
		t.Errorf("query %q missing exclude_users", built)
	}
}

func TestMergeScope(t *testing.T) {
	tests := []struct {
		name   string
		config string
		flag   string
		want   string
	}{
		{"both", "repo:myorg/myrepo", "label:bug", "repo:myorg/myrepo label:bug"},
		{"config only", "repo:myorg/myrepo", "", "repo:myorg/myrepo"},
		{"flag only", "", "label:bug", "label:bug"},
		{"neither", "", "", ""},
		{"whitespace config", "  repo:test  ", "", "repo:test"},
		{"whitespace flag", "", "  label:bug  ", "label:bug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeScope(tt.config, tt.flag)
			if got != tt.want {
				t.Errorf("MergeScope(%q, %q) = %q, want %q", tt.config, tt.flag, got, tt.want)
			}
		})
	}
}
