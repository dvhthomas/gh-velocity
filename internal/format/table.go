package format

import (
	"io"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
)

// NewTable creates a tableprinter that auto-truncates columns for TTY
// output and emits tab-separated values when piped.
func NewTable(w io.Writer, isTTY bool, width int) tableprinter.TablePrinter {
	if width <= 0 {
		width = 80
	}
	return tableprinter.New(w, isTTY, width)
}
