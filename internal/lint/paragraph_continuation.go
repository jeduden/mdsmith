package lint

import (
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/markdown"
)

// paragraphContinuationKey is the File.MemoFile key for the
// ParagraphContinuationLines projection.
const paragraphContinuationKey = "lint.ParagraphContinuationLines"

// ParagraphContinuationLines returns the 1-based source lines that
// continue a paragraph: every line of a paragraph after its first, in
// any container, lazy continuation lines included. goldmark stores a
// tight list item's paragraph as a TextBlock, so those count as well,
// and so do a setext heading's content lines, which are paragraph text
// until the underline. CommonMark reads such a line as text, so a
// line-based rule must not treat it as the start of a block: `#48`
// there is part of the paragraph, not a malformed ATX heading.
//
// It walks f.AST when the File was parsed. On the parse-skipped path
// (f.AST nil) it parses f.Source once, so both paths return the same
// set. The result is memoised per File and shared read-only; nil when
// the document has no multi-line paragraph.
func ParagraphContinuationLines(f *File) map[int]struct{} {
	lines, _ := f.MemoFile(paragraphContinuationKey, buildParagraphContinuationLines).(map[int]struct{})
	return lines
}

// buildParagraphContinuationLines computes the ParagraphContinuationLines
// set. f.Source is the front-matter-stripped body, so the nil-AST parse
// goes through ParseContext (not markdown.Parse, which would strip front
// matter a second time) and its offsets are already body-absolute.
func buildParagraphContinuationLines(f *File) any {
	root := f.AST
	if root == nil {
		root = markdown.ParseContext(f.Source, parser.NewContext())
	}
	var lines map[int]struct{}
	collectParagraphContinuations(root, f, &lines)
	return lines
}

// collectParagraphContinuations descends n and records every line after
// the first of each paragraph, text block, or heading into *lines,
// allocating the map on first use. An ATX heading has one line, so only
// setext headings contribute. The children of these nodes are inlines,
// so the walk does not descend into them.
func collectParagraphContinuations(n ast.Node, f *File, lines *map[int]struct{}) {
	switch n.Kind() {
	case ast.KindParagraph, ast.KindTextBlock, ast.KindHeading:
		segs := n.Lines()
		for i := 1; i < segs.Len(); i++ {
			if *lines == nil {
				*lines = make(map[int]struct{}, segs.Len())
			}
			(*lines)[f.LineOfOffset(segs.At(i).Start)] = struct{}{}
		}
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectParagraphContinuations(c, f, lines)
	}
}
