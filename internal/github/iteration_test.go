package github

import (
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestParseIteration(t *testing.T) {
	tests := []struct {
		name    string
		input   iterationValueNode
		want    model.Iteration
		wantErr bool
	}{
		{
			name: "valid iteration",
			input: iterationValueNode{
				ID:        "iter1",
				Title:     "Sprint 5",
				StartDate: "2026-01-06",
				Duration:  14,
			},
			want: model.Iteration{
				ID:        "iter1",
				Title:     "Sprint 5",
				StartDate: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC),
				Duration:  14,
				EndDate:   time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "7-day sprint",
			input: iterationValueNode{
				ID:        "iter2",
				Title:     "Week 1",
				StartDate: "2026-03-03",
				Duration:  7,
			},
			want: model.Iteration{
				ID:        "iter2",
				Title:     "Week 1",
				StartDate: time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC),
				Duration:  7,
				EndDate:   time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "invalid date",
			input: iterationValueNode{
				ID:        "iter3",
				Title:     "Bad",
				StartDate: "not-a-date",
				Duration:  14,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIteration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseIteration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.ID != tt.want.ID || got.Title != tt.want.Title ||
				!got.StartDate.Equal(tt.want.StartDate) || got.Duration != tt.want.Duration ||
				!got.EndDate.Equal(tt.want.EndDate) {
				t.Errorf("parseIteration() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBuildVelocityItemsQuery(t *testing.T) {
	tests := []struct {
		name          string
		iterFieldName string
		numFieldName  string
		wantContains  []string
		wantMissing   []string
	}{
		{
			name:          "both fields",
			iterFieldName: "Sprint",
			numFieldName:  "Story Points",
			wantContains:  []string{`fieldValueByName(name: "Sprint")`, `fieldValueByName(name: "Story Points")`, "iterationId", "number"},
			wantMissing:   nil,
		},
		{
			name:          "iteration only",
			iterFieldName: "Sprint",
			numFieldName:  "",
			wantContains:  []string{`fieldValueByName(name: "Sprint")`, "iterationId"},
			wantMissing:   []string{"Story Points"},
		},
		{
			name:          "number only",
			iterFieldName: "",
			numFieldName:  "Points",
			wantContains:  []string{`fieldValueByName(name: "Points")`},
			wantMissing:   []string{"iterationId"},
		},
		{
			name:          "neither field",
			iterFieldName: "",
			numFieldName:  "",
			wantContains:  []string{"stateReason", "content"},
			wantMissing:   []string{"fieldValueByName"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := buildVelocityItemsQuery(tt.iterFieldName, tt.numFieldName, nil)
			for _, s := range tt.wantContains {
				if !contains(q, s) {
					t.Errorf("query should contain %q, got:\n%s", s, q)
				}
			}
			for _, s := range tt.wantMissing {
				if contains(q, s) {
					t.Errorf("query should NOT contain %q", s)
				}
			}
		})
	}

	// Test SingleSelect field fragments.
	t.Run("with single select fields", func(t *testing.T) {
		q := buildVelocityItemsQuery("", "", []string{"Size", "Priority"})
		for _, want := range []string{
			`ss0: fieldValueByName(name: "Size")`,
			`ss1: fieldValueByName(name: "Priority")`,
			"ProjectV2ItemFieldSingleSelectValue",
		} {
			if !contains(q, want) {
				t.Errorf("query should contain %q, got:\n%s", want, q)
			}
		}
	})
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
