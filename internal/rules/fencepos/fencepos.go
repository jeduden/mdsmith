// Package fencepos answers "where in the source does a fenced code
// block's opening and closing fence line sit, and what fence character
// did the author use?" The helpers here let any rule reason about the
// raw fence delimiters of a *ast.FencedCodeBlock without owning that
// scanning logic itself.
package fencepos

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// CharAt returns the fence character at the given position, skipping
// leading spaces. Returns 0 when no fence character (` or ~) follows.
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
func OpenLineRange(src []byte, fcb *ast.FencedCodeBlock) (int, int) {
	if fcb.Info != nil {
		// Walk back from info start to find line start
		lineStart := fcb.Info.Segment.Start
		for lineStart > 0 && src[lineStart-1] != '\n' {
			lineStart--
		}
		// Line end is the end of the info segment (there may be trailing space)
		lineEnd := fcb.Info.Segment.Stop
		for lineEnd < len(src) && src[lineEnd] != '\n' {
			lineEnd++
		}
		return lineStart, lineEnd
	}
	if fcb.Lines().Len() > 0 {
		firstContentStart := fcb.Lines().At(0).Start
		// Walk backwards past the newline ending the opening fence line
		pos := firstContentStart
		if pos > 0 && src[pos-1] == '\n' {
			pos--
		}
		lineEnd := pos
		lineStart := pos
		for lineStart > 0 && src[lineStart-1] != '\n' {
			lineStart--
		}
		return lineStart, lineEnd
	}
	// Empty code block with no info - scan from previous sibling or start of file
	searchStart := 0
	if prev := fcb.PreviousSibling(); prev != nil {
		searchStart = lastByteOfNodeStop(src, prev)
	}
	pos := searchStart
	for pos < len(src) {
		lineStart := pos
		lineEnd := pos
		for lineEnd < len(src) && src[lineEnd] != '\n' {
			lineEnd++
		}
		line := bytes.TrimLeft(src[lineStart:lineEnd], " ")
		if bytes.HasPrefix(line, []byte("```")) || bytes.HasPrefix(line, []byte("~~~")) {
			return lineStart, lineEnd
		}
		if lineEnd >= len(src) {
			break
		}
		pos = lineEnd + 1
	}
	return len(src), len(src)
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

func lastByteOfNodeStop(src []byte, n ast.Node) int {
	if block, ok := n.(interface{ Lines() *text.Segments }); ok {
		lines := block.Lines()
		if lines.Len() > 0 {
			return lines.At(lines.Len() - 1).Stop
		}
	}
	return 0
}
