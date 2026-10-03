package fencepos

import "github.com/jeduden/mdsmith/pkg/goldmark/ast"

// OpenRun returns the opening fence run of fcb: its start offset in
// src, its length, and its character (backtick or tilde). n is 0 and
// ch 0 when no fence run can be read. The run is the first thing on
// the opening line past the container prefix — indent, block-quote
// markers, list markers — none of which is a backtick or tilde, so
// a fence behind "- " or "> " is read at the marker's end, not at the
// line start. The node position only picks the line: goldmark counts
// a tab's padding in columns, so with tabs it can land inside the run.
func OpenRun(src []byte, fcb *ast.FencedCodeBlock) (start, n int, ch byte) {
	lineStart, lineEnd := OpenLineRange(src, fcb)
	start = lineStart
	for start < lineEnd && isContainerPrefixByte(src[start]) {
		start++
	}
	if start >= lineEnd || !isFenceChar(src[start]) {
		return 0, 0, 0
	}
	ch = src[start]
	return start, runLen(src, start, ch), ch
}

// isContainerPrefixByte reports whether b can precede a fence run on
// its opening line: indent, a block-quote marker, or a bullet or
// ordered list marker.
func isContainerPrefixByte(b byte) bool {
	switch b {
	case ' ', '\t', '>', '-', '+', '*', '.', ')':
		return true
	}
	return b >= '0' && b <= '9'
}

// CloseRun returns the closing fence run of fcb: its start offset in
// src and its length, with ok false when the block has no closing
// fence. The candidate is the line after the block's last content
// line (CloseLineRange). It is read through the block's containers,
// outermost first, the way the parser continues them — a block quote
// takes up to three spaces, ">" and one optional space; a list item
// takes its content offset — and the rest must be a CommonMark closer:
// at most three spaces, a run of at least the opener's length of the
// opener's character, then only whitespace. A line that ends a
// container (the block quote stops, the list item is dedented)
// belongs to another block and so never closes this one.
func CloseRun(src []byte, fcb *ast.FencedCodeBlock) (start, n int, ok bool) {
	_, openN, ch := OpenRun(src, fcb)
	if openN == 0 {
		return 0, 0, false
	}
	_, openEnd := OpenLineRange(src, fcb)
	lineStart, lineEnd := CloseLineRange(src, fcb, openEnd)
	if lineStart >= len(src) {
		return 0, 0, false
	}
	c := columnCursor{src: src[:lineEnd], i: lineStart}
	if !c.enterContainers(fcb.Parent()) {
		return 0, 0, false
	}
	if w := c.blankCols(); w >= 4 {
		return 0, 0, false
	} else {
		c.advance(w)
	}
	if c.pend != 0 || c.i >= len(c.src) || c.src[c.i] != ch {
		return 0, 0, false
	}
	start = c.i
	n = runLen(c.src, start, ch)
	if n < openN || !isBlankTail(c.src[start+n:]) {
		return 0, 0, false
	}
	return start, n, true
}

// columnCursor walks the leading whitespace and container markers of
// one line in tab-expanded columns (tab stops every four columns).
// pend counts columns of a tab already stepped over but not yet
// consumed, as when a block-quote marker's optional space is taken
// from a tab.
type columnCursor struct {
	src    []byte
	i, col int
	pend   int
}

// enterContainers consumes the container prefixes of the line for
// node's block-quote and list-item ancestors, outermost first. It
// reports false when the line does not continue one of them.
func (c *columnCursor) enterContainers(node ast.Node) bool {
	if node == nil {
		return true
	}
	if !c.enterContainers(node.Parent()) {
		return false
	}
	switch n := node.(type) {
	case *ast.Blockquote:
		return c.enterBlockquote()
	case *ast.ListItem:
		w := c.blankCols()
		if w < n.Offset && w < 4 {
			return false
		}
		c.advance(min(w, n.Offset))
	}
	return true
}

// enterBlockquote consumes one block-quote marker: up to three
// columns of indent, ">", and one optional column of whitespace.
func (c *columnCursor) enterBlockquote() bool {
	w := c.blankCols()
	if w > 3 {
		return false
	}
	c.advance(w)
	if c.pend != 0 || c.i >= len(c.src) || c.src[c.i] != '>' {
		return false
	}
	c.i++
	c.col++
	if c.blankCols() > 0 {
		c.advance(1)
	}
	return true
}

// blankCols returns the columns of whitespace available at the cursor.
func (c *columnCursor) blankCols() int {
	w, col := c.pend, c.col+c.pend
	for j := c.i; j < len(c.src); j++ {
		switch c.src[j] {
		case ' ':
			w++
			col++
		case '\t':
			step := 4 - col%4
			w += step
			col += step
		default:
			return w
		}
	}
	return w
}

// advance consumes n columns of whitespace; the caller checked with
// blankCols that they exist.
func (c *columnCursor) advance(n int) {
	for n > 0 {
		if c.pend > 0 {
			c.pend--
			c.col++
			n--
			continue
		}
		step := 1
		if c.src[c.i] == '\t' {
			step = 4 - c.col%4
		}
		c.i++
		c.pend = step
	}
}

// runLen counts the bytes equal to ch starting at p.
func runLen(src []byte, p int, ch byte) int {
	n := 0
	for p+n < len(src) && src[p+n] == ch {
		n++
	}
	return n
}

func isFenceChar(b byte) bool { return b == '`' || b == '~' }

// isBlankTail reports whether b holds only spaces, tabs, and a "\r".
func isBlankTail(b []byte) bool {
	for _, x := range b {
		if x != ' ' && x != '\t' && x != '\r' {
			return false
		}
	}
	return true
}
