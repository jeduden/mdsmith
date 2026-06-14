package markdown

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/arena"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/stretchr/testify/assert"
)

// countInlineNodes walks node and returns how many inline (TypeInline)
// nodes it contains — the nodes a block-only parse must not build.
func countInlineNodes(node ast.Node) int {
	n := 0
	_ = ast.Walk(node, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && c.Type() == ast.TypeInline {
			n++
		}
		return ast.WalkContinue, nil
	})
	return n
}

// TestParseBlockOnlyContextArena_SuppressesInline verifies the
// block-only parse builds the block tree with no inline children, yet
// still records link reference definitions (collected by the paragraph
// transformer during block close, not by the skipped inline phase).
func TestParseBlockOnlyContextArena_SuppressesInline(t *testing.T) {
	src := []byte("# Heading\n\nPara with [a](u) and `code`.\n\n[a]: http://example.com\n")

	blockCtx := parser.NewContext()
	block := ParseBlockOnlyContextArena(src, blockCtx, arena.New())
	assert.Equal(t, ast.KindDocument, block.Kind())
	assert.Zero(t, countInlineNodes(block), "block-only parse must build no inline nodes")
	assert.NotEmpty(t, blockCtx.References(),
		"block-only parse still records link reference definitions")

	// Contrast: the full parse over the same source DOES build inline nodes.
	full := ParseContextArena(src, parser.NewContext(), arena.New())
	assert.Positive(t, countInlineNodes(full), "full parse builds inline nodes")
}
