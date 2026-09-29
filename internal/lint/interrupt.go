package lint

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
// The block detectors are the Layer 0 scanner's, which mirrors the
// goldmark fork. The answer is true when either the fork or the spec
// reads the line as a block start. Where the fork is narrower (it wants
// an uppercase letter after "<!", and a space rather than a tab after a
// block tag name), the spec wins. Where it is broader (it allows spaces
// between "</" and the tag name), the fork wins. So the answer holds for
// both mdsmith's parser and other CommonMark renderers.
func InterruptsParagraph(line []byte) bool {
	if isBlankLine(line) || isSetextUnderline(line) || isThematicBreak(line) || isATXHeadingLine(line) {
		return true
	}
	if _, ok := openingFence(line); ok {
		return true
	}
	if openHTMLBlock(line, true) != htmlNone {
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
