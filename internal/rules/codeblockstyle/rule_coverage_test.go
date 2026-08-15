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
// several fenced code blocks. len(lint.Layer0(f).BlockSpans) is NOT
// a tight bound to presize against — BlockSpans covers every block
// kind (headings, paragraphs, lists, quotes, ...), so presizing to
// its length wastes memory on the common case (a mostly-prose file
// with few or no code blocks: verified on this repo's own L0-eligible
// corpus, most files have zero code blocks against dozens of other
// spans). collectBlocksL0 instead counts the actual code-block spans
// first — a cheap second switch-only pass over the same slice — and
// presizes to that. This test pins the presized case; the
// zero-code-block case below pins that the common case still
// allocates nothing.
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

// TestCollectBlocksL0_NoCodeBlocks_ZeroAllocs pins the common case:
// a prose file with headings, paragraphs, and lists but no code
// blocks at all. A naive make([]blockInfo, 0, len(spans)) presize
// would allocate here even though the result is always empty — this
// is the regression an xhigh-severity review pass on PR #785 measured
// (a ~14x slowdown and an 8KB allocation on a 351-line, 200-span,
// zero-code-block file) against the naive version of this fix.
func TestCollectBlocksL0_NoCodeBlocks_ZeroAllocs(t *testing.T) {
	src := []byte("# Heading\n\nSome prose paragraph here.\n\n" +
		"## Sub heading\n\n- a list item\n- another item\n\n> a blockquote\n")
	f := lint.NewFileLinesFromSource("test.md", src, false)
	require.Nil(t, f.AST, "collectBlocksL0 is the nil-AST path")
	require.NotEmpty(t, lint.Layer0(f).BlockSpans, "fixture must have non-code spans to exercise the over-count risk")

	allocs := testing.AllocsPerRun(200, func() {
		blocks := collectBlocksL0(f)
		if blocks != nil {
			t.Fatalf("expected nil for a file with no code blocks, got %v", blocks)
		}
	})
	if allocs > 0 {
		t.Fatalf("collectBlocksL0 allocs per call for a code-block-free file: want 0, got %v", allocs)
	}
}
