// Package log provides structured stderr output that adapts to CI environments.
// When GITHUB_ACTIONS=true, messages use workflow commands (::error::, ::warning::, ::notice::).
// Otherwise, plain text is written to stderr.
package log

import (
	"fmt"
	"os"
	"strings"
)

// isCI returns true when running inside GitHub Actions.
func isCI() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// Warn writes a warning message to stderr.
// In GitHub Actions: ::warning::message
// Locally: warning: message
func Warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if isCI() {
		fmt.Fprintf(os.Stderr, "::warning::%s\n", escapeCI(msg))
	} else {
		fmt.Fprintf(os.Stderr, "warning: %s\n", msg)
	}
}

// Error writes an error message to stderr.
// In GitHub Actions: ::error::message
// Locally: plain message
func Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if isCI() {
		fmt.Fprintf(os.Stderr, "::error::%s\n", escapeCI(msg))
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
}

// Notice writes an informational notice to stderr.
// In GitHub Actions: ::notice::message
// Locally: plain message
func Notice(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if isCI() {
		fmt.Fprintf(os.Stderr, "::notice::%s\n", escapeCI(msg))
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
}

// Debug writes a debug message to stderr (same format in CI and local).
func Debug(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[debug] %s\n", msg)
}

// escapeCI URL-encodes newlines and other special characters for GitHub Actions
// workflow commands, which require single-line messages.
func escapeCI(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}
