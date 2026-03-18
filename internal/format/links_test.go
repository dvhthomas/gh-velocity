package format

import (
	"bytes"
	"testing"
)

func TestFormatItemLink(t *testing.T) {
	tests := []struct {
		name   string
		number int
		url    string
		rc     RenderContext
		want   string
	}{
		{
			name:   "markdown with URL",
			number: 42,
			url:    "https://github.com/owner/repo/issues/42",
			rc:     RenderContext{Format: Markdown},
			want:   "[#42](https://github.com/owner/repo/issues/42)",
		},
		{
			name:   "markdown without URL",
			number: 42,
			url:    "",
			rc:     RenderContext{Format: Markdown},
			want:   "#42",
		},
		{
			name:   "json ignores URL",
			number: 42,
			url:    "https://github.com/owner/repo/issues/42",
			rc:     RenderContext{Format: JSON},
			want:   "#42",
		},
		{
			name:   "pretty non-TTY",
			number: 42,
			url:    "https://github.com/owner/repo/issues/42",
			rc:     RenderContext{Format: Pretty, IsTTY: false},
			want:   "#42",
		},
		{
			name:   "pretty TTY without URL",
			number: 42,
			url:    "",
			rc:     RenderContext{Format: Pretty, IsTTY: true},
			want:   "#42",
		},
		{
			name:   "pretty TTY with URL contains OSC8",
			number: 42,
			url:    "https://github.com/owner/repo/issues/42",
			rc:     RenderContext{Format: Pretty, IsTTY: true},
		},
		{
			name:   "URL with control chars stripped",
			number: 1,
			url:    "https://evil.com/\x1b[31mred",
			rc:     RenderContext{Format: Markdown},
			want:   "[#1](https://evil.com/[31mred)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatItemLink(tt.number, tt.url, tt.rc)
			if tt.want != "" && got != tt.want {
				t.Errorf("FormatItemLink() = %q, want %q", got, tt.want)
			}
			// For TTY with URL, just verify the link text is present
			if tt.name == "pretty TTY with URL contains OSC8" {
				if got == "#42" {
					t.Error("expected OSC8 hyperlink, got plain text")
				}
			}
		})
	}
}

func TestFormatReleaseLink(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		url  string
		rc   RenderContext
		want string
	}{
		{
			name: "markdown with URL",
			tag:  "v1.0.0",
			url:  "https://github.com/owner/repo/releases/tag/v1.0.0",
			rc:   RenderContext{Format: Markdown},
			want: "[v1.0.0](https://github.com/owner/repo/releases/tag/v1.0.0)",
		},
		{
			name: "markdown without URL",
			tag:  "v1.0.0",
			url:  "",
			rc:   RenderContext{Format: Markdown},
			want: "v1.0.0",
		},
		{
			name: "json ignores URL",
			tag:  "v1.0.0",
			url:  "https://github.com/owner/repo/releases/tag/v1.0.0",
			rc:   RenderContext{Format: JSON},
			want: "v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatReleaseLink(tt.tag, tt.url, tt.rc)
			if got != tt.want {
				t.Errorf("FormatReleaseLink() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripControlChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"clean string", "https://github.com", "https://github.com"},
		{"null byte", "a\x00b", "ab"},
		{"escape sequence", "a\x1b[31mb", "a[31mb"},
		{"delete char", "a\x7fb", "ab"},
		{"tab removed", "a\tb", "ab"},
		{"newline removed", "a\nb", "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripControlChars(tt.input)
			if got != tt.want {
				t.Errorf("stripControlChars(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   string
	}{
		{"nil labels", nil, ""},
		{"empty labels", []string{}, ""},
		{"single label", []string{"bug"}, "bug"},
		{"multiple labels", []string{"bug", "enhancement"}, "bug, enhancement"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatLabels(tt.labels)
			if got != tt.want {
				t.Errorf("FormatLabels() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDocLink(t *testing.T) {
	got := DocLink("CV", "/concepts/statistics/#cv")
	want := "[CV](https://dvhthomas.github.io/gh-velocity/concepts/statistics/#cv)"
	if got != want {
		t.Errorf("DocLink() = %q, want %q", got, want)
	}
}

func TestLinkStatTerms(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "wraps CV in insight message",
			input: "Delivery times vary widely (CV 2.7)",
			want:  "Delivery times vary widely ([CV](https://dvhthomas.github.io/gh-velocity/concepts/statistics/#coefficient-of-variation-cv) 2.7)",
		},
		{
			name:  "no CV in message",
			input: "Mean 10d vs median 5d",
			want:  "Mean 10d vs median 5d",
		},
		{
			name:  "only first CV is linked",
			input: "(CV 1.0) and (CV 2.0)",
			want:  "([CV](https://dvhthomas.github.io/gh-velocity/concepts/statistics/#coefficient-of-variation-cv) 1.0) and (CV 2.0)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LinkStatTerms(tt.input)
			if got != tt.want {
				t.Errorf("LinkStatTerms() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripMarkdownLinks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"link with text", "[CV](url) is 2.7", "CV is 2.7"},
		{"issue link", "[#123](url) took 5d", "#123 took 5d"},
		{"no links", "no links here", "no links here"},
		{"multiple links", "[a](url1) and [b](url2)", "a and b"},
		{"nested brackets", "[[not a link]]", "[[not a link]]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripMarkdownLinks(tt.input)
			if got != tt.want {
				t.Errorf("StripMarkdownLinks(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRenderContext(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{
		Writer: &buf,
		Format: Pretty,
		IsTTY:  true,
		Width:  120,
		Owner:  "owner",
		Repo:   "repo",
	}

	if rc.Writer != &buf {
		t.Error("Writer mismatch")
	}
	if rc.Format != Pretty {
		t.Error("Format mismatch")
	}
	if !rc.IsTTY {
		t.Error("IsTTY should be true")
	}
	if rc.Width != 120 {
		t.Error("Width mismatch")
	}
}
