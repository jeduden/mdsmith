package codeblockstyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Defensive guard: synthetic CodeBlock with zero segments ---
//
// Real goldmark output never produces a CodeBlock with zero segments —
// the parser always appends the line that opened the block. The
// indented-branch guard handles the synthetic shape, and this test
// drives it so the branch stays covered.

func TestCollectBlocks_SyntheticCodeBlock_NoSegments(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte(""))
	require.NoError(t, err)
	f.AST.AppendChild(f.AST, ast.NewCodeBlock())

	r := &Rule{Style: "fenced"}
	assert.Empty(t, r.Check(f))
}

// TestCollectBlocksL0_AllocBudget pins collectBlocksL0's per-call
// allocation count on the Layer 0 (nil-AST) path for a file with
// several fenced code blocks. len(lint.Layer0(f).BlockSpans) is a
// ready-made upper bound on the number of code blocks (BlockSpans
// covers every block kind, code blocks being a subset), known before
// the loop starts — the "pre-size slices" pattern in
// docs/development/high-performance-go.md. Without it, `blocks`
// starts nil and grows via unsized append, reallocating each time it
// outgrows its capacity.
func TestCollectBlocksL0_AllocBudget(t *testing.T) {
	src := []byte("```go\ncode1\n```\n\n```go\ncode2\n```\n\n```go\ncode3\n```\n\n```go\ncode4\n```\n")
	f := lint.NewFileLinesFromSource("test.md", src, false)
	require.Nil(t, f.AST, "collectBlocksL0 is the nil-AST path")

	allocs := testing.AllocsPerRun(200, func() {
		blocks := collectBlocksL0(f)
		if len(blocks) != 4 {
			t.Fatalf("unexpected block count: %d", len(blocks))
		}
	})
	if allocs > 1 {
		t.Fatalf("collectBlocksL0 allocs per call: want <= 1, got %v", allocs)
	}
}
