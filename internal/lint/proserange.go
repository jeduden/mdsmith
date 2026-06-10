package lint

import (
	"github.com/jeduden/mdsmith/internal/piparser"
	"github.com/yuin/goldmark/ast"
)

// Range is a half-open byte range [Start, End) into File.Source.
type Range struct{ Start, End int }

// ProseRanges returns the byte ranges of File.Source that fall inside
// prose nodes — the text of paragraphs, headings, list items, and
// blockquotes — with the spans of fenced code blocks, indented code
// blocks, HTML blocks, inline code spans, autolinks, and inline raw
// HTML excluded. Ranges are in ascending document order and never
// overlap.
//
// The projection is derived from f.AST, not re-implemented from
// f.Lines: a Lines-only rewrite of every prose rule would otherwise
// each re-grow a fence/HTML/code-span scanner, the exact parallel-parser
// divergence plan 2606022126 set out to avoid. A rule that only inspects prose
// text (proper-name casing, forbidden substrings, most readability
// checks) scans these ranges with bytes helpers and never walks the AST
// itself; the AST's implicit code-skipping filter is reproduced once
// here and shared.
//
// Computed once per File and cached; the returned slice is shared
// read-only and must not be mutated. Returns nil for a File with a nil
// AST (struct-literal Files in unit tests), matching the collect*
// helpers' guard. The atomic.Bool + mutex memo avoids the once.Do
// closure box — see the File.proseRanges field comment.
func (f *File) ProseRanges() []Range {
	if f.proseRangesDone.Load() {
		return f.proseRanges
	}
	f.proseRangesMu.Lock()
	defer f.proseRangesMu.Unlock()
	if !f.proseRangesDone.Load() {
		defer f.proseRangesDone.Store(true)
		f.proseRanges = collectProseRanges(f.AST)
	}
	return f.proseRanges
}

// collectProseRanges walks the AST and gathers the segments of every
// inline text node that lies outside an excluded subtree.
func collectProseRanges(root ast.Node) []Range {
	if root == nil {
		return nil
	}
	var out []Range
	collectProseRangesInto(root, &out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// collectProseRangesInto descends node n by recursion (not ast.Walk, so
// the per-File build sheds the closure box ast.Walk allocates) and
// appends each prose Text segment to out.
//
// The exclusions are realized by NOT descending into the node types
// whose content is non-prose:
//
//   - FencedCodeBlock / CodeBlock / HTMLBlock: block-level code/HTML.
//     Their bytes live in node.Lines(), not in child Text nodes, so
//     even descending would not surface them — but pruning the subtree
//     also skips any stray inline children and is cheaper.
//   - CodeSpan: an inline code span's content IS exposed as child Text
//     nodes, so the span must be pruned to keep `code` out of prose.
//   - AutoLink: stores its URL Text in a private field (not a child),
//     so the walk never reaches it; pruning is belt-and-suspenders.
//   - RawHTML: inline raw HTML keeps its bytes in Segments, not child
//     Text nodes, so it contributes nothing — pruned for clarity.
//   - ProcessingInstruction: mdsmith directive markers; their body is a
//     generated section, never prose to lint.
//
// A Link node is NOT pruned: its visible text is real prose (child Text
// nodes), while its destination lives in Link.Destination (not a Text
// node), so the link text is included and the URL is excluded for free.
func collectProseRangesInto(n ast.Node, out *[]Range) {
	switch t := n.(type) {
	case *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock,
		*ast.CodeSpan, *ast.AutoLink, *ast.RawHTML, *piparser.ProcessingInstruction:
		return
	case *ast.Text:
		seg := t.Segment
		if seg.Stop > seg.Start {
			appendProseRange(out, seg.Start, seg.Stop)
		}
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectProseRangesInto(c, out)
	}
}

// appendProseRange adds [start, stop) to out, coalescing it with the
// previous range when they are adjacent or overlap. Adjacent Text
// segments are common (goldmark splits a paragraph at soft line breaks
// and inline-markup boundaries), so coalescing keeps the slice small and
// lets a scanning rule treat a contiguous run of prose as one span.
func appendProseRange(out *[]Range, start, stop int) {
	if n := len(*out); n > 0 && start <= (*out)[n-1].End {
		if stop > (*out)[n-1].End {
			(*out)[n-1].End = stop
		}
		return
	}
	*out = append(*out, Range{Start: start, End: stop})
}
