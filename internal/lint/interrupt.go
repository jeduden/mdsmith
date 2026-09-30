package lint

import (
	"bytes"
	"regexp"
)

// InterruptsParagraph reports whether line, placed directly after a line
// of an open paragraph, would stop continuing that paragraph. It is true
// for a blank line, for a setext underline (which turns the paragraph
// into a heading), and for the start of every block that CommonMark lets
// interrupt a paragraph:
//
//   - an ATX heading: 1-6 '#' then a space, a tab, or the end of the line
//   - a thematic break
//   - a fenced code block, with or without an info string
//   - a block quote
//   - a bullet list item that has content
//   - an ordered list item that has content and starts at 1
//   - an HTML block of types 1-6, which covers mdsmith's `<?name`
//     processing instructions
//
// Everything else continues the paragraph, including indented code, HTML
// type 7, an empty list item, and an ordered item that starts at another
// number. Up to three spaces of indent are allowed, as CommonMark allows.
//
// The block detectors are the Layer 0 scanner's. For HTML they follow
// the spec, which is broader than the goldmark fork in two places: a
// lowercase letter after "<!" (type 4) and a tab after a block tag name
// (type 6) open an HTML block in the spec but not in the fork. The fork
// is broader in one place: it allows spaces between "</" and a block
// tag name. Layer 0 stays strict there, so forkSpacedCloseTag adds that
// case here. The answer is true when either reading sees a block start,
// so it holds for mdsmith's parser and for other CommonMark renderers.
func InterruptsParagraph(line []byte) bool {
	if isBlankLine(line) || isSetextUnderline(line) || isThematicBreak(line) || isATXHeadingLine(line) {
		return true
	}
	if _, ok := openingFence(line); ok {
		return true
	}
	if openHTMLBlock(line, true) != htmlNone || forkSpacedCloseTag(line) {
		return true
	}
	indent := leadingSpaces(line)
	if indent >= 4 {
		return false
	}
	// line is not blank, so a non-space byte sits at indent.
	switch line[indent] {
	case '>':
		return true
	case '-', '+', '*':
		return isBulletMarker(line, indent) && !isBlankLine(line[indent+1:])
	}
	return interruptingOrderedMarker(line, indent)
}

// interruptingOrderedMarker reports whether line opens an ordered list
// item that can interrupt a paragraph: the item has content, and its
// start number is 1. goldmark reads the number with strconv.Atoi, so
// leading zeros ("01.") still count as 1.
func interruptingOrderedMarker(line []byte, indent int) bool {
	if !isOrderedMarker(line, indent) {
		return false
	}
	j := indent
	for line[j] == '0' {
		j++
	}
	// isOrderedMarker found a '.' or ')' after the digits, so j+1 is in
	// range whenever line[j] is a digit.
	if line[j] != '1' || (line[j+1] != '.' && line[j+1] != ')') {
		return false
	}
	return !isBlankLine(line[j+2:])
}

// spacedCloseTag matches a close tag with spaces between "</" and the
// tag name, then a space, ">", "/>", or the line end. It is the part of
// the fork's type-6 HTML block pattern that the spec does not have.
var spacedCloseTag = regexp.MustCompile(`^[ ]{0,3}</[ ]+([a-zA-Z][a-zA-Z0-9-]*)(?:[ ]|>|/>|$)`)

// forkSpacedCloseTag reports whether line is a close tag of a type-6
// block tag with spaces after "</", such as "</ div>". The goldmark fork
// opens an HTML block for it; the spec and Layer 0 do not.
func forkSpacedCloseTag(line []byte) bool {
	// Most lines fail this byte check, which spares them the regexp.
	if i := leadingSpaces(line); i > 3 || !bytes.HasPrefix(line[i:], []byte("</ ")) {
		return false
	}
	m := spacedCloseTag.FindSubmatch(line)
	return m != nil && tagInAllowedSet(m[1])
}
