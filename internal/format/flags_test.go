package format

import (
	"bytes"
	"testing"
	"time"
)

func TestFlagEmoji(t *testing.T) {
	tests := []struct {
		flag string
		want string
	}{
		{FlagOutlier, "🚩"},
		{FlagNoise, "🤖"},
		{FlagHotfix, "⚡"},
		{FlagBug, "🐛"},
		{FlagStale, "⏳"},
		{FlagAging, "🟡"},
		{"unknown", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			got := FlagEmoji(tt.flag)
			if got != tt.want {
				t.Errorf("FlagEmoji(%q) = %q, want %q", tt.flag, got, tt.want)
			}
		})
	}
}

func TestWriteCapNote(t *testing.T) {
	tests := []struct {
		name  string
		shown int
		total int
		want  string
	}{
		{"capped", 25, 142, "\nShowing 25 of 142 items (sorted by flag). Use --results json for full data.\n"},
		{"not capped", 10, 10, ""},
		{"zero total", 0, 0, ""},
		{"shown exceeds total", 30, 25, ""},
		{"single over", 25, 26, "\nShowing 25 of 26 items (sorted by flag). Use --results json for full data.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			WriteCapNote(&buf, tt.shown, tt.total)
			got := buf.String()
			if got != tt.want {
				t.Errorf("WriteCapNote(%d, %d) = %q, want %q", tt.shown, tt.total, got, tt.want)
			}
		})
	}
}

func dur(d time.Duration) *time.Duration { return &d }

func TestSortBy_Desc(t *testing.T) {
	type item struct {
		name string
		dur  *time.Duration
	}
	items := []item{
		{"short", dur(time.Minute)},
		{"nil", nil},
		{"long", dur(time.Hour)},
		{"mid", dur(30 * time.Minute)},
	}
	sorted := SortBy(items, "duration", Desc, func(it item) *time.Duration { return it.dur })

	want := []string{"long", "mid", "short", "nil"}
	for i, g := range sorted.Items {
		if g.name != want[i] {
			t.Errorf("SortBy Desc [%d] = %q, want %q", i, g.name, want[i])
		}
	}
	if sorted.Field != "duration" {
		t.Errorf("Field = %q, want %q", sorted.Field, "duration")
	}
	if sorted.Direction != Desc {
		t.Errorf("Direction = %q, want %q", sorted.Direction, Desc)
	}
	// Verify original is not mutated.
	if items[0].name != "short" {
		t.Errorf("SortBy mutated original slice")
	}
}

func TestSortBy_Asc(t *testing.T) {
	type item struct {
		name string
		dur  *time.Duration
	}
	items := []item{
		{"long", dur(time.Hour)},
		{"nil", nil},
		{"short", dur(time.Minute)},
	}
	sorted := SortBy(items, "duration", Asc, func(it item) *time.Duration { return it.dur })

	want := []string{"short", "long", "nil"}
	for i, g := range sorted.Items {
		if g.name != want[i] {
			t.Errorf("SortBy Asc [%d] = %q, want %q", i, g.name, want[i])
		}
	}
}

func TestSorted_Header(t *testing.T) {
	s := Sorted[int]{Field: "lead_time", Direction: Desc}

	if got := s.Header("lead_time", "Lead Time"); got != "Lead Time ↓" {
		t.Errorf("Header matching field = %q, want %q", got, "Lead Time ↓")
	}
	if got := s.Header("other", "Other"); got != "Other" {
		t.Errorf("Header non-matching field = %q, want %q", got, "Other")
	}

	s.Direction = Asc
	if got := s.Header("lead_time", "Lead Time"); got != "Lead Time ↑" {
		t.Errorf("Header asc = %q, want %q", got, "Lead Time ↑")
	}
}

func TestSorted_JSONSort(t *testing.T) {
	s := Sorted[int]{Field: "cycle_time", Direction: Desc}
	js := s.JSONSort()
	if js.Field != "cycle_time" || js.Direction != "desc" {
		t.Errorf("JSONSort = %+v, want field=cycle_time direction=desc", js)
	}
}
