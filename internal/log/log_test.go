package log

import (
	"testing"
)

func TestEscapeCI(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello world", "hello world"},
		{"newline", "line1\nline2", "line1%0Aline2"},
		{"carriage return", "line1\r\nline2", "line1%0D%0Aline2"},
		{"percent sign", "100% done", "100%25 done"},
		{"mixed", "err: 50%\ndetail", "err: 50%25%0Adetail"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeCI(tt.in)
			if got != tt.want {
				t.Errorf("escapeCI(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsCI(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	if !isCI() {
		t.Error("expected isCI() = true when GITHUB_ACTIONS=true")
	}

	t.Setenv("GITHUB_ACTIONS", "")
	if isCI() {
		t.Error("expected isCI() = false when GITHUB_ACTIONS is empty")
	}
}
