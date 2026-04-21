package ext

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// TestMathBlockOpenRejectsShortLine covers the `line == nil` early-
// return path in Open, and the three-space indent limit.
func TestMathBlockOpenEdgeCases(t *testing.T) {
	p := &mathBlockParser{}
	pc := parser.NewContext()

	// An empty document: PeekLine returns nil → Open rejects.
	empty := text.NewReader([]byte(""))
	node, _ := p.Open(nil, empty, pc)
	assert.Nil(t, node, "Open on empty input must not open a block")

	// Four-space indent: Open rejects even though the line starts
	// with `$$` further in.
	indented := text.NewReader([]byte("    $$\n    x\n    $$\n"))
	node, _ = p.Open(nil, indented, pc)
	assert.Nil(t, node, "Open must not open on four-space-indented line")

	// Line that shares the `$` trigger but isn't a fence.
	notFence := text.NewReader([]byte("$price\n"))
	node, _ = p.Open(nil, notFence, pc)
	assert.Nil(t, node, "Open must reject non-fence `$` lines")
}

// TestMathBlockContinueEOF covers the `line == nil` path in Continue:
// the reader reaches EOF while the block is still open.
func TestMathBlockContinueEOF(t *testing.T) {
	src := "$$\na^2 + b^2\n"
	doc := parseWith(t, src, MathBlock)
	n := walkFindKind(doc, KindMathBlock)
	if assert.NotNil(t, n) {
		mb := n.(*MathBlockNode)
		assert.False(t, mb.HasClosure(),
			"unclosed block must carry closed=false")
	}
}
