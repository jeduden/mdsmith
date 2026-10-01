// Package mdfence answers one question about a single Markdown source
// line: does it open, or close, a CommonMark fenced code block?
//
// It is the one shared copy of the fence rules that line scanners
// across mdsmith (the Layer 0 scanner, the line classifier, rule-local
// scanners, the website link rewriter) used to re-implement by hand.
// The rules mirror goldmark's fencedCodeBlockParser in
// pkg/goldmark/parser/fcode_block.go:
//
//   - An opener is up to three spaces of indentation, then a run of
//     three or more identical backticks or tildes. A tab in the
//     indentation reaches column four (indented code), so it never
//     precedes a fence. A backtick fence's info string may not hold a
//     backtick; such a line is paragraph text (an inline code span).
//   - A closer is up to three spaces of indentation, then a run of the
//     opener's character at least as long as the opener, then only
//     whitespace. The closer's indentation is independent of the
//     opener's.
//   - Whitespace is goldmark's util.IsSpace set: space, tab, CR and
//     LF. A trailing "\r" from a CRLF line therefore never counts as
//     info text nor blocks a close.
//
// The helpers work on one line's bytes, never allocate, and know
// nothing about containers. A caller scanning inside a list item or
// block quote passes the line with the container prefix already
// stripped (or sliced at the container's content column), so
// indentation is measured relative to that container.
package mdfence

// Fence describes an opening fence line. Fields are ordered
// large-to-small (both ints before the two single-byte fields) so the
// struct is 24 bytes with no padding; callers return it by value on
// per-line scan paths (docs/development/high-performance-go.md
// "Struct layout").
type Fence struct {
	// Indent is the number of leading spaces before the fence run
	// (0 to 3).
	Indent int
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
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 || indent >= len(line) {
		return Fence{}, false
	}
	ch := line[indent]
	if ch != '`' && ch != '~' {
		return Fence{}, false
	}
	j := indent + 1
	for j < len(line) && line[j] == ch {
		j++
	}
	if j-indent < 3 {
		return Fence{}, false
	}
	hasInfo := false
	for _, c := range line[j:] {
		if c == '`' && ch == '`' {
			return Fence{}, false
		}
		if !isSpace(c) {
			hasInfo = true
		}
	}
	return Fence{Indent: indent, Len: j - indent, Char: ch, HasInfo: hasInfo}, true
}

// Close reports whether line closes the fenced code block f opened.
// f must come from Open; the zero Fence is never open.
func Close(line []byte, f Fence) bool {
	if f.Char == 0 {
		return false
	}
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return false
	}
	j := indent
	for j < len(line) && line[j] == f.Char {
		j++
	}
	if j-indent < f.Len {
		return false
	}
	for _, c := range line[j:] {
		if !isSpace(c) {
			return false
		}
	}
	return true
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

// InFence reports whether a fence opened by an earlier Step is still
// open.
func (t *Tracker) InFence() bool { return t.open.Char != 0 }

// isSpace reports whether c is whitespace in goldmark's util.IsSpace
// sense: space, tab, LF, or CR.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
