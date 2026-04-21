package ext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMathInlineParses(t *testing.T) {
	doc := parseWith(t, "foo $x+1$ bar\n", MathInline)
	assert.NotNil(t, walkFindKind(doc, KindMathInline),
		"expected MathInline node for $x+1$")
}

func TestMathInlineParensWrapped(t *testing.T) {
	doc := parseWith(t, "area is ($x$)\n", MathInline)
	assert.NotNil(t, walkFindKind(doc, KindMathInline))
}

func TestMathInlineRejectsLeadingSpace(t *testing.T) {
	// Opening `$` must be followed by a non-space character.
	doc := parseWith(t, "$ x $\n", MathInline)
	assert.Nil(t, walkFindKind(doc, KindMathInline),
		"'$ x $' has space after opening $ — no match")
}

func TestMathInlineRejectsTrailingSpace(t *testing.T) {
	// Closing `$` must be preceded by a non-space character.
	doc := parseWith(t, "a $x $\n", MathInline)
	assert.Nil(t, walkFindKind(doc, KindMathInline))
}

func TestMathInlineRejectsDollarAmount(t *testing.T) {
	// Closing `$` must not be followed by a digit. This rejects
	// currency-style text like `$20$30`.
	doc := parseWith(t, "pay $20$30 dollars\n", MathInline)
	assert.Nil(t, walkFindKind(doc, KindMathInline),
		"digit after closing $ prevents the match")
}

func TestMathInlineUnbalanced(t *testing.T) {
	doc := parseWith(t, "this costs $5 maybe\n", MathInline)
	assert.Nil(t, walkFindKind(doc, KindMathInline))
}

func TestMathInlineInsideCodeIgnored(t *testing.T) {
	doc := parseWith(t, "see `$x+1$` here.\n", MathInline)
	assert.Nil(t, walkFindKind(doc, KindMathInline))
}

func TestMathInlineDoesNotMatchDoubleDollar(t *testing.T) {
	// `$$` is the start of a math block; the inline parser must not
	// fire on consecutive dollars.
	doc := parseWith(t, "before $$ not inline $$ after\n", MathInline)
	assert.Nil(t, walkFindKind(doc, KindMathInline))
}

// TestMathInlineSkipsDoubleDollarAsCloser exercises the Parse branch
// that rejects a candidate closing `$` when it is immediately
// followed by another `$` (i.e. a `$$` fence). The parser must look
// past the first `$$` and pair the opening `$` with a later, valid
// closing `$`.
func TestMathInlineSkipsDoubleDollarAsCloser(t *testing.T) {
	// The first candidate closer `$` at index after `x` is followed
	// by another `$`, so it is skipped; the next `$` after `y`
	// closes the span.
	doc := parseWith(t, "see $x$$y$ here\n", MathInline)
	assert.NotNil(t, walkFindKind(doc, KindMathInline))
}
