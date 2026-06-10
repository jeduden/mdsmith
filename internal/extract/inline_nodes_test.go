package extract

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// TestInlineSpanStringNode covers the *ast.String case: typographer and
// autolink transformers emit String nodes whose payload lives in Value, so
// the projector treats them as a text span. Bare String nodes almost never
// survive a normal parse, so the branch is exercised directly.
func TestInlineSpanStringNode(t *testing.T) {
	p := &projector{f: &lint.File{Path: "headline.md"}, sch: inlineScope()}
	got := p.inlineSpan("inline", ast.NewString([]byte("typeset")), false)
	assert.Equal(t, map[string]any{"span": "text", "value": "typeset"}, got)
	assert.Empty(t, p.diags)
}

// TestWalkInlineChildren_TextNodeContributesTextAndBreak covers the
// one-node-two-spans path directly: a Text child whose SoftLineBreak
// flag is set must contribute its text span followed by a `break`
// span, so the walker appends both rather than one.
func TestWalkInlineChildren_TextNodeContributesTextAndBreak(t *testing.T) {
	p := &projector{
		f:   &lint.File{Path: "headline.md", Source: []byte("first")},
		sch: inlineScope(),
	}
	parent := ast.NewParagraph()
	txt := ast.NewTextSegment(text.NewSegment(0, 5))
	txt.SetSoftLineBreak(true)
	parent.AppendChild(parent, txt)
	got := p.walkInlineChildren("inline", parent, false)
	require.Len(t, got, 2)
	assert.Equal(t, map[string]any{"span": "text", "value": "first"}, got[0])
	assert.Equal(t, map[string]any{"span": "break", "hard": false}, got[1])
	assert.Empty(t, p.diags)
}

// TestUnsupportedInlineDefault covers unsupportedInline's default branch: a
// node that is neither an image nor inline raw HTML is named by its Go type
// so a future custom inline node surfaces a clear diagnostic. The diagnostic
// also leads with the passed projection key (here a non-default "body") so
// the field tracks the real output key rather than the literal "inline".
func TestUnsupportedInlineDefault(t *testing.T) {
	p := &projector{f: &lint.File{Path: "headline.md"}, sch: inlineScope()}
	p.unsupportedInline("body", ast.NewText())
	require.Len(t, p.diags, 1)
	assert.Equal(t, "MDS020", p.diags[0].RuleID)
	assert.Contains(t, p.diags[0].Message, "unsupported inline node")
	assert.Contains(t, p.diags[0].Message, "ast.Text")
	assert.Truef(t, strings.HasPrefix(p.diags[0].Message, "body:"),
		"diagnostic should lead with the projection key, got %q", p.diags[0].Message)
}

// TestWalkInlineChildren_ChildlessContainerReturnsEmpty pins the
// null-vs-empty contract: a childless container (`[](u)`) must return
// the empty list, never nil, so its `children` key serialises as `[]`
// where the published `[...#Span]` constraint rejects `null`.
func TestWalkInlineChildren_ChildlessContainerReturnsEmpty(t *testing.T) {
	p := &projector{f: &lint.File{Path: "headline.md"}, sch: inlineScope()}
	got := p.walkInlineChildren("inline", ast.NewLink(), false)
	require.NotNil(t, got, "a nil slice would serialise to null")
	assert.Empty(t, got)
	assert.Empty(t, p.diags)
}
