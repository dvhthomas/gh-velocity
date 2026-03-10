package posting

import (
	"testing"
)

func TestMarkerKey(t *testing.T) {
	tests := []struct {
		command string
		context string
		want    string
	}{
		{"lead-time", "42", "gh-velocity:lead-time:42"},
		{"cycle-time", "pr-5", "gh-velocity:cycle-time:pr-5"},
		{"report", "30d", "gh-velocity:report:30d"},
		{"release", "v1.0.0", "gh-velocity:release:v1.0.0"},
		{"report", "2026-01-01..2026-02-01", "gh-velocity:report:2026-01-01..2026-02-01"},
	}
	for _, tt := range tests {
		t.Run(tt.command+":"+tt.context, func(t *testing.T) {
			got := MarkerKey(tt.command, tt.context)
			if got != tt.want {
				t.Errorf("MarkerKey(%q, %q) = %q, want %q", tt.command, tt.context, got, tt.want)
			}
		})
	}
}

func TestWrapWithMarker(t *testing.T) {
	content := "| Metric | Value |\n| --- | --- |\n| Lead Time | 3d 4h |\n"
	got := WrapWithMarker("lead-time", "42", content)

	want := "<!-- gh-velocity:lead-time:42 -->\n" + content + "\n<!-- /gh-velocity -->\n"
	if got != want {
		t.Errorf("WrapWithMarker mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFindMarker(t *testing.T) {
	body := "<!-- gh-velocity:lead-time:42 -->\nsome content\n<!-- /gh-velocity -->\n"

	tests := []struct {
		name    string
		body    string
		command string
		context string
		want    bool
	}{
		{"exact match", body, "lead-time", "42", true},
		{"different context", body, "lead-time", "99", false},
		{"different command", body, "cycle-time", "42", false},
		{"empty body", "", "lead-time", "42", false},
		{"missing closing tag (still findable)", "<!-- gh-velocity:lead-time:42 -->\ncontent", "lead-time", "42", true},
		{"partial match in context", "<!-- gh-velocity:lead-time:421 -->", "lead-time", "42", false},
		{"special chars in context", "<!-- gh-velocity:release:v1.0.0-rc.1 -->", "release", "v1.0.0-rc.1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindMarker(tt.body, tt.command, tt.context)
			if got != tt.want {
				t.Errorf("FindMarker(%q, %q) = %v, want %v", tt.command, tt.context, got, tt.want)
			}
		})
	}
}
