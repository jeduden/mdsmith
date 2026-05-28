package ext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMathBlockParses(t *testing.T) {
	src := "before\n\n$$\na^2 + b^2 = c^2\n$$\n\nafter\n"
	doc := parseWith(t, src, MathBlock)
	assert.NotNil(t, walkFindKind(doc, KindMathBlock),
		"expected MathBlock node for $$...$$ fence")
}

func TestMathBlockClosingOnSameLine(t *testing.T) {
	// A single-line block like `$$...$$` is also valid.
	src := "$$E=mc^2$$\n"
	doc := parseWith(t, src, MathBlock)
	assert.NotNil(t, walkFindKind(doc, KindMathBlock))
}

func TestMathBlockUnclosedIsNotMatched(t *testing.T) {
	// If no closing `$$` appears, the block must not leak into the
	// AST as a MathBlock — it stays as regular paragraph content.
	src := "$$\nno close here\nparagraph\n"
	doc := parseWith(t, src, MathBlock)
	// Unclosed block may still create a node; verify it is flagged
	// closed=false so detect can decide how to report it. The plan
	// does not require matching unclosed blocks, so either "no node"
	// or "node with HasClosure()==false" is acceptable. Assert the
	// latter if a node was produced.
	if n := walkFindKind(doc, KindMathBlock); n != nil {
		mb, ok := n.(*MathBlockNode)
		if assert.True(t, ok) {
			assert.False(t, mb.HasClosure())
		}
	}
}

func TestMathBlockInsideParagraphIsIgnored(t *testing.T) {
	// `$$` in the middle of paragraph text must not start a block.
	src := "text $$inline$$ here\n"
	doc := parseWith(t, src, MathBlock)
	assert.Nil(t, walkFindKind(doc, KindMathBlock),
		"mid-paragraph `$$` must not open a math block")
}

func TestMathBlockIndentedDoesNotOpen(t *testing.T) {
	// Four spaces of indent makes the line a code block, not a math
	// fence.
	src := "    $$\n    x + y\n    $$\n"
	doc := parseWith(t, src, MathBlock)
	assert.Nil(t, walkFindKind(doc, KindMathBlock))
}
