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
// It reads the shared run-grouped inline parse (InlineBlocks): every
// paragraph in every run — including the paragraphs goldmark nests inside a
// block quote — is tested for the lone-emphasis shape, exactly the set of
// paragraph nodes the AST walk visits. List items are not paragraphs in
// goldmark's tight-list model, so a `- *x*` item never qualifies, matching
// the AST path.
//
// nil when no paragraph qualifies or the File has no source. The returned
// slice is shared read-only and must not be mutated.
func WholeParagraphEmphasis(f *File) []EmphasisParagraph {
	var out []EmphasisParagraph
	for _, blk := range InlineBlocks(f) {
		base := blk.Offset
		_ = ast.Walk(blk.Node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			para, ok := n.(*ast.Paragraph)
			if !ok {
				return ast.WalkContinue, nil
			}
			if p, ok := loneEmphasisFromParagraph(f, para, base); ok {
				out = append(out, p)
			}
			return ast.WalkContinue, nil
		})
	}
	return out
}

// loneEmphasisFromParagraph tests one parsed paragraph for the
// lone-emphasis-child shape and, when it matches, returns the
// EmphasisParagraph describing it. base is the run's start offset in
// f.Source, used to map the paragraph's run-local first line and its
// emphasis Text segments back to the document.
func loneEmphasisFromParagraph(f *File, para *ast.Paragraph, base int) (EmphasisParagraph, bool) {
	first := para.FirstChild()
	if first == nil || first.NextSibling() != nil {
		return EmphasisParagraph{}, false
	}
	emph, isEmph := first.(*ast.Emphasis)
	if !isEmph {
		return EmphasisParagraph{}, false
	}
	line := f.LineOfOffset(base)
	if local := paraLocalFirstLineOffset(para); local >= 0 {
		line = f.LineOfOffset(base + local)
	}
	return EmphasisParagraph{Line: line, TextSegments: emphasisTextSegments(f.Source, base, emph)}, true
}

// emphasisTextSegments returns the plain-text value of every Text
// descendant of the emphasis node, in walk order, read from f.Source via
// the run-absolute offsets (base + the run-local segment bounds). It
// mirrors, segment by segment, the incremental text the AST-path
// placeholder check accumulates over the emphasis subtree — so a caller
// that re-accumulates and tests each prefix reproduces that check
// byte-identically, including its early stop.
func emphasisTextSegments(source []byte, base int, emph *ast.Emphasis) []string {
	var segs []string
	_ = ast.Walk(emph, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := n.(*ast.Text); ok {
			segs = append(segs, string(source[base+t.Segment.Start:base+t.Segment.Stop]))
		}
		return ast.WalkContinue, nil
	})
	return segs
}

// paraLocalFirstLineOffset returns the byte offset (within the run) of the
// paragraph's first content line, or -1 when the node carries no line
// information.
func paraLocalFirstLineOffset(para *ast.Paragraph) int {
	lines := para.Lines()
	if lines == nil || lines.Len() == 0 {
		return -1
	}
	return lines.At(0).Start
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
