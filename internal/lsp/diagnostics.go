package lsp

import (
	"bytes"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
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
// Both startCol and endCol come from utf16FromByteOffset/utf16Length
// on the same line, which clamps every input to [0, line's UTF-16
// length], so endCol is always >= startCol — no end-before-start
// guard is needed.
func toLSP(d lint.Diagnostic, lines [][]byte) Diagnostic {
	startLine := d.Line - 1
	if startLine < 0 {
		startLine = 0
	}
	line := currentLineBytes(lines, d.Line)
	startCol := utf16FromByteOffset(line, d.Column-1)
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
		Data:     &diagnosticData{RuleName: d.RuleName},
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
// The function operates entirely on []byte (no string round-trip)
// because it is on the diagnostics-publish hot path; allocating a
// full-document string once per publish was a noticeable per-request
// overhead on large files.
func splitLines(source []byte) [][]byte {
	if len(source) == 0 {
		return nil
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
// Equivalent to utf16FromByteOffset(line, len(line)) but spelled out
// for readability at call sites that just want "end of line".
func utf16Length(line []byte) int {
	return utf16FromByteOffset(line, len(line))
}

// utf16FromByteOffset returns the UTF-16 code-unit offset that
// corresponds to UTF-8 byte offset `byteOff` within `line`. The
// result is clamped to [0, utf16Length(line)] so callers cannot
// receive a negative or past-end position even when given a
// malformed mdsmith column. Invalid UTF-8 sequences count as a
// single replacement-character UTF-16 unit each (DecodeRune
// returns (RuneError, 1) for them, and RuneLen(RuneError) == 1),
// so the result stays non-negative on adversarial input.
func utf16FromByteOffset(line []byte, byteOff int) int {
	if byteOff <= 0 {
		return 0
	}
	if byteOff > len(line) {
		byteOff = len(line)
	}
	units := 0
	for i := 0; i < byteOff; {
		r, size := utf8.DecodeRune(line[i:])
		// utf8.DecodeRune always returns size >= 1 when the input is
		// non-empty, and the loop guard `i < byteOff <= len(line)`
		// guarantees a non-empty slice. utf16.RuneLen returns -1 only
		// for surrogate code points (U+D800..U+DFFF), which DecodeRune
		// never yields from valid or invalid UTF-8 — invalid bytes
		// produce RuneError (U+FFFD), whose RuneLen is 1. So both the
		// zero-size and negative-width branches are unreachable from
		// any []byte; we simply add the width.
		units += utf16.RuneLen(r)
		i += size
	}
	return units
}
