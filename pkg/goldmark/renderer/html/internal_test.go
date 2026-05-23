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

func TestCSS3DraftSoftLineBreak_AllRules(t *testing.T) {
	// Drive each of the 4 rules of CSS3 Draft segment break
	// transformation.
	cases := []struct {
		name     string
		a, b     rune
		want     bool
	}{
		{"rule1-zwsp-before", '​', 'A', false},
		{"rule1-zwsp-after", 'A', '​', false},
		{"rule2-both-wide-non-hangul", 0x4E00, 0x4E01, false},
		{"rule2-wide-with-hangul", 0x1100, 0x4E00, true}, // Hangul + Wide -> preserve
		{"rule3-space-discarding", 0x3000, 'A', false},
		{"rule3-punct", '。', 'A', false},
		{"rule4-default", 'A', 'B', true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := eastAsianLineBreaksCSS3DraftSoftLineBreak(c.a, c.b); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestRenderImage_AllBranches(t *testing.T) {
	// renderImage has branches: dangerous URL (skip), Title set,
	// Attributes set, XHTML mode.  Construct AST manually.
	doc := ast.NewDocument()
	p := ast.NewParagraph()
	doc.AppendChild(doc, p)

	// Dangerous URL.
	dlink := ast.NewLink()
	dlink.Destination = []byte("javascript:alert(1)")
	dimg := ast.NewImage(dlink)
	p.AppendChild(p, dimg)

	// Image with title + attributes.
	titledLink := ast.NewLink()
	titledLink.Destination = []byte("/img.png")
	titledLink.Title = []byte("img-title")
	timg := ast.NewImage(titledLink)
	timg.SetAttribute([]byte("class"), []byte("img-cls"))
	p.AppendChild(p, timg)

	// Render with XHTML option (just toggling the renderer Config).
	r := NewRenderer().(*Renderer)
	r.Config.XHTML = true
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		_, _ = r.renderImage(bw, []byte("source"), c, true)
		_, _ = r.renderImage(bw, []byte("source"), c, false)
	}
	_ = bw.Flush()
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
