package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
)

// asciiClean reports whether s consists solely of printable ASCII
// (plus tab when allowTab is set). Such strings need no sanitizing, so
// the formatters can skip the rune-decoding strings.Map pass — the
// overwhelmingly common case for paths, messages, and source lines.
// Any byte >= 0x80 defers to the slow path so multi-byte sequences,
// C1 controls, and invalid UTF-8 keep strings.Map's exact semantics.
func asciiClean(s string, allowTab bool) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c < 0x7f {
			continue
		}
		if allowTab && c == '\t' {
			continue
		}
		return false
	}
	return true
}

// sanitizeControl strips all C0/C1 control characters from s.
// Used for diagnostic header fields (file path, message) where
// newlines and tabs could break the single-line format.
func sanitizeControl(s string) string {
	if asciiClean(s, false) {
		return s
	}
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
	if asciiClean(s, true) {
		return s
	}
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
	// buf is the scratch each diagnostic's lines are assembled into so
	// the destination sees one Write per diagnostic instead of one per
	// formatted line. Reused across diagnostics; grows to the largest
	// block and stays there for the formatter's lifetime.
	buf []byte
}

// Format writes each diagnostic as a header line followed by an optional
// source snippet with line-number gutter and caret marker.
func (f *TextFormatter) Format(w io.Writer, diagnostics []lint.Diagnostic) error {
	for i := range diagnostics {
		d := diagnostics[i]
		// Normalize the local copy to the user-facing line up front so the
		// header and the snippet caret always agree on which line to mark,
		// even when Line is a non-positive body-anchor sentinel (plan 230).
		// DisplayLine clamps to >= 1; the engine skips snippet context for
		// such diagnostics today, so this keeps the formatter self-consistent
		// for any diagnostic it is handed rather than relying on that guard.
		d.Line = d.DisplayLine()
		f.buf = f.buf[:0]
		f.appendHeader(&d)
		f.appendSnippet(&d)
		f.appendRelated(d.RelatedLocations)
		f.appendExplanation(d.Explanation)
		if _, err := w.Write(f.buf); err != nil {
			return err
		}
	}
	return nil
}

// appendHeader renders the "<file>:<line>:<col> <rule> <message>" line.
func (f *TextFormatter) appendHeader(d *lint.Diagnostic) {
	safeFile := sanitizeControl(d.File)
	safeMsg := sanitizeControl(d.Message)
	if f.Color {
		f.buf = append(f.buf, "\033[36m"...)
		f.appendLocation(safeFile, d.Line, d.Column)
		f.buf = append(f.buf, "\033[0m \033[33m"...)
		f.buf = append(f.buf, d.RuleID...)
		f.buf = append(f.buf, "\033[0m "...)
	} else {
		f.appendLocation(safeFile, d.Line, d.Column)
		f.buf = append(f.buf, ' ')
		f.buf = append(f.buf, d.RuleID...)
		f.buf = append(f.buf, ' ')
	}
	f.buf = append(f.buf, safeMsg...)
	f.buf = append(f.buf, '\n')
}

// appendLocation renders "<file>:<line>:<col>".
func (f *TextFormatter) appendLocation(file string, line, col int) {
	f.buf = append(f.buf, file...)
	f.buf = append(f.buf, ':')
	f.buf = strconv.AppendInt(f.buf, int64(line), 10)
	f.buf = append(f.buf, ':')
	f.buf = strconv.AppendInt(f.buf, int64(col), 10)
}

// appendRelated renders one dimmed trailer line per related location,
// e.g. "  ↳ plan/proto.md:4 — schema requires one of: ...". For an
// MDS020 diagnostic this is where the schema reference surfaces,
// sourced from the structured RelatedLocations rather than the message
// body. File, line, and message can carry user-controlled text (schema
// paths, expected vocabularies), so each piece is sanitized before it
// is joined into the single-line trailer.
func (f *TextFormatter) appendRelated(locs []lint.RelatedLocation) {
	for _, loc := range locs {
		body := relatedLine(loc)
		if body == "" {
			continue
		}
		if f.Color {
			f.buf = append(f.buf, "  ↳ \033[2m"...)
			f.buf = append(f.buf, body...)
			f.buf = append(f.buf, "\033[0m\n"...)
		} else {
			f.buf = append(f.buf, "  ↳ "...)
			f.buf = append(f.buf, body...)
			f.buf = append(f.buf, '\n')
		}
	}
}

// relatedLine renders the "<file>:<line> — <message>" body for one
// related location. The location is omitted when File is empty, the
// ":<line>" is omitted when Line is 0, and the " — " separator appears
// only when both a location and a message are present.
func relatedLine(loc lint.RelatedLocation) string {
	where := sanitizeControl(loc.File)
	if where != "" && loc.Line > 0 {
		where += ":" + strconv.Itoa(loc.Line)
	}
	msg := sanitizeControl(loc.Message)
	switch {
	case where != "" && msg != "":
		return where + " — " + msg
	case where != "":
		return where
	default:
		return msg
	}
}

// appendExplanation renders a one-line trailer naming the rule and
// the winning source of each leaf setting that contributed to the
// rule's effective config. No-op when explanation is nil.
//
// Rule names, leaf paths, leaf values, and source labels can come from
// user-controlled YAML (kind names, settings keys/values), so each
// piece is run through sanitizeControl before it's joined into the
// single-line trailer to prevent newlines or ANSI escapes from
// breaking the format or injecting terminal sequences.
func (f *TextFormatter) appendExplanation(e *lint.Explanation) {
	if e == nil {
		return
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
	f.buf = append(f.buf, "  └─ "...)
	if f.Color {
		f.buf = append(f.buf, "\033[2m"...)
		f.buf = append(f.buf, rule...)
		f.buf = append(f.buf, ": "...)
		f.buf = append(f.buf, body...)
		f.buf = append(f.buf, "\033[0m\n"...)
		return
	}
	f.buf = append(f.buf, rule...)
	f.buf = append(f.buf, ": "...)
	f.buf = append(f.buf, body...)
	f.buf = append(f.buf, '\n')
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

// appendSnippet renders the source context lines with a line-number gutter
// and a dot-leader caret line under the diagnostic line. The dots always
// start at column 1, creating a visual path that identifies which line
// is the diagnostic and (for Column>1) guides the eye to the exact column.
func (f *TextFormatter) appendSnippet(d *lint.Diagnostic) {
	if len(d.SourceLines) == 0 {
		return
	}

	maxLineNum := d.SourceStartLine + len(d.SourceLines) - 1
	gutterWidth := len(strconv.Itoa(maxLineNum))

	for i, line := range d.SourceLines {
		lineNum := d.SourceStartLine + i
		isDiagLine := lineNum == d.Line

		f.appendSourceLine(gutterWidth, lineNum, line, isDiagLine)

		if isDiagLine && d.Column > 0 {
			f.appendCaretLine(gutterWidth, d.Column)
		}
	}
}

// appendSourceLine renders a single source line with line-number gutter.
// Context lines are dimmed when color is on.
func (f *TextFormatter) appendSourceLine(gutterWidth, lineNum int, line string, isDiag bool) {
	safeLine := sanitizeSourceLine(line)
	dim := f.Color && !isDiag
	if dim {
		f.buf = append(f.buf, "\033[2m"...)
	}
	f.appendGutterNum(gutterWidth, lineNum)
	f.buf = append(f.buf, " | "...)
	f.buf = append(f.buf, safeLine...)
	if dim {
		f.buf = append(f.buf, "\033[0m"...)
	}
	f.buf = append(f.buf, '\n')
}

// appendGutterNum renders lineNum right-aligned in a gutterWidth-wide
// space-padded field, matching fmt's "%*d".
func (f *TextFormatter) appendGutterNum(gutterWidth, lineNum int) {
	var tmp [20]byte
	num := strconv.AppendInt(tmp[:0], int64(lineNum), 10)
	for pad := gutterWidth - len(num); pad > 0; pad-- {
		f.buf = append(f.buf, ' ')
	}
	f.buf = append(f.buf, num...)
}

// caretDots is a pre-rendered run of the U+00B7 dot used by the caret
// line, appended in chunks so no per-caret strings.Repeat allocation
// is needed. 64 dots cover a typical caret; longer columns loop.
const caretDots = "································································"

// caretDotCount is how many dots caretDots holds (each is 2 bytes).
const caretDotCount = len(caretDots) / 2

// appendCaretLine renders a continuous dot path from column 0 to the caret.
// Source lines use "%*d | %s" so content column C (1-based) starts at
// rune position gutterWidth+3+C-1. Dots fill positions 0..caret-1.
func (f *TextFormatter) appendCaretLine(gutterWidth, column int) {
	for n := gutterWidth + column + 2; n > 0; n -= caretDotCount {
		if n >= caretDotCount {
			f.buf = append(f.buf, caretDots...)
			continue
		}
		f.buf = append(f.buf, caretDots[:2*n]...)
	}
	if f.Color {
		f.buf = append(f.buf, "\033[31m^\033[0m\n"...)
		return
	}
	f.buf = append(f.buf, "^\n"...)
}
