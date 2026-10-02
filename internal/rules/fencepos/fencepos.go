// Package fencepos answers "where in the source does a fenced code
// block's opening and closing fence line sit, and what fence character
// did the author use?" The helpers here let any rule reason about the
// raw fence delimiters of a *ast.FencedCodeBlock without owning that
// scanning logic itself.
package fencepos

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdfence"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// CharAt returns the fence character at the given position, skipping
// leading spaces. Returns 0 when no fence character (` or ~) follows.
// It reads the character of a line the parser already opened a fence
// on and decides no fence-ness itself, so it stays a plain byte read
// rather than an mdfence call.
func CharAt(src []byte, pos int) byte {
	for pos < len(src) && src[pos] == ' ' {
		pos++
	}
	if pos < len(src) && (src[pos] == '`' || src[pos] == '~') {
		return src[pos]
	}
	return 0
}

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
// fence line (without trailing newline).
//
// A parsed block carries its opening position (Node.Pos, the offset of
// the fence run), so the line around it is the opening line in every
// layout: inside a list item or block quote, and for an empty fence
// with no info string, which goldmark gives neither an info nor a
// content segment. The remaining branches serve only blocks built by
// hand, which have no position.
func OpenLineRange(src []byte, fcb *ast.FencedCodeBlock) (int, int) {
	if p := fcb.Pos(); p >= 0 && p <= len(src) {
		return lineAround(src, p)
	}
	if fcb.Info != nil {
		return lineAround(src, fcb.Info.Segment.Start)
	}
	if fcb.Lines().Len() > 0 {
		// The opening fence line ends just before the first content line.
		pos := fcb.Lines().At(0).Start
		if pos > 0 && src[pos-1] == '\n' {
			pos--
		}
		return lineAround(src, pos)
	}
	// No position, info, or content: the first line mdfence reads as an
	// opening fence.
	for pos := 0; pos < len(src); {
		lineEnd := pos + lineLen(src[pos:])
		if _, ok := mdfence.Open(src[pos:lineEnd]); ok {
			return pos, lineEnd
		}
		pos = lineEnd + 1
	}
	return len(src), len(src)
}

// lineAround returns the byte range [start, end) of the line holding
// offset p, without its trailing newline.
func lineAround(src []byte, p int) (int, int) {
	start := p
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	return start, p + lineLen(src[p:])
}

// lineLen returns the length of b's first line, without its newline.
func lineLen(b []byte) int {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return i
	}
	return len(b)
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
