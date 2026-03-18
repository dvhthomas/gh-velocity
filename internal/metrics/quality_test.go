package metrics

import (
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

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
