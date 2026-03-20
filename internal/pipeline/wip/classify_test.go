package wip

import (
	"testing"
)

func TestClassifyItem(t *testing.T) {
	t.Parallel()

	inProgress := []string{"label:in-progress", "label:wip"}
	inReview := []string{"label:in-review", "label:needs-review"}

	tests := []struct {
		name               string
		labels             []string
		issueType          string
		title              string
		isPR               bool
		isDraft            bool
		inProgressMatchers []string
		inReviewMatchers   []string
		wantStage          string
		wantMatcher        string
		wantExcluded       bool
	}{
		{
			name:               "issue matches in-progress label",
			labels:             []string{"in-progress", "bug"},
			inProgressMatchers: inProgress,
			inReviewMatchers:   inReview,
			wantStage:          "In Progress",
			wantMatcher:        "label:in-progress",
		},
		{
			name:               "issue matches in-review label",
			labels:             []string{"in-review"},
			inProgressMatchers: inProgress,
			inReviewMatchers:   inReview,
			wantStage:          "In Review",
			wantMatcher:        "label:in-review",
		},
		{
			name:               "in-review takes precedence over in-progress",
			labels:             []string{"in-progress", "in-review"},
			inProgressMatchers: inProgress,
			inReviewMatchers:   inReview,
			wantStage:          "In Review",
			wantMatcher:        "label:in-review",
		},
		{
			name:               "PR with matching label uses label stage",
			labels:             []string{"in-progress"},
			isPR:               true,
			isDraft:            false,
			inProgressMatchers: inProgress,
			inReviewMatchers:   inReview,
			wantStage:          "In Progress",
			wantMatcher:        "label:in-progress",
		},
		{
			name:               "PR with no matching label and draft -> In Progress",
			labels:             []string{"enhancement"},
			isPR:               true,
			isDraft:            true,
			inProgressMatchers: inProgress,
			inReviewMatchers:   inReview,
			wantStage:          "In Progress",
			wantMatcher:        "draft",
		},
		{
			name:               "PR with no matching label and non-draft -> In Review",
			labels:             []string{"enhancement"},
			isPR:               true,
			isDraft:            false,
			inProgressMatchers: inProgress,
			inReviewMatchers:   inReview,
			wantStage:          "In Review",
			wantMatcher:        "open-pr",
		},
		{
			name:               "PR label takes precedence over native signal",
			labels:             []string{"in-review"},
			isPR:               true,
			isDraft:            true, // draft, but label says in-review
			inProgressMatchers: inProgress,
			inReviewMatchers:   inReview,
			wantStage:          "In Review",
			wantMatcher:        "label:in-review",
		},
		{
			name:               "issue with no match is excluded",
			labels:             []string{"enhancement"},
			isPR:               false,
			inProgressMatchers: inProgress,
			inReviewMatchers:   inReview,
			wantExcluded:       true,
		},
		{
			name:               "multiple matchers - first match wins",
			labels:             []string{"wip"},
			inProgressMatchers: inProgress, // "label:in-progress", "label:wip"
			inReviewMatchers:   inReview,
			wantStage:          "In Progress",
			wantMatcher:        "label:wip",
		},
		{
			name:               "type matcher in-progress",
			labels:             []string{},
			issueType:          "Bug",
			inProgressMatchers: []string{"type:Bug"},
			inReviewMatchers:   inReview,
			wantStage:          "In Progress",
			wantMatcher:        "type:Bug",
		},
		{
			name:               "empty matchers - issue excluded",
			labels:             []string{"something"},
			inProgressMatchers: nil,
			inReviewMatchers:   nil,
			wantExcluded:       true,
		},
		{
			name:               "empty matchers - PR uses native signal",
			labels:             []string{},
			isPR:               true,
			isDraft:            false,
			inProgressMatchers: nil,
			inReviewMatchers:   nil,
			wantStage:          "In Review",
			wantMatcher:        "open-pr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stage, matcher, excluded := classifyItem(
				tt.labels, tt.issueType, tt.title,
				tt.isPR, tt.isDraft,
				tt.inProgressMatchers, tt.inReviewMatchers,
			)

			if excluded != tt.wantExcluded {
				t.Errorf("excluded = %v, want %v", excluded, tt.wantExcluded)
				return
			}
			if tt.wantExcluded {
				return
			}
			if stage != tt.wantStage {
				t.Errorf("stage = %q, want %q", stage, tt.wantStage)
			}
			if matcher != tt.wantMatcher {
				t.Errorf("matcher = %q, want %q", matcher, tt.wantMatcher)
			}
		})
	}
}
