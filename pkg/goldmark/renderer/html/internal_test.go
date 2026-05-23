package html

// Internal unit tests for unexported helpers. Drives the method
// receivers that the public test files cannot reach because
// they live in package html_test.

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestSoftLineBreak_AllEnumValues(t *testing.T) {
	// softLineBreak's switch covers None, Simple, CSS3Draft, and
	// a default fallthrough that returns false. The caller in
	// renderText guards on `EastAsianLineBreaks != None` so the
	// None branch is unreachable through Convert, and the default
	// is unreachable for any valid enum value. A direct unit test
	// drives both.
	cases := []struct {
		mode EastAsianLineBreaks
		a, b rune
		want bool
	}{
		{EastAsianLineBreaksNone, 'A', 'B', false},
		{EastAsianLineBreaksSimple, 'A', 'B', true},                                 // narrow + narrow
		{EastAsianLineBreaksSimple, 0x4E00, 0x4E01, false},                          // wide + wide
		{EastAsianLineBreaksCSS3Draft, 'A', 'B', true},                              // Rule 4 default
		{EastAsianLineBreaks(99), 'A', 'B', false},                                  // default arm of switch
	}
	for _, c := range cases {
		if got := c.mode.softLineBreak(c.a, c.b); got != c.want {
			t.Errorf("softLineBreak(%d, %U, %U) = %v, want %v", c.mode, c.a, c.b, got, c.want)
		}
	}
}

func TestRenderTexts_AllChildTypes(t *testing.T) {
	// renderTexts dispatches on child type: ast.String,
	// ast.Text, otherwise recurses.  Construct a node tree with
	// all three child types and call renderTexts directly.
	r := NewRenderer().(*Renderer)
	parent := ast.NewParagraph()

	// ast.String child.
	parent.AppendChild(parent, ast.NewString([]byte("from-string")))
	// ast.Text child.
	parent.AppendChild(parent, ast.NewTextSegment(text.NewSegment(0, 5)))
	// Other type (Emphasis) -> recursive.
	em := ast.NewEmphasis(1)
	em.AppendChild(em, ast.NewString([]byte("nested")))
	parent.AppendChild(parent, em)

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	r.renderTexts(bw, []byte("source"), parent)
	_ = bw.Flush()
}
