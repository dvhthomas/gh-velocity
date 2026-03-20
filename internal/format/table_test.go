package format

import (
	"bytes"
	"strings"
	"testing"
)

func TestTable_TSV(t *testing.T) {
	var buf bytes.Buffer
	tp := NewTable(&buf, false, 80)
	tp.AddHeader([]string{"Name", "Age"})
	tp.AddField("Alice")
	tp.AddField("30")
	tp.EndRow()
	tp.AddField("Bob")
	tp.AddField("25")
	tp.EndRow()

	if err := tp.Render(); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Name\tAge") {
		t.Errorf("want header with tabs, got %q", got)
	}
	if !strings.Contains(got, "Alice\t30") {
		t.Errorf("want Alice row with tabs, got %q", got)
	}
	if !strings.Contains(got, "Bob\t25") {
		t.Errorf("want Bob row with tabs, got %q", got)
	}
}

func TestTable_Lipgloss(t *testing.T) {
	var buf bytes.Buffer
	tp := NewTable(&buf, true, 60)
	tp.AddHeader([]string{"#", "Title", "Status"})
	tp.AddField("1")
	tp.AddField("Fix bug")
	tp.AddField("Open")
	tp.EndRow()

	if err := tp.Render(); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	got := buf.String()
	// Lipgloss table should contain the data.
	if !strings.Contains(got, "Fix bug") {
		t.Errorf("want 'Fix bug' in output, got %q", got)
	}
	// Should have rounded border characters.
	if !strings.Contains(got, "╭") {
		t.Errorf("want rounded border character ╭ in TTY output, got %q", got)
	}
	// Should NOT be tab-separated.
	if strings.Contains(got, "#\tTitle\tStatus") {
		t.Errorf("TTY output should not be plain TSV, got %q", got)
	}
}

func TestTable_EmptyTable(t *testing.T) {
	var buf bytes.Buffer
	tp := NewTable(&buf, false, 80)
	tp.AddHeader([]string{"A", "B"})

	if err := tp.Render(); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "A\tB") {
		t.Errorf("want header even with no rows, got %q", got)
	}
}

func TestTable_PartialRow(t *testing.T) {
	var buf bytes.Buffer
	tp := NewTable(&buf, false, 80)
	tp.AddField("partial")
	tp.AddField("row")
	// No EndRow call — Render should flush.

	if err := tp.Render(); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "partial\trow") {
		t.Errorf("want partial row flushed, got %q", got)
	}
}

func TestTable_SanitizesOSC8InLipgloss(t *testing.T) {
	var buf bytes.Buffer
	tp := NewTable(&buf, true, 60)
	tp.AddHeader([]string{"#", "Title"})
	// Simulate FormatItemLink's OSC 8 hyperlink output
	tp.AddField("\x1b]8;;https://github.com/test/repo/issues/42\x07#42\x1b]8;;\x07")
	tp.AddField("Fix the bug")
	tp.EndRow()

	if err := tp.Render(); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	got := buf.String()
	// OSC 8 should be stripped, leaving just "#42"
	if strings.Contains(got, "]8;;") {
		t.Errorf("lipgloss output should strip OSC 8 sequences, got %q", got)
	}
	if !strings.Contains(got, "#42") {
		t.Errorf("want visible text '#42' preserved, got %q", got)
	}
	if !strings.Contains(got, "Fix the bug") {
		t.Errorf("want 'Fix the bug' in output, got %q", got)
	}
}

func TestTable_SanitizesControlCharsInLipgloss(t *testing.T) {
	var buf bytes.Buffer
	tp := NewTable(&buf, true, 60)
	tp.AddHeader([]string{"Title"})
	tp.AddField("Normal\x1b[2Jinjected")
	tp.EndRow()

	if err := tp.Render(); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	got := buf.String()
	// The injected clear-screen sequence \x1b[2J should NOT appear intact.
	// sanitizeForLipgloss strips ESC bytes, so [2J remains as harmless printable text.
	if strings.Contains(got, "\x1b[2J") {
		t.Errorf("lipgloss output should not contain injected escape sequence \\x1b[2J")
	}
}

func TestSanitizeForLipgloss(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello", "hello"},
		{"OSC 8 with BEL", "\x1b]8;;https://example.com\x07click\x1b]8;;\x07", "click"},
		{"control chars", "bad\x1b[2Jtext", "bad[2Jtext"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeForLipgloss(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeForLipgloss(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTable_DefaultWidth(t *testing.T) {
	var buf bytes.Buffer
	tp := NewTable(&buf, true, 0) // zero width
	tp.AddHeader([]string{"X"})
	tp.AddField("data")
	tp.EndRow()

	if err := tp.Render(); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	// Should not panic; default width kicks in.
	if !strings.Contains(buf.String(), "data") {
		t.Errorf("want 'data' in output")
	}
}
