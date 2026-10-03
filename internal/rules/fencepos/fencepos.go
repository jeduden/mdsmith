// Package fencepos answers "where in the source does a fenced code
// block's opening and closing fence line sit, and what fence character
// did the author use?" The helpers here let any rule reason about the
// raw fence delimiters of a *ast.FencedCodeBlock without owning that
// scanning logic itself.
package fencepos

import (
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// OpenLine returns the 1-based line number of the opening fence.
func OpenLine(f *lint.File, fcb *ast.FencedCodeBlock) int {
	start, _ := OpenLineRange(f.Source, fcb)
	return f.LineOfOffset(start)
}

// CloseLine returns the 1-based line number of the closing fence.
func CloseLine(f *lint.File, fcb *ast.FencedCodeBlock) int {
	_, openEnd := OpenLineRange(f.Source, fcb)
	start, _ := CloseLineRange(f.Source, fcb, openEnd)
	return f.LineOfOffset(start)
}

// OpenLineRange returns the byte range [start, end) of the opening
// fence line (without trailing newline). The parser records the
// opener's offset as the node position, so the line holding it is the
// opening fence line whatever the block holds and wherever it sits:
// indented, in a list item, or in a block quote. A node without a
// position inside src (one built by hand, not parsed) yields the
// (len(src), len(src)) sentinel, which every caller skips.
func OpenLineRange(src []byte, fcb *ast.FencedCodeBlock) (int, int) {
	if p := fcb.Pos(); p >= 0 && p < len(src) {
		return lineAround(src, p)
	}
	return len(src), len(src)
}

// lineAround returns the byte range [start, end) of the line holding
// offset p (without trailing newline).
func lineAround(src []byte, p int) (int, int) {
	start, end := p, p
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	for end < len(src) && src[end] != '\n' {
		end++
	}
	return start, end
}

// CloseLineRange returns the byte range [start, end) of the closing
// fence line (without trailing newline). openEnd is the byte offset
// returned by OpenLineRange for the same block.
func CloseLineRange(src []byte, fcb *ast.FencedCodeBlock, openEnd int) (int, int) {
	var closingStart int
	if fcb.Lines().Len() > 0 {
		lastLine := fcb.Lines().At(fcb.Lines().Len() - 1)
		closingStart = lastLine.Stop
	} else {
		closingStart = openEnd
		if closingStart < len(src) && src[closingStart] == '\n' {
			closingStart++
		}
	}
	closingEnd := closingStart
	for closingEnd < len(src) && src[closingEnd] != '\n' {
		closingEnd++
	}
	return closingStart, closingEnd
}
