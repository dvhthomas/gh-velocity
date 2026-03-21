package github

import (
	"testing"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestEnrichIssueTypes_SkipsAlreadySet(t *testing.T) {
	// Issues that already have IssueType set should not be overwritten.
	issues := []model.Issue{
		{Number: 1, Title: "Fix crash", IssueType: "Bug"},
		{Number: 2, Title: "Add feature", IssueType: "Feature"},
	}

	// Build targets — should be empty since all are already set.
	var needsEnrich int
	for _, iss := range issues {
		if iss.IssueType == "" {
			needsEnrich++
		}
	}
	if needsEnrich != 0 {
		t.Errorf("expected 0 issues needing enrichment, got %d", needsEnrich)
	}
}

func TestEnrichIssueTypes_EmptySlice(t *testing.T) {
	// EnrichIssueTypes should handle nil/empty input gracefully.
	c := &Client{
		owner: "test",
		repo:  "repo",
		cache: NewQueryCache(0),
	}

	// Should return nil without panicking, even with no gql client.
	err := c.EnrichIssueTypes(nil, nil)
	if err != nil {
		t.Errorf("expected nil error for nil input, got %v", err)
	}

	err = c.EnrichIssueTypes(nil, []model.Issue{})
	if err != nil {
		t.Errorf("expected nil error for empty input, got %v", err)
	}
}

func TestEnrichIssueTypes_SkipsAllSet(t *testing.T) {
	// When all issues already have IssueType, no API call should be made.
	c := &Client{
		owner: "test",
		repo:  "repo",
		cache: NewQueryCache(0),
	}

	issues := []model.Issue{
		{Number: 1, IssueType: "Bug"},
		{Number: 2, IssueType: "Feature"},
	}

	// Should return nil without making any API call (gql is nil — would panic if called).
	err := c.EnrichIssueTypes(nil, issues)
	if err != nil {
		t.Errorf("expected nil error when all types set, got %v", err)
	}
}

func TestEnrichBatchSize(t *testing.T) {
	if enrichBatchSize != 20 {
		t.Errorf("enrichBatchSize = %d, want 20 (must match FetchIssues pattern)", enrichBatchSize)
	}
}
