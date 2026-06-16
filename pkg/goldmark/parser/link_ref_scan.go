package parser

import (
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
)

// ScanReferenceDefinitions parses the link reference definitions that
// open the block described by lineSegments and records each one in pc
// via AddReference, exactly as the link-reference paragraph
// transformer does during a full parse. It exists so a byte-level
// caller (mdsmith's Layer 1 reference scanner) can collect a
// document's reference map by feeding it each candidate paragraph's
// line segments, without building the whole document AST.
//
// lineSegments must hold one Segment per source line of the candidate
// block, each spanning [lineStart, nextLineStart) so that its Stop
// includes the trailing newline — the boundary goldmark's own reader
// produces. source is the full document buffer the segments index
// into.
//
// Only definitions at the head of the block are recognised, matching
// parseLinkReferenceDefinition's contract: the loop stops at the first
// line that is not a definition, leaving the rest of the block (a
// paragraph) untouched. AddReference keeps the first definition for a
// given normalised label, so calling this for blocks in document order
// reproduces the full parse's first-wins dedup.
func ScanReferenceDefinitions(source []byte, lineSegments *text.Segments, pc Context) {
	if lineSegments == nil || lineSegments.Len() == 0 {
		return
	}
	block := text.NewBlockReader(source, lineSegments)
	for {
		_, start, _ := parseLinkReferenceDefinition(block, pc)
		if start < 0 {
			return
		}
	}
}
