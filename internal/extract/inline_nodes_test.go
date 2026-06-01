package extract

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// TestInlineSpanStringNode covers the *ast.String case: typographer and
// autolink transformers emit String nodes whose payload lives in Value, so
// the projector treats them as a text span. Bare String nodes almost never
// survive a normal parse, so the branch is exercised directly.
func TestInlineSpanStringNode(t *testing.T) {
	p := &projector{f: &lint.File{Path: "headline.md"}, sch: inlineScope()}
	got := p.inlineSpan(ast.NewString([]byte("typeset")))
	assert.Equal(t, map[string]any{"span": "text", "value": "typeset"}, got)
	assert.Empty(t, p.diags)
}

// TestUnsupportedInlineDefault covers unsupportedInline's default branch: a
// node that is neither an image nor inline raw HTML is named by its Go type
// so a future custom inline node surfaces a clear diagnostic.
func TestUnsupportedInlineDefault(t *testing.T) {
	p := &projector{f: &lint.File{Path: "headline.md"}, sch: inlineScope()}
	p.unsupportedInline(ast.NewText())
	require.Len(t, p.diags, 1)
	assert.Equal(t, "MDS020", p.diags[0].RuleID)
	assert.Contains(t, p.diags[0].Message, "unsupported inline node")
	assert.Contains(t, p.diags[0].Message, "ast.Text")
}
