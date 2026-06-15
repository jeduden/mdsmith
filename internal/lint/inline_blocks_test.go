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
