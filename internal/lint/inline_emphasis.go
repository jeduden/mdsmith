package lint

import (
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// EmphasisParagraph is one paragraph whose sole inline child is a single
// emphasis (or strong emphasis) span — the exact shape MDS018
// (no-emphasis-as-heading) flags. Line is the paragraph's 1-based source
// line; TextSegments are the ordered plain-text values of the emphasis
// span's Text descendants, so the rule can apply its placeholder filter
// with the same incremental accumulation the AST path uses without
// re-parsing.
type EmphasisParagraph struct {
	Line         int
	TextSegments []string
}

// WholeParagraphEmphasis returns every paragraph whose sole inline child is
// a single emphasis span, in document order. It is the Layer 1 projection
// source for MDS018 (no-emphasis-as-heading) on the parse-skipped path
// (f.AST nil), so the rule no longer forces a whole-document goldmark parse
// to read one parsed emphasis node.
//
// The detector is bounded: it parses only the paragraphs whose first
// non-space byte is `*` or `_` (the only bytes that can open emphasis),
// one block at a time, rather than the whole document. A paragraph that
// cannot open emphasis is skipped without any parse. This is the per-block
// lazy parse the plan calls for: re-using goldmark's own inline parser on a
// single block keeps the lone-emphasis result byte-identical to the AST
// path by construction, while the byte gate keeps the cost proportional to
// the rare emphasis-led paragraph rather than every paragraph.
//
// nil when no paragraph qualifies or the File has no source. Computed once
// per File and cached; the slice is shared read-only and must not be
// mutated.
func WholeParagraphEmphasis(f *File) []EmphasisParagraph {
	if f.emphasisLinesDone.Load() {
		return f.emphasisLines
	}
	f.emphasisLinesMu.Lock()
	defer f.emphasisLinesMu.Unlock()
	if !f.emphasisLinesDone.Load() {
		defer f.emphasisLinesDone.Store(true)
		f.emphasisLines = scanWholeParagraphEmphasis(f)
	}
	return f.emphasisLines
}

// WholeParagraphEmphasisLines returns just the 1-based source lines of the
// paragraphs WholeParagraphEmphasis reports, in document order. nil when
// none qualify.
func WholeParagraphEmphasisLines(f *File) []int {
	paras := WholeParagraphEmphasis(f)
	if len(paras) == 0 {
		return nil
	}
	lines := make([]int, len(paras))
	for i, p := range paras {
		lines[i] = p.Line
	}
	return lines
}

// scanWholeParagraphEmphasis walks the Layer 0 block spans and, for each
// paragraph whose first non-space byte can open emphasis, parses that
// block's bytes in isolation to test the lone-emphasis-child shape.
func scanWholeParagraphEmphasis(f *File) []EmphasisParagraph {
	if len(f.Source) == 0 {
		return nil
	}
	l0 := Layer0(f)
	var out []EmphasisParagraph
	for _, span := range l0.BlockSpans {
		if span.Kind != BlockParagraph {
			continue
		}
		if p, ok := LoneEmphasisParagraph(f, span); ok {
			out = append(out, p)
		}
	}
	return out
}

// LoneEmphasisParagraph tests one Layer 0 paragraph span for the
// emphasis-as-heading shape and, when it matches, returns the
// EmphasisParagraph describing it. It is the per-span entry point the
// engine's block dispatch drives for MDS018 on the parse-skipped path:
// CheckBlock(span) calls it once per BlockParagraph span. The span is
// gated on its first non-space byte being a delimiter before any parse, so
// a non-emphasis paragraph costs only the cheap byte check.
func LoneEmphasisParagraph(f *File, span BlockSpan) (EmphasisParagraph, bool) {
	if span.Kind != BlockParagraph || !paragraphMayOpenEmphasis(f, span) {
		return EmphasisParagraph{}, false
	}
	return loneEmphasisParagraph(f, span)
}

// paragraphMayOpenEmphasis reports whether the first non-space byte of the
// paragraph's first line is `*` or `_` — the only delimiter characters
// goldmark's emphasis parser triggers on. Paragraphs that fail this gate
// can never parse to a lone emphasis child, so they are skipped without a
// parse.
func paragraphMayOpenEmphasis(f *File, span BlockSpan) bool {
	line := f.lineBytes(span.Start)
	for _, b := range line {
		switch b {
		case ' ', '\t':
			continue
		case '*', '_':
			return true
		default:
			return false
		}
	}
	return false
}

// loneEmphasisParagraph parses the source bytes of one paragraph block in
// isolation and, when the parse yields a single paragraph whose only inline
// child is an emphasis node, returns the paragraph's source line (in the
// original document's coordinates) and the emphasis span's plain inner
// text. The block is parsed standalone because emphasis resolution is local
// to a paragraph: the delimiter run, its flanking context, and its closer
// all lie within the block, so the inline tree is identical to the one the
// whole-document parse produces for the same paragraph.
func loneEmphasisParagraph(f *File, span BlockSpan) (EmphasisParagraph, bool) {
	start := f.LineStartOffset(span.Start - 1)
	end := f.lineEndOffset(span.End - 1)
	if end <= start {
		return EmphasisParagraph{}, false
	}
	block := f.Source[start:end]
	doc := ParseInline(block)
	para, ok := singleParagraph(doc)
	if !ok {
		return EmphasisParagraph{}, false
	}
	first := para.FirstChild()
	if first == nil || first.NextSibling() != nil {
		return EmphasisParagraph{}, false
	}
	emph, isEmph := first.(*ast.Emphasis)
	if !isEmph {
		return EmphasisParagraph{}, false
	}
	// The paragraph's first content line, in the original document's
	// coordinates: the local paragraph's first text line offset plus the
	// block's start offset maps back through LineOfOffset.
	line := span.Start
	if local := paraLocalFirstLineOffset(para); local >= 0 {
		line = f.LineOfOffset(start + local)
	}
	return EmphasisParagraph{Line: line, TextSegments: emphasisTextSegments(emph, block)}, true
}

// emphasisTextSegments returns the plain-text value of every Text
// descendant of the emphasis node, in walk order, against the standalone
// block bytes the node was parsed from. It mirrors, segment by segment, the
// incremental text the AST-path placeholder check accumulates over the
// emphasis subtree — so a caller that re-accumulates and tests each prefix
// reproduces that check byte-identically, including its early stop.
func emphasisTextSegments(emph *ast.Emphasis, block []byte) []string {
	var segs []string
	_ = ast.Walk(emph, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := n.(*ast.Text); ok {
			segs = append(segs, string(t.Segment.Value(block)))
		}
		return ast.WalkContinue, nil
	})
	return segs
}

// singleParagraph returns the lone Paragraph child of a parsed standalone
// block, or ok=false when the block parsed to anything else (no paragraph,
// several blocks, a heading, a list, …). A lone-emphasis-child shape can
// only arise inside a single paragraph, so a non-paragraph or multi-block
// parse is never the emphasis-as-heading shape this detector reports.
func singleParagraph(doc ast.Node) (*ast.Paragraph, bool) {
	first := doc.FirstChild()
	if first == nil || first.NextSibling() != nil {
		return nil, false
	}
	para, ok := first.(*ast.Paragraph)
	if !ok {
		return nil, false
	}
	return para, true
}

// paraLocalFirstLineOffset returns the byte offset (within the standalone
// block) of the paragraph's first content line, or -1 when the node
// carries no line information.
func paraLocalFirstLineOffset(para *ast.Paragraph) int {
	lines := para.Lines()
	if lines == nil || lines.Len() == 0 {
		return -1
	}
	return lines.At(0).Start
}

// lineBytes returns the bytes of 1-based source line n (without the line
// terminator), or nil when n is out of range.
func (f *File) lineBytes(n int) []byte {
	if n < 1 || n > len(f.Lines) {
		return nil
	}
	return f.Lines[n-1]
}

// lineEndOffset returns the byte offset in Source just past the end of
// 0-based line i (the newline, or len(Source) for the last line).
func (f *File) lineEndOffset(i int) int {
	nl := f.lineIndex()
	if i < 0 {
		return 0
	}
	if i >= len(nl) {
		return len(f.Source)
	}
	return nl[i]
}
