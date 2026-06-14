package lint

import "bytes"

// leadingSpaces counts the run of ASCII space characters at the start of
// b. A leading tab stops the count (it is handled as indentation worth a
// full tab stop by the ≥4 indented-code test, never as a ≤3 fence indent).
func leadingSpaces(b []byte) int {
	n := 0
	for n < len(b) && b[n] == ' ' {
		n++
	}
	return n
}

// indentColumns counts the column width of the leading whitespace run of
// b, treating a tab as an advance to the next four-column tab stop (the
// CommonMark indentation rule). It stops at the first non-whitespace byte.
// A line whose leading whitespace is four or more columns can open an
// indented code block; ≤3 columns is the fence/heading/HTML start gate.
func indentColumns(b []byte) int {
	col := 0
	for _, c := range b {
		switch c {
		case ' ':
			col++
		case '\t':
			col += 4 - (col % 4)
		default:
			return col
		}
	}
	return col
}

// isBlankBytes reports whether b is empty or only spaces and tabs.
func isBlankBytes(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}

// isBlankFrom reports whether line is blank from pos onward.
func isBlankFrom(line []byte, pos int) bool {
	for i := pos; i < len(line); i++ {
		if line[i] != ' ' && line[i] != '\t' {
			return false
		}
	}
	return true
}

// isATXHeading reports whether rest (already at ≤3 indent) is an ATX
// heading: 1–6 `#` followed by a space, a tab, or end of line. Mirrors
// the CommonMark ATX rule the goldmark heading parser applies.
func isATXHeading(rest []byte) bool {
	i := leadingSpaces(rest)
	n := 0
	for i < len(rest) && rest[i] == '#' {
		i++
		n++
	}
	if n < 1 || n > 6 {
		return false
	}
	return i >= len(rest) || rest[i] == ' ' || rest[i] == '\t'
}

// isSetextUnderline reports whether rest is a setext underline: a run of
// only `=` or only `-` (after ≤3 indent), optionally trailed by spaces.
// The caller gates this on the previous line being paragraph text.
func isSetextUnderline(rest []byte) bool {
	i := leadingSpaces(rest)
	if i >= len(rest) {
		return false
	}
	c := rest[i]
	if c != '=' && c != '-' {
		return false
	}
	for i < len(rest) && rest[i] == c {
		i++
	}
	for ; i < len(rest); i++ {
		if rest[i] != ' ' && rest[i] != '\t' {
			return false
		}
	}
	return true
}

// detectFenceOpen reports whether rest opens a fenced code block, and if
// so returns the fence character, its length, and whether a non-empty
// info string follows. For backtick fences the info string may not
// contain a backtick (the CommonMark rule that lets inline code spans
// coexist with fences); tilde fences allow any info string.
func detectFenceOpen(rest []byte) (ch byte, length int, hasInfo, ok bool) {
	i := leadingSpaces(rest)
	if i > 3 || i >= len(rest) {
		return 0, 0, false, false
	}
	c := rest[i]
	if c != '`' && c != '~' {
		return 0, 0, false, false
	}
	n := 0
	for i < len(rest) && rest[i] == c {
		i++
		n++
	}
	if n < 3 {
		return 0, 0, false, false
	}
	info := rest[i:]
	if c == '`' && bytes.IndexByte(info, '`') >= 0 {
		return 0, 0, false, false
	}
	return c, n, len(bytes.TrimSpace(info)) > 0, true
}

// isFenceClose reports whether rest closes a fence of the given character
// and minimum length: a run of at least length fence characters (after
// ≤3 indent) followed only by spaces or tabs.
func isFenceClose(rest []byte, ch byte, length int) bool {
	i := leadingSpaces(rest)
	if i > 3 {
		return false
	}
	n := 0
	for i < len(rest) && rest[i] == ch {
		i++
		n++
	}
	if n < length {
		return false
	}
	for ; i < len(rest); i++ {
		if rest[i] != ' ' && rest[i] != '\t' {
			return false
		}
	}
	return true
}

// htmlBlockEnd reports the closing string for a marker-terminated
// CommonMark HTML block (comment, CDATA section, processing instruction,
// or declaration) beginning at rest after ≤3 indent, and whether rest
// opens one. These are the HTML blocks whose interior may hold a blank
// line followed by indented text; a fence-only scanner would misread that
// text as an indented code block, so the classifier tracks them to keep
// those lines out of the code set.
//
// The tag blocks (types 1, 6, 7 — script/pre/style and block-level
// elements) are deliberately not tracked. They are terminated by a blank
// line and their non-blank interior keeps the paragraph-continuation flag
// set, which already prevents an indented interior line from being read as
// code — so tracking them buys no code-set accuracy and the loose closing
// match it would need (a bare `</`) risks ending a block early on a nested
// inline tag.
func htmlBlockEnd(rest []byte) (end []byte, ok bool) {
	i := leadingSpaces(rest)
	if i > 3 || i >= len(rest) || rest[i] != '<' {
		return nil, false
	}
	s := rest[i:]
	switch {
	case bytes.HasPrefix(s, htmlOpenComment):
		return htmlCloseComment, true
	case bytes.HasPrefix(s, htmlOpenCDATA):
		return htmlCloseCDATA, true
	case bytes.HasPrefix(s, htmlOpenPI):
		return htmlClosePI, true
	case len(s) >= 3 && s[1] == '!' && isASCIILetterByte(s[2]):
		return htmlCloseDecl, true
	}
	return nil, false
}

var (
	htmlOpenComment  = []byte("<!--")
	htmlOpenCDATA    = []byte("<![CDATA[")
	htmlOpenPI       = []byte("<?")
	htmlCloseComment = []byte("-->")
	htmlCloseCDATA   = []byte("]]>")
	htmlClosePI      = []byte("?>")
	htmlCloseDecl    = []byte(">")
)

// isASCIILetterByte reports whether b is an ASCII letter.
func isASCIILetterByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// blockquoteMarker returns the byte width of a blockquote opener at the
// start of rest (`[ ]{0,3}>` plus one optional following space), or 0
// when rest does not open a blockquote.
func blockquoteMarker(rest []byte) int {
	i := leadingSpaces(rest)
	if i > 3 || i >= len(rest) || rest[i] != '>' {
		return 0
	}
	i++
	if i < len(rest) && rest[i] == ' ' {
		i++
	}
	return i
}

// listMarkerWidth returns the content width of a list-item marker at the
// start of rest — the byte count from rest[0] to the first content
// character — or 0 when rest does not open a list item. It handles bullet
// markers (`-`, `+`, `*`) and ordered markers (digits then `.` or `)`),
// each followed by at least one space or tab. A marker with no following
// space (or one whose content would be empty) is not a list start.
func listMarkerWidth(rest []byte) int {
	i := leadingSpaces(rest)
	if i > 3 || i >= len(rest) {
		return 0
	}
	switch rest[i] {
	case '-', '+', '*':
		i++
	default:
		start := i
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		if i == start || i >= len(rest) || (rest[i] != '.' && rest[i] != ')') {
			return 0
		}
		i++
	}
	// At least one space/tab must follow the marker, and some content
	// must follow it (a bare `-` on its own line is a thematic break or
	// empty list item, not a content-bearing marker this pass tracks).
	if i >= len(rest) || (rest[i] != ' ' && rest[i] != '\t') {
		return 0
	}
	for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t') {
		i++
	}
	if i >= len(rest) {
		return 0
	}
	return i
}
