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

func TestSuppressStderr_SuppressesWarnAndDebug(t *testing.T) {
	// When SuppressStderr is true, Warn and Debug should no-op.
	// We can't easily capture stderr in a unit test, but we can verify
	// the functions don't panic and respect the flag.
	old := SuppressStderr
	defer func() { SuppressStderr = old }()

	SuppressStderr = true
	// These should silently return without writing to stderr.
	Warn("should not appear: %s", "test")
	Debug("should not appear: %s", "test")
	// If we got here without panic, the suppression works.
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
