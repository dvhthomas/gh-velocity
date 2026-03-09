package linking

import (
	"testing"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestExtractIssueNumbers(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    []int
	}{
		{
			name:    "hash reference",
			message: "fix typo in README #42",
			want:    []int{42},
		},
		{
			name:    "parenthesized hash",
			message: "add feature (#15)",
			want:    []int{15},
		},
		{
			name:    "fixes keyword",
			message: "fixes #7",
			want:    []int{7},
		},
		{
			name:    "closes keyword",
			message: "closes #10",
			want:    []int{10},
		},
		{
			name:    "resolves keyword",
			message: "Resolves #99",
			want:    []int{99},
		},
		{
			name:    "multiple references",
			message: "fixes #1, closes #2, also see #3",
			want:    []int{1, 2, 3},
		},
		{
			name:    "no duplicates",
			message: "fixes #5 and also #5",
			want:    []int{5},
		},
		{
			name:    "no references",
			message: "update dependencies",
			want:    nil,
		},
		{
			name:    "case insensitive closes",
			message: "FIXES #12",
			want:    []int{12},
		},
		{
			name:    "hash at start of message",
			message: "#8 fix login",
			want:    []int{8},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractIssueNumbers(tt.message)
			if len(got) != len(tt.want) {
				t.Fatalf("want %v, got %v", tt.want, got)
			}
			for i, n := range got {
				if n != tt.want[i] {
					t.Errorf("index %d: want %d, got %d", i, tt.want[i], n)
				}
			}
		})
	}
}

func TestLinkCommitsToIssues(t *testing.T) {
	commits := []model.Commit{
		{SHA: "aaa", Message: "fix #1"},
		{SHA: "bbb", Message: "closes #1, also #2"},
		{SHA: "ccc", Message: "no reference here"},
		{SHA: "ddd", Message: "resolves #3"},
	}

	links := LinkCommitsToIssues(commits)

	if len(links[1]) != 2 {
		t.Errorf("issue #1: expected 2 commits, got %d", len(links[1]))
	}
	if len(links[2]) != 1 {
		t.Errorf("issue #2: expected 1 commit, got %d", len(links[2]))
	}
	if len(links[3]) != 1 {
		t.Errorf("issue #3: expected 1 commit, got %d", len(links[3]))
	}
}
