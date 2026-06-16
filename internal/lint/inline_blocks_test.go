package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/pkg/goldmark/arena"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
)

// TestInlineBlocks_RunGrouping pins that contiguous inline-bearing lines are
// grouped into one run (so a construct wrapping onto a continuation line
// stays whole) and that blank lines split runs.
func TestInlineBlocks_RunGrouping(t *testing.T) {
	f := NewFileLines("doc.md", []byte("para one\nstill one\n\npara two\n"))
	blocks := InlineBlocks(f)
	require.Len(t, blocks, 2, "blank line splits into two runs")
	assert.Equal(t, 0, blocks[0].Offset)
	// The second run starts after "para one\nstill one\n\n".
	assert.Equal(t, len("para one\nstill one\n\n"), blocks[1].Offset)
}

// TestInlineBlocks_Memoized pins one scan per file.
func TestInlineBlocks_Memoized(t *testing.T) {
	f := NewFileLines("doc.md", []byte("a paragraph\n"))
	first := InlineBlocks(f)
	second := InlineBlocks(f)
	require.Len(t, first, 1)
	assert.Same(t, &first[0], &second[0], "InlineBlocks is cached per File")
}

// TestInlineBlocks_EmptySource returns nil for a struct-literal File.
func TestInlineBlocks_EmptySource(t *testing.T) {
	assert.Nil(t, InlineBlocks(&File{}))
}

// TestParseInlineWithRefs_ResolvesCrossBlockReference pins that a
// reference-style link is reconstructed as a Link node when its definition
// is seeded from another block — the property that lets the per-block parse
// match the whole-document parse on cross-block references.
func TestParseInlineWithRefs_ResolvesCrossBlockReference(t *testing.T) {
	refs := []Reference{parser.NewReference([]byte("ref"), []byte("http://example.com"), nil)}
	doc := parseInlineWithRefsArena([]byte("[text][ref]"), refs, arena.New())
	found := false
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if _, ok := n.(*ast.Link); ok {
				found = true
			}
		}
		return ast.WalkContinue, nil
	})
	assert.True(t, found, "seeded reference resolves [text][ref] to a Link node")

	// Without the seed the same source has no Link node — it degrades to text.
	none := parseInlineWithRefsArena([]byte("[text][ref]"), nil, arena.New())
	hasLink := false
	_ = ast.Walk(none, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if _, ok := n.(*ast.Link); ok {
				hasLink = true
			}
		}
		return ast.WalkContinue, nil
	})
	assert.False(t, hasLink, "unseeded reference leaves no Link node")
}

// TestWalkInlineNodes_OffsetMapping pins that the base offset WalkInlineNodes
// hands the visitor maps a run-local Text segment back to its
// document-absolute bytes.
func TestWalkInlineNodes_OffsetMapping(t *testing.T) {
	src := []byte("first\n\nhttp://example.com\n")
	f := NewFileLines("doc.md", src)
	var gotText string
	WalkInlineNodes(f, func(n ast.Node, base int) {
		if tn, ok := n.(*ast.Text); ok {
			seg := tn.Segment
			gotText += string(src[base+seg.Start : base+seg.Stop])
		}
	})
	assert.Contains(t, gotText, "http://example.com",
		"base+segment offsets recover the document bytes")
}

// TestLineEndOffset_NegativeIndex covers the i < 0 guard: a negative line
// index must return 0.
func TestLineEndOffset_NegativeIndex(t *testing.T) {
	f := NewFileLines("doc.md", []byte("hello\nworld\n"))
	assert.Equal(t, 0, f.lineEndOffset(-1))
}

// TestLineEndOffset_PastEnd covers the i >= len(nl) guard: an index past the
// last line must return len(Source).
func TestLineEndOffset_PastEnd(t *testing.T) {
	src := []byte("hello\nworld\n")
	f := NewFileLines("doc.md", src)
	assert.Equal(t, len(src), f.lineEndOffset(999))
}

// TestInlineBlocks_RefDefGate pins the `]:` short-circuit in
// scanInlineBlocks: a source carrying a reference definition resolves a
// cross-block reference link to a Link node (the seed fired), while a
// reference-free source still parses but leaves a bare `[text][ref]` as
// plain text (no seed, no Link).
func TestInlineBlocks_RefDefGate(t *testing.T) {
	countLinks := func(blocks []InlineBlock) int {
		n := 0
		for _, b := range blocks {
			_ = ast.Walk(b.Node, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
				if entering {
					if _, ok := node.(*ast.Link); ok {
						n++
					}
				}
				return ast.WalkContinue, nil
			})
		}
		return n
	}

	withDef := NewFileLines("doc.md", []byte("[text][ref]\n\n[ref]: http://example.com\n"))
	assert.Equal(t, 1, countLinks(InlineBlocks(withDef)),
		"a `]:` definition seeds the reference so [text][ref] resolves to a Link")

	noDef := NewFileLines("doc.md", []byte("[text][ref] and more text here\n"))
	assert.Equal(t, 0, countLinks(InlineBlocks(noDef)),
		"no `]:` definition leaves [text][ref] as unresolved plain text")
}

// TestNonInlineLines_CodeBlockLinesBody covers the body of the
// `for ln := range l0.CodeBlockLines` loop (inside the merge path): it runs
// only when hasHTML || len(PIBlockLines) > 0. An HTML block plus a fenced
// code block satisfies hasHTML AND provides CodeBlockLines, so the loop body
// executes and the code-block line numbers are merged into the set.
func TestNonInlineLines_CodeBlockLinesBody(t *testing.T) {
	// Line 1: HTML block open, blank line 2, fenced code lines 3-5.
	src := []byte("<div>\n\n```\ncode\n```\n")
	f := NewFileLines("doc.md", src)
	set := nonInlineLines(f)
	require.NotNil(t, set)
	found := false
	for _, ln := range []int{3, 4, 5} {
		if _, ok := set[ln]; ok {
			found = true
			break
		}
	}
	assert.True(t, found,
		"merged set must contain code-block line numbers when an HTML block is present")
}

// TestNonInlineLines_PIBlockLinesBody covers the body of the
// `for ln := range l0.PIBlockLines` loop: it runs when PIBlockLines is
// non-empty. A PI block (`<?…?>`) is the minimal trigger.
func TestNonInlineLines_PIBlockLinesBody(t *testing.T) {
	src := []byte("<?foo\nbar\n?>\n")
	f := NewFileLines("doc.md", src)
	set := nonInlineLines(f)
	require.NotNil(t, set)
	found := false
	for _, ln := range []int{1, 2, 3} {
		if _, ok := set[ln]; ok {
			found = true
			break
		}
	}
	assert.True(t, found, "merged set must contain PI-block line numbers")
}

// TestNonInlineLines_CodeOnlyNoCopy pins the no-extra-allocation fast path:
// a document with code blocks but no PI or HTML returns the Layer 0
// CodeBlockLines map directly.
func TestNonInlineLines_CodeOnlyNoCopy(t *testing.T) {
	f := NewFileLines("doc.md", []byte("```\ncode\n```\n"))
	set := nonInlineLines(f)
	assert.Equal(t, Layer0(f).CodeBlockLines, set)
}
