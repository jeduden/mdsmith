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
// and a caret marker under the diagnostic column.
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

		if f.Color && !isDiagLine {
			// Context lines in dim
			if _, err := fmt.Fprintf(w, "\033[2m%*d | %s\033[0m\n", gutterWidth, lineNum, line); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "%*d | %s\n", gutterWidth, lineNum, line); err != nil {
				return err
			}
		}

		if isDiagLine && d.Column > 1 {
			caretPad := strings.Repeat("·", d.Column-1)
			gutterPad := strings.Repeat(" ", gutterWidth)
			if f.Color {
				if _, err := fmt.Fprintf(w, "%s | %s\033[31m^\033[0m\n", gutterPad, caretPad); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "%s | %s^\n", gutterPad, caretPad); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
