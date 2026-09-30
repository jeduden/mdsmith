package lint

import (
	"bytes"

	"github.com/jeduden/mdsmith/pkg/goldmark/util"
)

// ExtensionInterruptsParagraph reports whether line starts a block that
// ends an open paragraph under a Markdown extension the canonical parser
// leaves off. Such a line is paragraph text to mdsmith's own parser, but
// the flavor parser, GFM renderers and other extended renderers read it
// as a block:
//
//   - a GFM table delimiter row, which turns the line before it into a
//     table header
//   - a footnote definition, "[^label]:"
//
// Up to three spaces of indent are allowed.
func ExtensionInterruptsParagraph(line []byte) bool {
	return isTableDelimiterRow(line) || isFootnoteDefinition(line)
}

// isTableDelimiterRow reports whether line is a GFM table delimiter
// row: cells split by '|', each a run of '-' with an optional ':' on
// either side and optional spaces or tabs around it. The outer pipes are
// optional, so ":-" alone is a one-column row. A line of '-' alone is
// not one (it is a setext underline or a thematic break). The row makes
// a table only when the line before it has as many cells, which a check
// of one line cannot see, so every delimiter row counts.
func isTableDelimiterRow(line []byte) bool {
	indent := leadingSpaces(line)
	if indent > 3 {
		return false
	}
	row := bytes.TrimRight(line[indent:], " \t")
	if len(bytes.Trim(row, "-")) == 0 {
		return false
	}
	row = bytes.TrimPrefix(row, []byte("|"))
	row = bytes.TrimSuffix(row, []byte("|"))
	if len(row) == 0 {
		return false
	}
	for {
		cell, rest, more := bytes.Cut(row, []byte("|"))
		if !isDelimiterCell(cell) {
			return false
		}
		if !more {
			return true
		}
		row = rest
	}
}

// isDelimiterCell reports whether cell is one delimiter-row cell: spaces
// or tabs, an optional ':', one or more '-', an optional ':', then
// spaces or tabs.
func isDelimiterCell(cell []byte) bool {
	cell = bytes.Trim(cell, " \t")
	cell = bytes.TrimPrefix(cell, []byte(":"))
	cell = bytes.TrimSuffix(cell, []byte(":"))
	return len(cell) > 0 && len(bytes.Trim(cell, "-")) == 0
}

// isFootnoteDefinition reports whether line opens a footnote definition
// as the goldmark footnote extension reads one: "[^", a label, then "]:"
// with anything after it. The label is not blank and has no unescaped
// '['; it ends at the first unescaped ']'. A backslash escapes an ASCII
// punctuation byte.
func isFootnoteDefinition(line []byte) bool {
	indent := leadingSpaces(line)
	if indent > 3 || !bytes.HasPrefix(line[indent:], []byte("[^")) {
		return false
	}
	start := indent + 2
	for i := start; i < len(line); i++ {
		switch line[i] {
		case '\\':
			if i+1 < len(line) && util.IsPunct(line[i+1]) {
				i++
			}
		case '[':
			return false
		case ']':
			return i+1 < len(line) && line[i+1] == ':' && !isBlankLine(line[start:i])
		}
	}
	return false
}
