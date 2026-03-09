package metrics

import (
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestReleaseComposition(t *testing.T) {
	bugLabels := []string{"bug"}
	featureLabels := []string{"enhancement"}

	tests := []struct {
		name         string
		issues       []model.Issue
		wantBug      float64
		wantFeature  float64
		wantOther    float64
	}{
		{
			name:   "empty",
			issues: nil,
		},
		{
			name: "all bugs",
			issues: []model.Issue{
				{Labels: []string{"bug"}},
				{Labels: []string{"bug", "urgent"}},
			},
			wantBug: 1.0,
		},
		{
			name: "mixed",
			issues: []model.Issue{
				{Labels: []string{"bug"}},
				{Labels: []string{"enhancement"}},
				{Labels: []string{"documentation"}},
				{Labels: []string{"chore"}},
			},
			wantBug:     0.25,
			wantFeature: 0.25,
			wantOther:   0.5,
		},
		{
			name: "no labels",
			issues: []model.Issue{
				{Labels: nil},
				{Labels: []string{}},
			},
			wantOther: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bug, feature, other := ReleaseComposition(tt.issues, bugLabels, featureLabels)
			if bug != tt.wantBug {
				t.Errorf("bug ratio: want %v, got %v", tt.wantBug, bug)
			}
			if feature != tt.wantFeature {
				t.Errorf("feature ratio: want %v, got %v", tt.wantFeature, feature)
			}
			if other != tt.wantOther {
				t.Errorf("other ratio: want %v, got %v", tt.wantOther, other)
			}
		})
	}
}

func TestIsHotfix(t *testing.T) {
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		curr   model.Release
		prev   model.Release
		window float64
		want   bool
	}{
		{
			name:   "within window",
			curr:   model.Release{TagName: "v1.0.1", CreatedAt: base.Add(24 * time.Hour)},
			prev:   model.Release{TagName: "v1.0.0", CreatedAt: base},
			window: 72,
			want:   true,
		},
		{
			name:   "outside window",
			curr:   model.Release{TagName: "v1.1.0", CreatedAt: base.Add(7 * 24 * time.Hour)},
			prev:   model.Release{TagName: "v1.0.0", CreatedAt: base},
			window: 72,
			want:   false,
		},
		{
			name:   "no previous release",
			curr:   model.Release{TagName: "v1.0.0", CreatedAt: base},
			prev:   model.Release{},
			window: 72,
			want:   false,
		},
		{
			name:   "exact boundary",
			curr:   model.Release{TagName: "v1.0.1", CreatedAt: base.Add(72 * time.Hour)},
			prev:   model.Release{TagName: "v1.0.0", CreatedAt: base},
			window: 72,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsHotfix(tt.curr, tt.prev, tt.window)
			if got != tt.want {
				t.Errorf("want %v, got %v", tt.want, got)
			}
		})
	}
}
