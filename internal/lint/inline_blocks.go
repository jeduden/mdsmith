package lint

import (
	"bytes"

	"github.com/jeduden/mdsmith/pkg/goldmark/arena"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/markdown"
)

// refDefMarker is the literal every link reference definition contains: the
// `]` closing its label immediately followed by the `:` before its
// destination. A source without it has no definitions to seed.
var refDefMarker = []byte("]:")

// InlineBlock is one contiguous run of inline-bearing source lines whose
// markup has been parsed in isolation on the parse-skipped path. Node is
// the parsed run's root (a goldmark Document holding the re-parsed lines);
// Offset is the byte offset, in the original document's Source, of the
// run's first byte, so a caller maps any run-local segment offset back to
// the document with Offset + segment.Start.
type InlineBlock struct {
	Node   ast.Node
	Offset int
}

// InlineBlocks returns the inline markup of the document parsed one
// contiguous run of inline-bearing lines at a time, computed once per File
// and cached. It is the single shared inline-node stream for the
// parse-skipped path (f.AST nil): every inline rule consumes it, so a run
// is parsed once per file rather than once per rule. The returned slice and
// the parsed nodes are shared read-only and must not be mutated.
//
// A run is the maximal span of consecutive lines that are not blank and
// carry no code-block, processing-instruction, or HTML-block content (the
// lines the Layer 0 scan marks classCode / classPI, or that fall inside a
// BlockHTML span). goldmark parses no inline link/image markup on those
// lines, so they are excluded; everything else — paragraphs, ATX and setext
// headings, list items, block quotes — is parsed as part of its run.
// Grouping by run (rather than by Layer 0 block span, which records a list
// item or block quote as a single line) keeps a construct that wraps onto a
// continuation line whole, so its link, image, or emphasis is reconstructed
// exactly as the whole-document parse reconstructs it.
//
// Each run is parsed with the document's link reference definitions
// (f.LinkReferences) pre-seeded into the parse context, so a reference-style
// link or image whose definition lives in another block still resolves to a
// Link / Image node — matching the whole-document parse, where all
// definitions are visible to every block.
func InlineBlocks(f *File) []InlineBlock {
	if f.inlineBlocksDone.Load() {
		return f.inlineBlocks
	}
	f.inlineBlocksMu.Lock()
	defer f.inlineBlocksMu.Unlock()
	if !f.inlineBlocksDone.Load() {
		defer f.inlineBlocksDone.Store(true)
		f.inlineBlocks = scanInlineBlocks(f)
	}
	return f.inlineBlocks
}

// nonInlineLines returns the set of 1-based source lines whose bytes carry
// no inline markup: fenced/indented code-block lines, PI-block lines, and
// every line inside an HTML block. goldmark parses no inline content on
// these lines, so they never open or continue an inline run. The set is
// built from the Layer 0 scan: its CodeBlockLines and PIBlockLines sets plus
// the line span of every BlockHTML block. Returns the CodeBlockLines map
// directly (no copy) when there are no PI or HTML lines to add, so the
// common code-only document allocates nothing extra.
func nonInlineLines(f *File) map[int]struct{} {
	l0 := Layer0(f)
	hasHTML := false
	for _, span := range l0.BlockSpans {
		if span.Kind == BlockHTML {
			hasHTML = true
			break
		}
	}
	if len(l0.PIBlockLines) == 0 && !hasHTML {
		return l0.CodeBlockLines
	}
	set := make(map[int]struct{}, len(l0.CodeBlockLines)+len(l0.PIBlockLines))
	for ln := range l0.CodeBlockLines {
		set[ln] = struct{}{}
	}
	for ln := range l0.PIBlockLines {
		set[ln] = struct{}{}
	}
	for _, span := range l0.BlockSpans {
		if span.Kind != BlockHTML {
			continue
		}
		for ln := span.Start; ln <= span.End; ln++ {
			set[ln] = struct{}{}
		}
	}
	return set
}

// scanInlineBlocks groups the inline-bearing lines into runs and parses each
// run with the document references pre-seeded.
func scanInlineBlocks(f *File) []InlineBlock {
	if len(f.Source) == 0 {
		return nil
	}
	// A link reference definition always contains the literal `]:` (the
	// close-bracket of its label followed by the destination colon). When
	// the source has none, there are no definitions to seed and the
	// LinkReferences parse is pure waste, so the common reference-free file
	// skips it. This keeps the parse-skipped path parse-free for documents
	// without reference definitions; a file with `]:` pays one shared parse
	// (cached on the File and reused by every inline rule).
	var refs []Reference
	if bytes.Contains(f.Source, refDefMarker) {
		refs = f.LinkReferences()
	}
	skip := nonInlineLines(f)
	// One arena backs every run's parse for this file. Goldmark draws the
	// run's inline nodes from it; growing it across runs (never Reset) keeps
	// every earlier run's nodes valid while reusing slab memory instead of
	// allocating a fresh node pool per run. The arena outlives this scan via
	// the parsed nodes cached on the File, so it is per-file, not pooled.
	a := arena.New()
	var out []InlineBlock
	n := len(f.Lines)
	i := 0
	for i < n {
		if f.skipInlineLine(skip, i) {
			i++
			continue
		}
		runStart := i
		for i < n && !f.skipInlineLine(skip, i) {
			i++
		}
		start := f.LineStartOffset(runStart)
		end := f.lineEndOffset(i - 1)
		out = append(out, InlineBlock{
			Node:   parseInlineWithRefsArena(f.Source[start:end], refs, a),
			Offset: start,
		})
	}
	return out
}

// WalkInlineNodes drives visit over every node of every run in the shared
// run-grouped inline parse (InlineBlocks), in document order. base is the
// run's start byte offset in f.Source, which the visitor adds to a node's
// run-local segment offsets to recover document-absolute positions. It is
// the shared seam every inline rule's nil-AST Check uses so the
// parse-once-per-file projection drives each rule's per-node predicate
// without re-parsing.
func WalkInlineNodes(f *File, visit func(n ast.Node, base int)) {
	for _, blk := range InlineBlocks(f) {
		base := blk.Offset
		_ = ast.Walk(blk.Node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if entering {
				visit(n, base)
			}
			return ast.WalkContinue, nil
		})
	}
}

// skipInlineLine reports whether 0-based line i cannot start or continue an
// inline run: it is the trailing split artifact, a blank line, or a line in
// the skip set (code / PI / HTML-block content).
func (f *File) skipInlineLine(skip map[int]struct{}, i int) bool {
	if f.trailingEmptyLine(i) || isBlankLine(f.Lines[i]) {
		return true
	}
	_, ok := skip[i+1]
	return ok
}

// trailingEmptyLine reports whether 0-based line i is the trailing empty
// element bytes.Split appends for a Source ending in a newline. That element
// has no corresponding source line, so it never opens a run.
func (f *File) trailingEmptyLine(i int) bool {
	return i == len(f.Lines)-1 && len(f.Lines[i]) == 0
}

// parseInlineWithRefsArena parses block as a standalone Markdown document
// with the given link reference definitions pre-seeded into the parse
// context, so a reference-style link or image in block resolves against a
// definition that lives in another block of the document. The caller-owned
// arena lets consecutive run parses for one file reuse slab memory; it must
// outlive every node in the returned tree (the inline scan caches them on
// the File, so the arena lives with the File). The returned tree shares no
// state with the File.
func parseInlineWithRefsArena(block []byte, refs []Reference, a *arena.Arena) ast.Node {
	ctx := parser.NewContext()
	for _, ref := range refs {
		ctx.AddReference(ref)
	}
	return markdown.ParseContextArena(block, ctx, a)
}

// lineEndOffset returns the byte offset in Source of the newline that ends
// 0-based source line i (or len(Source) for the last line). It bounds an
// inline run's bytes: a run of 0-based lines [start, end] slices
// Source[LineStartOffset(start):lineEndOffset(end)]. i < 0 returns 0; i
// past the last line returns len(Source).
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
