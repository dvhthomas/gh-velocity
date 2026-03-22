package format

import "testing"

func TestFormatOwnerMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain login", input: "liruiluo", want: "`@liruiluo`"},
		{name: "prefixed login", input: "@zanieb", want: "`@zanieb`"},
		{name: "bot login", input: "renovate[bot]", want: "`@renovate[bot]`"},
		{name: "unassigned", input: "unassigned", want: "unassigned"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatOwnerMarkdown(tt.input)
			if got != tt.want {
				t.Fatalf("formatOwnerMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
