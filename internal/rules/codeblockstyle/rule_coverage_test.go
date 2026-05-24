package codeblockstyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
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
