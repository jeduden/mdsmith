package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
)

// TextFormatter outputs diagnostics in human-readable text format.
// When Color is true, the file location is printed in cyan and the rule ID in yellow.
type TextFormatter struct {
	Color bool
}

// Format writes each diagnostic as a header line followed by an optional
// source snippet with line-number gutter and caret marker.
func (f *TextFormatter) Format(w io.Writer, diagnostics []lint.Diagnostic) error {
	for _, d := range diagnostics {
		var err error
		if f.Color {
			_, err = fmt.Fprintf(w, "\033[36m%s:%d:%d\033[0m \033[33m%s\033[0m %s\n",
				d.File, d.Line, d.Column, d.RuleID, d.Message)
		} else {
			_, err = fmt.Fprintf(w, "%s:%d:%d %s %s\n",
				d.File, d.Line, d.Column, d.RuleID, d.Message)
		}
		if err != nil {
			return err
		}

		if err := f.formatSnippet(w, d); err != nil {
			return err
		}
	}
	return nil
}

// formatSnippet writes the source context lines with a line-number gutter
// and a dot-leader caret line under the diagnostic line. The dots always
// start at column 1, creating a visual path that identifies which line
// is the diagnostic and (for Column>1) guides the eye to the exact column.
func (f *TextFormatter) formatSnippet(w io.Writer, d lint.Diagnostic) error {
	if len(d.SourceLines) == 0 {
		return nil
	}

	maxLineNum := d.SourceStartLine + len(d.SourceLines) - 1
	gutterWidth := len(fmt.Sprintf("%d", maxLineNum))
	if gutterWidth < 1 {
		gutterWidth = 1
	}

	for i, line := range d.SourceLines {
		lineNum := d.SourceStartLine + i
		isDiagLine := lineNum == d.Line

		if err := f.writeSourceLine(w, gutterWidth, lineNum, line, isDiagLine); err != nil {
			return err
		}

		if isDiagLine && d.Column > 0 {
			if err := f.writeCaretLine(w, gutterWidth, d.Column); err != nil {
				return err
			}
		}
	}

	return nil
}

// writeSourceLine writes a single source line with line-number gutter.
// Context lines are dimmed when color is on.
func (f *TextFormatter) writeSourceLine(w io.Writer, gutterWidth, lineNum int, line string, isDiag bool) error {
	format := "%*d | %s\n"
	if f.Color && !isDiag {
		format = "\033[2m%*d | %s\033[0m\n"
	}
	_, err := fmt.Fprintf(w, format, gutterWidth, lineNum, line)
	return err
}

// writeCaretLine writes a continuous dot path from column 0 to the caret,
// connecting visually with the > marker on the line above.
func (f *TextFormatter) writeCaretLine(w io.Writer, gutterWidth, column int) error {
	// The > line format is ">%*d | %s" so content column C starts at
	// position gutterWidth+3+C-1.  We fill everything before ^ with dots.
	dots := strings.Repeat("·", gutterWidth+column+2)
	format := "%s^\n"
	if f.Color {
		format = "%s\033[31m^\033[0m\n"
	}
	_, err := fmt.Fprintf(w, format, dots)
	return err
}
