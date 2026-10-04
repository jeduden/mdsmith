// Package mdfence answers one question about a single Markdown source
// line: does it open, or close, a CommonMark fenced code block?
//
// It is the one shared copy of the fence rules that line scanners
// across mdsmith (the Layer 0 scanner, the line classifier, rule-local
// scanners, the website link rewriter) used to re-implement by hand.
// The rules mirror goldmark's fencedCodeBlockParser in
// pkg/goldmark/parser/fcode_block.go:
//
//   - An opener is up to three columns of indentation, then a run of
//     three or more identical backticks or tildes. A backtick fence's
//     info string may not hold a backtick; such a line is paragraph
//     text (an inline code span).
//   - A closer is up to three columns of indentation, then a run of the
//     opener's character at least as long as the opener, then only
//     whitespace. The closer's indentation is independent of the
//     opener's.
//   - Indentation is measured in columns, as goldmark does: a space
//     takes one column and a tab advances to the next multiple of four
//     (Indent). Four or more columns is indented code, never a fence —
//     and indented code cannot interrupt a paragraph, so such a line
//     after paragraph text is continuation text, again no fence. Every
//     line scanner in mdsmith uses this one policy.
//   - Whitespace is goldmark's util.IsSpace set: space, tab, CR and
//     LF. A trailing "\r" from a CRLF line therefore never counts as
//     info text nor blocks a close.
//   - On the source's final line, when it ends without a newline,
//     goldmark ignores a one-byte info string; OpenFinal mirrors that.
//
// The helpers work on one line's bytes, never allocate, never write
// to the line, and never retain it; a Tracker holds only scalars. They
// know nothing about containers: Open and Close read a line from column
// zero, and OpenIn and CloseIn take the column the line starts at and
// the content column of the container the caller tracks, so tabs expand
// at their true columns and the indent budget counts from the
// container.
package mdfence

import "bytes"

// Fence describes an opening fence line. Fields are ordered
// large-to-small (the int before the two single-byte fields) so the
// struct is 16 bytes; callers return it by value on per-line scan paths
// (docs/development/high-performance-go.md "Struct layout").
type Fence struct {
	// Len is the length of the fence run (3 or more).
	Len int
	// Char is the fence character: '`' or '~'. It is 0 in the zero
	// Fence.
	Char byte
	// HasInfo reports whether a non-empty info string follows the run.
	// goldmark attaches no source position to a fence with neither info
	// nor content, which some scanners mirror.
	HasInfo bool
}

// Open reports whether line opens a fenced code block and, if so,
// returns the fence. See the package comment for the rules.
func Open(line []byte) (Fence, bool) {
	return open(line, 0, 0, false)
}

// OpenFinal is Open with one goldmark quirk mirrored: final reports
// that line is the source's last line and ends without a newline.
// goldmark reads an info string only when at least two bytes follow
// the run on the line it sees, newline included, so on such a final
// line a single byte after the run ("```x", "```\v") is dropped and
// the fence has no info. Whether the line opens a fence is unchanged:
// that one byte can never be a backtick after a backtick run.
func OpenFinal(line []byte, final bool) (Fence, bool) {
	return open(line, 0, 0, final)
}

// OpenIn is Open for a line that starts at column col inside a
// container whose content starts at column base: the fence run may sit
// at most three columns past base. A line indented less than base (a
// line the caller kept in the container lazily, or one at the top
// level) counts as indent zero.
func OpenIn(line []byte, col, base int) (Fence, bool) {
	return open(line, col, base, false)
}

// open is the shared opener check behind Open, OpenFinal, and OpenIn.
func open(line []byte, col, base int, final bool) (Fence, bool) {
	w, i := Indent(line, col)
	if col+w-base > 3 || i >= len(line) {
		return Fence{}, false
	}
	ch := line[i]
	if ch != '`' && ch != '~' {
		return Fence{}, false
	}
	j := i + 1
	for j < len(line) && line[j] == ch {
		j++
	}
	if j-i < 3 {
		return Fence{}, false
	}
	rest := line[j:]
	if ch == '`' && bytes.IndexByte(rest, '`') >= 0 {
		return Fence{}, false
	}
	hasInfo := false
	for _, c := range rest {
		if !isSpace(c) {
			hasInfo = true
			break
		}
	}
	if final && len(rest) == 1 {
		hasInfo = false
	}
	return Fence{Len: j - i, Char: ch, HasInfo: hasInfo}, true
}

// Close reports whether line closes the fenced code block f opened.
// f must come from Open; the zero Fence is never open.
func Close(line []byte, f Fence) bool {
	return CloseIn(line, f, 0, 0)
}

// CloseIn is Close for a line that starts at column col inside a
// container whose content starts at column base, measured as OpenIn
// measures an opener.
func CloseIn(line []byte, f Fence, col, base int) bool {
	if f.Char == 0 {
		return false
	}
	w, i := Indent(line, col)
	if col+w-base > 3 {
		return false
	}
	j := i
	for j < len(line) && line[j] == f.Char {
		j++
	}
	if j-i < f.Len {
		return false
	}
	for _, c := range line[j:] {
		if !isSpace(c) {
			return false
		}
	}
	return true
}

// Indent returns the width in columns of line's leading spaces and
// tabs and the number of bytes they span, when line[0] sits at column
// col. A space takes one column; a tab advances to the next multiple of
// four, as goldmark's util.IndentWidth counts.
func Indent(line []byte, col int) (width, n int) {
	c := col
	for ; n < len(line); n++ {
		switch line[n] {
		case ' ':
			c++
		case '\t':
			c += 4 - c%4
		default:
			return c - col, n
		}
	}
	return c - col, n
}

// Tracker follows fenced-code-block state across a line-by-line scan.
// The zero Tracker is outside any fence.
type Tracker struct {
	open Fence
}

// Step advances the tracker past line and reports whether line belongs
// to a fenced code block: its opener, a content line, or its closer.
// An unclosed fence runs to the end of the scan.
func (t *Tracker) Step(line []byte) bool {
	if t.open.Char != 0 {
		if Close(line, t.open) {
			t.open = Fence{}
		}
		return true
	}
	f, ok := Open(line)
	if !ok {
		return false
	}
	t.open = f
	return true
}

// isSpace reports whether c is whitespace in goldmark's util.IsSpace
// sense: space, tab, LF, or CR.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
