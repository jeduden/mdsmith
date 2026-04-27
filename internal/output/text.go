package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
)

// sanitizeControl strips all C0/C1 control characters from s.
// Used for diagnostic header fields (file path, message) where
// newlines and tabs could break the single-line format.
func sanitizeControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f ||
			(r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}

// sanitizeSourceLine strips C0/C1 control characters from s but
// preserves tab for source line indentation display.
func sanitizeSourceLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f ||
			(r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}

// TextFormatter outputs diagnostics in human-readable text format.
// When Color is true, the file location is printed in cyan and the rule ID in yellow.
type TextFormatter struct {
	Color bool
}

// Format writes each diagnostic as a header line followed by an optional
// source snippet with line-number gutter and caret marker.
func (f *TextFormatter) Format(w io.Writer, diagnostics []lint.Diagnostic) error {
	for _, d := range diagnostics {
		safeFile := sanitizeControl(d.File)
		safeMsg := sanitizeControl(d.Message)
		var err error
		if f.Color {
			_, err = fmt.Fprintf(w, "\033[36m%s:%d:%d\033[0m \033[33m%s\033[0m %s\n",
				safeFile, d.Line, d.Column, d.RuleID, safeMsg)
		} else {
			_, err = fmt.Fprintf(w, "%s:%d:%d %s %s\n",
				safeFile, d.Line, d.Column, d.RuleID, safeMsg)
		}
		if err != nil {
			return err
		}

		if err := f.formatSnippet(w, d); err != nil {
			return err
		}

		if err := f.formatExplanation(w, d.Explanation); err != nil {
			return err
		}
	}
	return nil
}

// formatExplanation writes a one-line trailer naming the rule and
// the winning source of each leaf setting that contributed to the
// rule's effective config. No-op when explanation is nil.
//
// Rule names, leaf paths, leaf values, and source labels can come from
// user-controlled YAML (kind names, settings keys/values), so each
// piece is run through sanitizeControl before it's joined into the
// single-line trailer to prevent newlines or ANSI escapes from
// breaking the format or injecting terminal sequences.
func (f *TextFormatter) formatExplanation(w io.Writer, e *lint.Explanation) error {
	if e == nil {
		return nil
	}
	parts := make([]string, 0, len(e.Leaves))
	for _, l := range e.Leaves {
		parts = append(parts, fmt.Sprintf("%s=%s (%s)",
			sanitizeControl(l.Path),
			sanitizeControl(formatLeafValue(l.Value)),
			sanitizeControl(l.Source)))
	}
	body := strings.Join(parts, ", ")
	if body == "" {
		body = "(no settings)"
	}
	rule := sanitizeControl(e.Rule)
	prefix := "  └─ "
	if f.Color {
		_, err := fmt.Fprintf(w, "%s\033[2m%s: %s\033[0m\n", prefix, rule, body)
		return err
	}
	_, err := fmt.Fprintf(w, "%s%s: %s\n", prefix, rule, body)
	return err
}

// formatLeafValue renders a leaf value compactly (JSON-like) so settings
// maps / lists / scalars all print on one line.
func formatLeafValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
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
	safeLine := sanitizeSourceLine(line)
	format := "%*d | %s\n"
	if f.Color && !isDiag {
		format = "\033[2m%*d | %s\033[0m\n"
	}
	_, err := fmt.Fprintf(w, format, gutterWidth, lineNum, safeLine)
	return err
}

// writeCaretLine writes a continuous dot path from column 0 to the caret.
// Source lines use "%*d | %s" so content column C (1-based) starts at
// rune position gutterWidth+3+C-1. Dots fill positions 0..caret-1.
func (f *TextFormatter) writeCaretLine(w io.Writer, gutterWidth, column int) error {
	dots := strings.Repeat("·", gutterWidth+column+2)
	format := "%s^\n"
	if f.Color {
		format = "%s\033[31m^\033[0m\n"
	}
	_, err := fmt.Fprintf(w, format, dots)
	return err
}
