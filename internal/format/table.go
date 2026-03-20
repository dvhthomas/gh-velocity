package format

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// Table wraps lipgloss/table for TTY output and plain tab-separated text
// for piped (non-TTY) output. Callers buffer rows via AddHeader/AddField/EndRow,
// then call Render to produce output.
type Table struct {
	w     io.Writer
	isTTY bool
	width int

	headers []string
	rows    [][]string
	current []string
}

// NewTable creates a Table that renders styled output for TTY
// and tab-separated values when piped.
func NewTable(w io.Writer, isTTY bool, width int) *Table {
	if width <= 0 {
		width = 80
	}
	return &Table{
		w:     w,
		isTTY: isTTY,
		width: width,
	}
}

// AddHeader sets the column headers. Takes a []string for backward
// compatibility with existing callers.
func (t *Table) AddHeader(cols []string) {
	t.headers = cols
}

// AddField appends a field value to the current row being built.
func (t *Table) AddField(text string) {
	t.current = append(t.current, text)
}

// EndRow finalises the current row and starts a new one.
func (t *Table) EndRow() {
	t.rows = append(t.rows, t.current)
	t.current = nil
}

// Render writes the table to the underlying writer. For TTY output it
// produces a lipgloss-styled table with rounded borders and bold headers.
// For non-TTY output it produces plain tab-separated text.
func (t *Table) Render() error {
	// Flush any partially built row.
	if len(t.current) > 0 {
		t.EndRow()
	}

	if !t.isTTY {
		return t.renderTSV()
	}
	return t.renderLipgloss()
}

// renderTSV writes plain tab-separated output, matching the previous
// go-gh tableprinter non-TTY behavior.
func (t *Table) renderTSV() error {
	if len(t.headers) > 0 {
		_, err := fmt.Fprintln(t.w, strings.Join(t.headers, "\t"))
		if err != nil {
			return err
		}
	}
	for _, row := range t.rows {
		_, err := fmt.Fprintln(t.w, strings.Join(row, "\t"))
		if err != nil {
			return err
		}
	}
	return nil
}

// osc8Re matches OSC 8 hyperlink sequences: ESC ] 8 ; params ; URI ST ... ESC ] 8 ; ; ST
// and extracts the visible display text between the open and close sequences.
var osc8Re = regexp.MustCompile(`\x1b\]8;[^;]*;[^\x07\x1b]*(?:\x07|\x1b\\)(.*?)\x1b\]8;;\x07|\x1b\]8;;\x1b\\`)

// sanitizeForLipgloss strips OSC 8 hyperlinks (preserving visible text) and
// removes control characters from cell content. lipgloss does not understand
// OSC 8 sequences and would miscount their width.
func sanitizeForLipgloss(s string) string {
	s = osc8Re.ReplaceAllString(s, "$1")
	return stripControlChars(s)
}

// renderLipgloss builds a styled table with rounded borders, bold headers,
// and right-aligned numeric-looking columns.
func (t *Table) renderLipgloss() error {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1)

	cellStyle := lipgloss.NewStyle().
		Padding(0, 1)

	borderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	// Sanitize all cell values: strip OSC 8 hyperlinks and control chars.
	sanitizedRows := make([][]string, len(t.rows))
	for i, row := range t.rows {
		sanitizedRows[i] = make([]string, len(row))
		for j, cell := range row {
			sanitizedRows[i][j] = sanitizeForLipgloss(cell)
		}
	}

	tbl := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		Width(t.width).
		Wrap(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return cellStyle
		})

	if len(t.headers) > 0 {
		tbl = tbl.Headers(t.headers...)
	}

	if len(sanitizedRows) > 0 {
		tbl = tbl.Rows(sanitizedRows...)
	}

	_, err := fmt.Fprintln(t.w, tbl.String())
	return err
}
