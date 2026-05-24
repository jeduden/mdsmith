package lsp

import (
	"bytes"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
)

// toLSP converts an mdsmith diagnostic to the LSP wire shape.
//
// Coordinates flip from 1-based (mdsmith) to 0-based (LSP). The end
// column is set to the line's UTF-16 length so the squiggle covers
// the remainder of the line; rules can later widen this with their
// own per-rule end column once LSP-aware spans land in the engine.
//
// LSP positions count UTF-16 code units. mdsmith's
// `lint.Diagnostic.Column` is a 1-based UTF-8 byte column (see
// `lint.File.ColumnOfOffset`, which derives column from byte offsets
// inside the source). We convert by walking the line's bytes,
// summing the UTF-16 width of every rune we cross until we reach
// the byte offset that maps to `Column-1`. This matters for any
// document containing non-ASCII text — treating Column as a rune
// offset would misplace the squiggle by N-1 positions for every
// preceding rune that takes N>1 bytes.
//
// Both startCol and endCol come from mdtext.UTF16FromByteOffset/utf16Length
// on the same line, which clamps every input to [0, line's UTF-16
// length], so endCol is always >= startCol — no end-before-start
// guard is needed.
func toLSP(d lint.Diagnostic, lines [][]byte) Diagnostic {
	startLine := d.Line - 1
	if startLine < 0 {
		startLine = 0
	}
	line := currentLineBytes(lines, d.Line)
	startCol := mdtext.UTF16FromByteOffset(line, d.Column-1)
	endCol := utf16Length(line)
	return Diagnostic{
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: endCol},
		},
		Severity: severityFor(d.Severity),
		Code:     d.RuleID,
		Source:   "mdsmith",
		Message:  d.Message,
		Data: &diagnosticData{
			RuleName:   d.RuleName,
			Deprecated: d.Deprecated,
			ReplacedBy: d.ReplacedBy,
		},
	}
}

// toLSPAll maps a slice. Returns an empty (non-nil) slice for empty
// input so the JSON wire form is `[]`, never `null`.
func toLSPAll(diags []lint.Diagnostic, source []byte) []Diagnostic {
	out := make([]Diagnostic, 0, len(diags))
	lines := splitLines(source)
	for _, d := range diags {
		out = append(out, toLSP(d, lines))
	}
	return out
}

func severityFor(s lint.Severity) DiagnosticSeverity {
	if s == lint.Warning {
		return severityWarning
	}
	return severityError
}

// splitLines splits source into per-line byte slices, preserving
// trailing empty lines so the indexing matches lint.File.Lines (which
// also uses bytes.Split). Rules such as single-trailing-newline emit
// diagnostics anchored at len(f.Lines) for trailing whitespace runs;
// trimming the trailing newlines here would make currentLine() return
// "" and toLSP would clamp to a position past the document. Each
// line has its trailing CR stripped so Windows-style line endings
// produce matching positions on the wire.
//
// Empty input returns a 1-element slice containing an empty line,
// matching what bytes.Split produces (and therefore lint.File.Lines).
// Returning nil here would put a diagnostic anchored at line 1 of an
// empty buffer past the line count splitLines reports — currentLineBytes
// would treat 1-based line 1 as out of range and toLSP would silently
// clamp the squiggle to the wrong place.
//
// The function operates entirely on []byte (no string round-trip)
// because it is on the diagnostics-publish hot path; allocating a
// full-document string once per publish was a noticeable per-request
// overhead on large files.
func splitLines(source []byte) [][]byte {
	if len(source) == 0 {
		// Match bytes.Split's "empty input → 1-element empty
		// slice" contract so lint.File.Lines and splitLines
		// agree on the line count of an empty buffer.
		return [][]byte{nil}
	}
	parts := bytes.Split(source, []byte{'\n'})
	for i, p := range parts {
		if n := len(p); n > 0 && p[n-1] == '\r' {
			parts[i] = p[:n-1]
		}
	}
	return parts
}

// currentLineBytes returns the content of 1-based line number n as a
// byte slice, or nil when out of range. The byte form lets the
// callers (toLSP, utf16Length) avoid an extra string conversion on
// the hot path.
func currentLineBytes(lines [][]byte, n int) []byte {
	if n < 1 || n > len(lines) {
		return nil
	}
	return lines[n-1]
}

// utf16Length returns the total UTF-16 code-unit length of line.
// Equivalent to mdtext.UTF16FromByteOffset(line, len(line)) but
// spelled out for readability at call sites that just want "end of
// line".
func utf16Length(line []byte) int {
	return mdtext.UTF16FromByteOffset(line, len(line))
}

// byteOffsetFromUTF16 maps a UTF-16 column position (LSP wire form)
// back to the matching UTF-8 byte offset within line. The result is
// clamped to [0, len(line)] so a malformed or past-end LSP position
// stays within the slice.
//
// This is the inverse of mdtext.UTF16FromByteOffset. The navigation
// surface uses it to convert `Position.Character` (UTF-16) to a byte
// column before calling the Locator, which works in 1-based byte
// columns. Without it, every cursor on a non-ASCII line would
// mis-locate by the number of multi-byte runes between byte 0 and
// the cursor. Distinct from mdtext.UTF16ToByteOffset: when utf16Off
// lands inside a surrogate pair, this rounds down to the rune's
// starting byte (LSP cursor semantics), whereas UTF16ToByteOffset
// rounds up to the next codepoint boundary.
func byteOffsetFromUTF16(line []byte, utf16Off int) int {
	if utf16Off <= 0 {
		return 0
	}
	units := 0
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRune(line[i:])
		w := mdtext.NonNegativeUTF16RuneLen(r)
		if units+w > utf16Off {
			return i
		}
		units += w
		i += size
		if units == utf16Off {
			return i
		}
	}
	return len(line)
}
