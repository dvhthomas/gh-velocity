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

func TestInjectMarkedSection(t *testing.T) {
	section := WrapWithMarker("issue", "42", "new metrics content")

	tests := []struct {
		name    string
		body    string
		wantHas string // substring that must appear
		wantNot string // substring that must NOT appear (empty = skip)
	}{
		{
			name:    "empty body",
			body:    "",
			wantHas: "<!-- gh-velocity:issue:42 -->",
		},
		{
			name:    "append to existing body",
			body:    "Original issue description.\n\nMore details here.",
			wantHas: "Original issue description.",
		},
		{
			name:    "append preserves body",
			body:    "Original issue description.",
			wantHas: "new metrics content",
		},
		{
			name:    "replace existing section",
			body:    "Description.\n\n<!-- gh-velocity:issue:42 -->\nold content\n<!-- /gh-velocity -->\n",
			wantHas: "new metrics content",
			wantNot: "old content",
		},
		{
			name:    "replace preserves surrounding",
			body:    "Before.\n\n<!-- gh-velocity:issue:42 -->\nold\n<!-- /gh-velocity -->\n\nAfter.",
			wantHas: "After.",
		},
		{
			name:    "different marker untouched",
			body:    "Body.\n\n<!-- gh-velocity:lead-time:42 -->\nlead time\n<!-- /gh-velocity -->\n",
			wantHas: "lead time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InjectMarkedSection(tt.body, "issue", "42", section)
			if tt.wantHas != "" && !contains(got, tt.wantHas) {
				t.Errorf("result missing %q:\n%s", tt.wantHas, got)
			}
			if tt.wantNot != "" && contains(got, tt.wantNot) {
				t.Errorf("result should not contain %q:\n%s", tt.wantNot, got)
			}
			// Always must contain the new marker
			if !contains(got, "<!-- gh-velocity:issue:42 -->") {
				t.Errorf("result missing new marker:\n%s", got)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
