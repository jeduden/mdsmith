package include

import (
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdfence"
)

// The include rule rewrites included content as strings, line by line,
// and tracks no list or block-quote containers. Its fence checks follow
// mdfence's one indentation policy: an opener or closer sits at most
// three columns in, a tab reaches the next multiple of four, and a
// fence-like line four or more columns in is indented code or paragraph
// continuation text, never a fence.
//
// One container case is read on purpose: a fence opened on a list
// marker line ("- ```"). Its body lines sit at the item's content
// column, where a heading-like line ("  # comment") must stay code. The
// fence then closes relative to that column, or ends with the item when
// a non-blank line falls left of it.
//
// Lines reach mdfence as byte views of the strings
// (util.StringToReadOnlyBytes) without copying: mdfence only reads a
// line and never retains it, and the scan keeps only scalars
// (mdfence's TestNoRetention pins both).

// fenceScan follows fenced-code-block state across the include rule's
// line scans. The zero fenceScan is outside any fence. It holds only
// scalars.
type fenceScan struct {
	// fence is the open fenced code block; the zero Fence when none.
	fence mdfence.Fence
	// base is the content column of the list item whose marker line
	// opened fence, or 0 for a fence opened at the top level.
	base int
}

// step advances the scan past b and reports whether b belongs to a
// fenced code block (opener, content, or closer). rootPara reports that
// a document-level paragraph is open, which a list marker line must be
// able to interrupt before it can open a fence. A paragraph inside a
// list item or block quote binds no marker line: goldmark applies the
// interrupt rule only to a paragraph in the parent the new list opens
// in, so "2. ```" after "1. a" starts a sibling item and its fence.
func (s *fenceScan) step(b []byte, rootPara bool) bool {
	if s.fence.Char != 0 {
		if !s.leavesItem(b) {
			if mdfence.CloseIn(b, s.fence, 0, s.base) {
				s.fence = mdfence.Fence{}
			}
			return true
		}
		// The line ends the list item, and the item's fence with it.
		s.fence = mdfence.Fence{}
	}
	if f, ok := mdfence.Open(b); ok {
		s.fence, s.base = f, 0
		return true
	}
	if rootPara && !lint.StartsInterruptingBlock(b) {
		return false
	}
	if col, off, ok := listContent(b); ok {
		if f, ok := mdfence.Open(b[off:]); ok {
			s.fence, s.base = f, col
			return true
		}
	}
	return false
}

// leavesItem reports whether b, read inside a fence a list marker line
// opened, is a non-blank line left of the item's content column: the
// item does not continue there, so neither does its fence.
func (s *fenceScan) leavesItem(b []byte) bool {
	if s.base == 0 {
		return false
	}
	w, n := mdfence.Indent(b, 0)
	return w < s.base && n < len(b) && b[n] != '\r' && b[n] != '\n'
}

// listContent reports whether b opens a list item with content on the
// marker line: up to three spaces, a bullet ('-', '+', '*') or an
// ordered marker (one to nine digits and '.' or ')'), then one to four
// columns of spaces or tabs before the content. It returns the
// content's column and byte offset. Five or more columns make the
// content indented code, which opens no fence, so ok is false.
func listContent(b []byte) (col, off int, ok bool) {
	i := 0
	for i < len(b) && b[i] == ' ' {
		i++
	}
	if i > 3 || i >= len(b) {
		return 0, 0, false
	}
	switch c := b[i]; {
	case c == '-' || c == '+' || c == '*':
		i++
	case c >= '0' && c <= '9':
		d := i
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
		if i-d > 9 || i >= len(b) || b[i] != '.' && b[i] != ')' {
			return 0, 0, false
		}
		i++
	default:
		return 0, 0, false
	}
	w, n := mdfence.Indent(b[i:], i)
	if w == 0 || w > 4 || i+n >= len(b) || b[i+n] == '\r' || b[i+n] == '\n' {
		return 0, 0, false
	}
	return i + w, i + n, true
}
