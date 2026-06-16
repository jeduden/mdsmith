package lint

import (
	"bytes"

	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
)

// refDefMarker is the minimal byte sequence every link reference
// definition contains: a `]` immediately followed by `:`. A paragraph
// whose head holds no `]:` cannot open a definition, so the scanner
// skips it without building any segments. A block quote or list line
// holding `]:` is the fallback trigger (see scanNeedsFallback).
var refDefMarker = []byte("]:")

// scanLinkReferences returns the link reference definitions in f by
// scanning the head of every Layer 0 paragraph block, without a full
// goldmark document parse. It mirrors goldmark's first-wins dedup: the
// first definition for a normalised label survives, so paragraphs are
// fed to the shared parser scanner in document order.
//
// The scanner only descends into top-level (Depth 0) paragraph blocks.
// Definitions nested in a block quote or list item are real to goldmark
// but invisible here, so a caller that needs full coverage must consult
// scanNeedsFallback first and parse the document when it returns true.
// scanLinkReferences itself returns whatever the paragraph heads yield;
// LinkReferences applies the fallback gate.
func scanLinkReferences(f *File) []Reference {
	l0 := Layer0(f)
	source := f.Source

	// Build line segments lazily, only once a candidate paragraph is
	// found, and reuse the same backing across candidates. Most
	// documents reach the end without ever allocating it.
	var allSegs []text.Segment
	var view *text.Segments
	ctx := parser.NewContext()

	for _, span := range l0.BlockSpans {
		if span.Kind != BlockParagraph || span.Depth != 0 {
			continue
		}
		// Cheap reject: a paragraph whose first non-blank byte is not
		// '[' cannot open a definition, and one with no `]:` anywhere in
		// its first line cannot either. parseLinkReferenceDefinition will
		// re-confirm; this only spares the segment build for prose.
		if !paragraphHeadMayDefine(f, span) {
			continue
		}
		if allSegs == nil {
			allSegs = buildLineSegments(source)
			view = text.NewSegments()
		}
		// span.Start/End are 1-based inclusive line numbers; allSegs is
		// 0-indexed by line. Layer0 guarantees Start <= End and both
		// within the real line range, so no bounds clamp is needed.
		lo := span.Start - 1
		hi := span.End
		view.SetBacking(allSegs[lo:hi], nil)
		parser.ScanReferenceDefinitions(source, view, ctx)
	}

	return ctx.References()
}

// scanNeedsFallback reports whether f holds a block quote or list block
// whose lines contain a `]:` marker — a place a link reference
// definition could legally live that the paragraph-head scanner does not
// descend into. When true, LinkReferences parses the whole document
// instead of trusting the scanner, trading the parse for guaranteed
// byte-identity on these rare shapes.
//
// It also detects tight list continuations: a depth-0 paragraph that
// immediately follows a BlockList span (no blank line between) is a
// list item continuation in goldmark's model — both lines belong to
// the same paragraph, which starts with the item text, so goldmark
// will not extract a reference definition from the continuation line
// even if it looks like one. The byte scanner, whose Layer0 is flat,
// would otherwise admit that paragraph and produce a false positive.
func scanNeedsFallback(f *File) bool {
	l0 := Layer0(f)
	lastListEnd := -1 // 1-based line number where the last BlockList span ended
	for _, span := range l0.BlockSpans {
		switch span.Kind {
		case BlockQuote, BlockList:
			lo := span.Start - 1
			hi := span.End
			for i := lo; i < hi; i++ {
				if bytes.Contains(f.Lines[i], refDefMarker) {
					return true
				}
			}
			if span.Kind == BlockList {
				lastListEnd = span.End
			} else {
				lastListEnd = -1
			}
		case BlockParagraph:
			// A paragraph that starts on the line immediately after a
			// BlockList span (span.Start == lastListEnd+1) is a tight list
			// continuation. Trigger the fallback if it contains `]:` so that
			// the full parse (which correctly ignores the continuation as a
			// definition) is used instead of the byte scanner.
			if lastListEnd >= 0 && span.Start == lastListEnd+1 {
				lo := span.Start - 1
				hi := span.End
				for i := lo; i < hi; i++ {
					if bytes.Contains(f.Lines[i], refDefMarker) {
						return true
					}
				}
			}
			lastListEnd = -1
		default:
			lastListEnd = -1
		}
	}
	return false
}

// paragraphHeadMayDefine reports whether the first line of the paragraph
// span could open a link reference definition: its first non-space byte
// is '[' and the line carries a `]:` marker. A laxer gate than
// parseLinkReferenceDefinition, so it can only over-admit (falling
// through to the full definition scan), never skip a real definition.
func paragraphHeadMayDefine(f *File, span BlockSpan) bool {
	idx := span.Start - 1
	line := f.Lines[idx]
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) || line[i] != '[' {
		return false
	}
	// The `]:` may be on a later line of a multi-line label, so check the
	// whole span head region rather than only the first line.
	for j := idx; j < span.End; j++ {
		if bytes.Contains(f.Lines[j], refDefMarker) {
			return true
		}
	}
	return false
}

// buildLineSegments returns one Segment per source line, each spanning
// [lineStart, nextLineStart) so its Stop includes the trailing newline —
// the boundary goldmark's reader produces. A source not ending in a
// newline yields a final segment to len(source). Pre-sized to the line
// count so the build is a single allocation.
func buildLineSegments(source []byte) []text.Segment {
	n := bytes.Count(source, lineIndexNewline) + 1
	segs := make([]text.Segment, 0, n)
	start := 0
	for i := 0; i < len(source); i++ {
		if source[i] == '\n' {
			segs = append(segs, text.NewSegment(start, i+1))
			start = i + 1
		}
	}
	if start < len(source) {
		segs = append(segs, text.NewSegment(start, len(source)))
	}
	return segs
}
