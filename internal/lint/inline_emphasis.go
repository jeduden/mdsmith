package lint

import (
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/markdown"
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
// goldmark's tight-list model, so a tight `- *x*` item never qualifies,
// matching the AST path.
//
// Loose lists are the exception, and the reason for the fallback below. A
// loose list item (one whose list has a blank line between items) holds its
// content in a Paragraph node, so the AST path flags a loose `- *x*` item.
// The run grouper splits at blank lines, so each loose item parses in
// isolation as a *tight* single-item list — losing the looseness and the
// Paragraph wrapper. The flat Layer 0 model cannot tell loose from tight, so
// when the file contains any list block this projection falls back to a full
// document parse and walks its real AST, exactly as LinkReferences falls
// back for container-nested definitions (scanNeedsFallback).
//
// It is computed once per File and cached (atomic.Bool + mutex, matching
// the other File projections), so a list-bearing file that takes the
// full-parse fallback below is not re-parsed on a second call.
//
// nil when no paragraph qualifies or the File has no source. The returned
// slice is shared read-only and must not be mutated.
func WholeParagraphEmphasis(f *File) []EmphasisParagraph {
	if f.emphasisParasDone.Load() {
		return f.emphasisParas
	}
	f.emphasisParasMu.Lock()
	defer f.emphasisParasMu.Unlock()
	if !f.emphasisParasDone.Load() {
		defer f.emphasisParasDone.Store(true)
		f.emphasisParas = scanWholeParagraphEmphasis(f)
	}
	return f.emphasisParas
}

// scanWholeParagraphEmphasis builds the lone-emphasis-paragraph projection:
// the run-grouped inline walk, or a single full-document parse when the
// file contains a list (whose looseness the flat Layer 0 model cannot
// resolve). See WholeParagraphEmphasis for the contract.
func scanWholeParagraphEmphasis(f *File) []EmphasisParagraph {
	if fileHasList(f) {
		return wholeParagraphEmphasisFullParse(f)
	}
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

// fileHasList reports whether the Layer 0 scan recorded any list block.
// List looseness (tight vs loose) decides whether a list item's content is
// wrapped in a Paragraph node, and the flat Layer 0 model does not capture
// it, so the lone-emphasis projection cannot trust the per-run parse when a
// list is present.
func fileHasList(f *File) bool {
	for _, span := range Layer0(f).BlockSpans {
		if span.Kind == BlockList {
			return true
		}
	}
	return false
}

// wholeParagraphEmphasisFullParse is the loose-list fallback: it parses the
// whole document once and walks the real AST, so loose-list paragraphs are
// classified exactly as the AST path classifies them. base is 0 because the
// parse spans the whole Source, so paragraph and segment offsets are already
// document-absolute. f.Source is the front-matter-stripped body (the same
// bytes NewFileLinesFromSource records), so it is parsed directly via
// ParseContext rather than markdown.Parse, which would strip front matter a
// second time and shift every offset.
func wholeParagraphEmphasisFullParse(f *File) []EmphasisParagraph {
	root := markdown.ParseContext(f.Source, parser.NewContext())
	var out []EmphasisParagraph
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		para, ok := n.(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		if p, ok := loneEmphasisFromParagraph(f, para, 0); ok {
			out = append(out, p)
		}
		return ast.WalkContinue, nil
	})
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
//
// Each segment is materialised with t.Segment.Value, the identical call the
// AST-path helper uses, so any Padding / ForceNewline a segment carries is
// reproduced rather than dropped. The segment offsets are run-local, so the
// buffer is f.Source sliced from the run's base; Value reads only
// [Start, Stop) (plus padding) within it.
func emphasisTextSegments(source []byte, base int, emph *ast.Emphasis) []string {
	runBytes := source[base:]
	var segs []string
	_ = ast.Walk(emph, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := n.(*ast.Text); ok {
			segs = append(segs, string(t.Segment.Value(runBytes)))
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
