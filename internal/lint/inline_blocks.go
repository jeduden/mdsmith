package lint

import (
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/markdown"
)

// ParseInline parses block as a standalone Markdown document and returns
// the resulting AST root. It is the per-block lazy-parse seam inline rules
// use on the parse-skipped path (f.AST nil): re-using goldmark's own parser
// on a single Layer 0 block span keeps the inline tree byte-identical to
// the one a whole-document parse produces for that block — link, image,
// autolink, code-span, and emphasis resolution are all local to a
// paragraph or list item, never crossing a blank-line block boundary.
//
// The caller maps any block-local segment offset back to the document by
// adding the span's start offset (LineStartOffset of the span's first
// line). The returned tree shares no state with the File.
func ParseInline(block []byte) ast.Node {
	return markdown.ParseContext(block, parser.NewContext())
}

// LineEndOffset returns the byte offset in Source of the newline that ends
// 0-based source line i (or len(Source) for the last line). Paired with
// LineStartOffset, it bounds a Layer 0 span's bytes for a per-block parse:
// a span covering 1-based lines [Start, End] slices
// Source[LineStartOffset(Start-1):LineEndOffset(End-1)]. i < 0 returns 0;
// i past the last line returns len(Source).
func (f *File) LineEndOffset(i int) int {
	return f.lineEndOffset(i)
}
