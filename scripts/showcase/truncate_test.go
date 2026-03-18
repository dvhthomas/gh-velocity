package main

import (
	"strings"
	"testing"
)

func TestTruncateAtDetailBoundary(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		maxLen     int
		wantKeep   []string // substrings that must be present
		wantRemove []string // substrings that must be absent
	}{
		{
			name:     "under limit keeps content and adds footer",
			body:     "hello world",
			maxLen:   1000,
			wantKeep: []string{"hello world", "Output truncated"},
		},
		{
			name: "removes last detail section first",
			body: "header\n<details>\n<summary>A</summary>\nsmall\n</details>\n\n" +
				"<details>\n<summary>B</summary>\n" + strings.Repeat("x", 200) + "\n</details>\n",
			maxLen:     150,
			wantKeep:   []string{"<summary>A</summary>"},
			wantRemove: []string{"<summary>B</summary>"},
		},
		{
			name: "removes multiple sections until fits",
			body: "header\n" +
				"<details>\n<summary>A</summary>\ndata\n</details>\n\n" +
				"<details>\n<summary>B</summary>\n" + strings.Repeat("x", 100) + "\n</details>\n\n" +
				"<details>\n<summary>C</summary>\n" + strings.Repeat("y", 100) + "\n</details>\n",
			maxLen:     100,
			wantKeep:   []string{"header"},
			wantRemove: []string{"<summary>B</summary>", "<summary>C</summary>"},
		},
		{
			name:     "preserves config detail section when possible",
			body:     "<details>\n<summary>Generated Config</summary>\nconfig\n</details>\n\n<details>\n<summary>Lead Time (215 issues)</summary>\n" + strings.Repeat("x", 500) + "\n</details>\n",
			maxLen:   200,
			wantKeep: []string{"Generated Config"},
		},
		{
			name:   "adds truncation notice",
			body:   "header\n<details>\n<summary>Big</summary>\n" + strings.Repeat("z", 500) + "\n</details>\n",
			maxLen: 100,
			wantKeep: []string{
				"Output truncated",
				"detail sections removed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateAtDetailBoundary(tt.body, tt.maxLen)
			for _, want := range tt.wantKeep {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q to be present, got:\n%s", want, got)
				}
			}
			for _, noWant := range tt.wantRemove {
				if strings.Contains(got, noWant) {
					t.Errorf("expected %q to be absent, got:\n%s", noWant, got)
				}
			}
		})
	}
}
