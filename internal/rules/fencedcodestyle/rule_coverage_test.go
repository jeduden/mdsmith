package fencedcodestyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Category ---

func TestCategory(t *testing.T) {
	r := &Rule{Style: "backtick"}
	assert.Equal(t, "code", r.Category())
}

// --- Fix with leading spaces ---

func TestFix_LeadingSpaces(t *testing.T) {
	// Indent and info string survive; only the fence runs change.
	f, err := lint.NewFile("test.md", []byte("  ~~~go\nx\n  ~~~\n"))
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	assert.Equal(t, "  ```go\nx\n  ```\n", string(r.Fix(f)))
}

// --- Fix with empty block after paragraph (exercises previousSibling path) ---

func TestFix_EmptyTildeBlockAfterParagraph(t *testing.T) {
	src := []byte("paragraph\n\n~~~\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	assert.Equal(t, "paragraph\n\n```\n```\n", string(result))
}

// --- Defensive guards: synthetic FCB with no resolvable open fence ---
//
// Real goldmark output never produces a FencedCodeBlock without a
// matching `` ``` `` or `~~~` marker in the source, but Check and Fix
// keep defensive guards anyway. The tests below append synthetic
// FencedCodeBlocks without a parser position, so fencepos.OpenRun
// reads no fence run and the walker reaches the `fenceChar == 0` guard.

func newFileWithSyntheticFCB(t *testing.T, src []byte, fcb *ast.FencedCodeBlock) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	f.AST.AppendChild(f.AST, fcb)
	return f
}

func TestCheck_SyntheticFCB_OpenStartPastSource(t *testing.T) {
	// Source has no fence and the synthetic FCB has no position, so
	// OpenLineRange returns the (len(src), len(src)) sentinel and
	// OpenRun reads no run. Check skips the block silently.
	fcb := ast.NewFencedCodeBlock(nil)
	f := newFileWithSyntheticFCB(t, []byte(""), fcb)
	r := &Rule{Style: "backtick"}
	assert.Empty(t, r.Check(f))
}

func TestCheck_SyntheticFCB_NonFenceFirstChar(t *testing.T) {
	// A hand-built node has no parser position, so fencepos.OpenRun
	// reads no fence run. Check must hit the `fenceChar == 0` guard.
	src := []byte("hello\n")
	info := ast.NewText()
	info.Segment = text.NewSegment(0, 5)
	fcb := ast.NewFencedCodeBlock(info)
	f := newFileWithSyntheticFCB(t, src, fcb)
	r := &Rule{Style: "backtick"}
	assert.Empty(t, r.Check(f))
}

func TestFix_SyntheticFCB_OpenStartPastSource(t *testing.T) {
	// Same sentinel as the Check variant: Fix returns the source
	// unchanged because no fence range was collected.
	fcb := ast.NewFencedCodeBlock(nil)
	src := []byte("")
	f := newFileWithSyntheticFCB(t, src, fcb)
	r := &Rule{Style: "backtick"}
	assert.Equal(t, src, r.Fix(f))
}

func TestFix_SyntheticFCB_NonFenceFirstChar(t *testing.T) {
	src := []byte("hello\n")
	info := ast.NewText()
	info.Segment = text.NewSegment(0, 5)
	fcb := ast.NewFencedCodeBlock(info)
	f := newFileWithSyntheticFCB(t, src, fcb)
	r := &Rule{Style: "backtick"}
	assert.Equal(t, src, r.Fix(f))
}
