package github

import (
	"testing"
	"time"
)

func makeUser(login string) *struct {
	Login string `json:"login"`
} {
	return &struct {
		Login string `json:"login"`
	}{Login: login}
}

func makeLabels(names ...string) []struct {
	Name string `json:"name"`
} {
	out := make([]struct {
		Name string `json:"name"`
	}, len(names))
	for i, n := range names {
		out[i].Name = n
	}
	return out
}

func makeAssignees(logins ...string) []struct {
	Login string `json:"login"`
} {
	out := make([]struct {
		Login string `json:"login"`
	}, len(logins))
	for i, l := range logins {
		out[i].Login = l
	}
	return out
}

func makePR(mergedAt *time.Time) *struct {
	MergedAt *time.Time `json:"merged_at"`
} {
	return &struct {
		MergedAt *time.Time `json:"merged_at"`
	}{MergedAt: mergedAt}
}

func TestSearchItemToIssue(t *testing.T) {
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC)

	item := searchIssueResponse{
		Number:      42,
		Title:       "Fix login bug",
		State:       "closed",
		StateReason: "completed",
		CreatedAt:   now,
		UpdatedAt:   now.Add(24 * time.Hour),
		ClosedAt:    &closed,
		HTMLURL:     "https://github.com/org/repo/issues/42",
		User:        makeUser("alice"),
		Labels:      makeLabels("bug", "priority:high"),
		Assignees:   makeAssignees("bob", "carol"),
	}

	issue := searchItemToIssue(item)

	if issue.Number != 42 {
		t.Errorf("Number = %d, want 42", issue.Number)
	}
	if issue.Title != "Fix login bug" {
		t.Errorf("Title = %q, want %q", issue.Title, "Fix login bug")
	}
	if issue.State != "closed" {
		t.Errorf("State = %q, want %q", issue.State, "closed")
	}
	if issue.StateReason != "completed" {
		t.Errorf("StateReason = %q, want %q", issue.StateReason, "completed")
	}
	if len(issue.Labels) != 2 {
		t.Fatalf("Labels len = %d, want 2", len(issue.Labels))
	}
	if issue.Labels[0] != "bug" || issue.Labels[1] != "priority:high" {
		t.Errorf("Labels = %v, want [bug priority:high]", issue.Labels)
	}
	if len(issue.Assignees) != 2 {
		t.Fatalf("Assignees len = %d, want 2", len(issue.Assignees))
	}
	if issue.Assignees[0] != "bob" || issue.Assignees[1] != "carol" {
		t.Errorf("Assignees = %v, want [bob carol]", issue.Assignees)
	}
	if issue.URL != "https://github.com/org/repo/issues/42" {
		t.Errorf("URL = %q, want %q", issue.URL, "https://github.com/org/repo/issues/42")
	}
}

func TestSearchItemToIssue_NoAssignees(t *testing.T) {
	item := searchIssueResponse{
		Number:    1,
		Title:     "No assignees",
		State:     "open",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		HTMLURL:   "https://github.com/org/repo/issues/1",
	}

	issue := searchItemToIssue(item)

	if len(issue.Assignees) != 0 {
		t.Errorf("Assignees len = %d, want 0", len(issue.Assignees))
	}
}

func TestSearchItemToPR(t *testing.T) {
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	merged := time.Date(2026, 3, 17, 8, 0, 0, 0, time.UTC)

	item := searchIssueResponse{
		Number:      99,
		Title:       "Add feature X",
		State:       "closed",
		CreatedAt:   now,
		UpdatedAt:   now.Add(48 * time.Hour),
		HTMLURL:     "https://github.com/org/repo/pull/99",
		User:        makeUser("alice"),
		Labels:      makeLabels("enhancement"),
		Assignees:   makeAssignees("dave"),
		Draft:       false,
		PullRequest: makePR(&merged),
	}

	pr := searchItemToPR(item)

	if pr.Number != 99 {
		t.Errorf("Number = %d, want 99", pr.Number)
	}
	if pr.Title != "Add feature X" {
		t.Errorf("Title = %q, want %q", pr.Title, "Add feature X")
	}
	if pr.Author != "alice" {
		t.Errorf("Author = %q, want %q", pr.Author, "alice")
	}
	if len(pr.Assignees) != 1 || pr.Assignees[0] != "dave" {
		t.Errorf("Assignees = %v, want [dave]", pr.Assignees)
	}
	if pr.Draft != false {
		t.Errorf("Draft = %v, want false", pr.Draft)
	}
	if pr.MergedAt == nil || !pr.MergedAt.Equal(merged) {
		t.Errorf("MergedAt = %v, want %v", pr.MergedAt, merged)
	}
	if !pr.UpdatedAt.Equal(now.Add(48 * time.Hour)) {
		t.Errorf("UpdatedAt = %v, want %v", pr.UpdatedAt, now.Add(48*time.Hour))
	}
	if pr.URL != "https://github.com/org/repo/pull/99" {
		t.Errorf("URL = %q, want %q", pr.URL, "https://github.com/org/repo/pull/99")
	}
}

func TestSearchItemToPR_Draft(t *testing.T) {
	now := time.Now()

	item := searchIssueResponse{
		Number:    50,
		Title:     "WIP: Draft PR",
		State:     "open",
		CreatedAt: now,
		UpdatedAt: now,
		HTMLURL:   "https://github.com/org/repo/pull/50",
		User:      makeUser("eve"),
		Draft:     true,
		Assignees: makeAssignees("eve", "frank"),
	}

	pr := searchItemToPR(item)

	if pr.Draft != true {
		t.Errorf("Draft = %v, want true", pr.Draft)
	}
	if len(pr.Assignees) != 2 {
		t.Fatalf("Assignees len = %d, want 2", len(pr.Assignees))
	}
	if pr.Assignees[0] != "eve" || pr.Assignees[1] != "frank" {
		t.Errorf("Assignees = %v, want [eve frank]", pr.Assignees)
	}
	if pr.MergedAt != nil {
		t.Errorf("MergedAt = %v, want nil", pr.MergedAt)
	}
}

func TestSearchItemToPR_NoAssignees(t *testing.T) {
	item := searchIssueResponse{
		Number:    10,
		Title:     "Simple fix",
		State:     "open",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		HTMLURL:   "https://github.com/org/repo/pull/10",
		User:      makeUser("alice"),
	}

	pr := searchItemToPR(item)

	if len(pr.Assignees) != 0 {
		t.Errorf("Assignees len = %d, want 0", len(pr.Assignees))
	}
	if pr.Draft != false {
		t.Errorf("Draft = %v, want false", pr.Draft)
	}
}
