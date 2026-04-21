package ext

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// TestRewriteTextEmptyBody covers the early-return when the Text
// node's segment is empty.
func TestRewriteTextEmptyBody(t *testing.T) {
	tbl := abbrTable{"HTML": []byte("Hyper Text Markup Language")}
	src := []byte("")
	tn := ast.NewTextSegment(text.NewSegment(0, 0))
	assert.NotPanics(t, func() { rewriteText(tn, tbl, src) })
}

// TestRewriteTextOrphanParent covers the early-return when the Text
// has no parent (the rewrite pass has nowhere to insert siblings).
func TestRewriteTextOrphanParent(t *testing.T) {
	src := []byte("HTML")
	tbl := abbrTable{"HTML": []byte("Hyper Text Markup Language")}
	tn := ast.NewTextSegment(text.NewSegment(0, len(src)))
	assert.NotPanics(t, func() { rewriteText(tn, tbl, src) })
}

// TestBestMatchAtWordBoundaryRejectsSuffix exercises the
// "endIdx < len(body) && isWordByte(...)" rejection in bestMatchAt:
// a defined term that is a prefix of a longer word must not match.
func TestBestMatchAtWordBoundaryRejectsSuffix(t *testing.T) {
	tbl := abbrTable{"API": []byte("Application Programming Interface")}
	body := []byte("APIserver does things")
	_, ok := bestMatchAt(body, 0, tbl)
	assert.False(t, ok,
		"API followed by word byte 's' must not match")
}

// TestAbbreviationOpenEdgeCases exercises every rejection path in
// the block parser's Open method that the happy-path tests don't
// already hit.
func TestAbbreviationOpenEdgeCases(t *testing.T) {
	p := &abbreviationBlockParser{}

	cases := []struct {
		name string
		src  string
	}{
		{"empty", ""},
		{"four-space indent", "    *[X]: y\n"},
		{"not a definition prefix", "paragraph text\n"},
		{"missing closing bracket", "*[HTML\n"},
		{"missing colon", "*[HTML] hyper\n"},
		{"empty term not accepted", "*[]: something\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := text.NewReader([]byte(tc.src))
			pc := parser.NewContext()
			node, _ := p.Open(nil, r, pc)
			assert.Nil(t, node,
				"Open must reject %q (%s)", tc.src, tc.name)
		})
	}
}

// TestAbbreviationDefinitionWithEmptyExpansion verifies the parser
// accepts `*[TERM]:` (no expansion) and records an empty Expansion.
func TestAbbreviationDefinitionWithEmptyExpansion(t *testing.T) {
	src := "*[HTML]:\n\nUse HTML here.\n"
	doc := parseWith(t, src, Abbreviation)
	def := walkFindKind(doc, KindAbbreviationDefinition)
	if assert.NotNil(t, def, "accepts `*[TERM]:` with no expansion") {
		n := def.(*AbbreviationDefinition)
		assert.Equal(t, "HTML", string(n.Term))
		assert.Empty(t, n.Expansion)
	}
}

// TestAbbreviationTransformerNoTable skips the walk entirely when no
// definitions were found.
func TestAbbreviationTransformerNoTable(t *testing.T) {
	// A paragraph with no *[term] definition: the transformer's
	// `raw == nil` early return fires and nothing gets marked.
	src := "Just a paragraph with HTML mentioned.\n"
	doc := parseWith(t, src, Abbreviation)
	assert.Nil(t, walkFindKind(doc, KindAbbreviationReference))
	assert.Nil(t, walkFindKind(doc, KindAbbreviationDefinition))
}

// TestAbbreviationReferenceAtParagraphStart exercises rewriteText's
// `first.start == 0` branch: when a definition match is the first
// byte of a paragraph, the original Text node is replaced with the
// reference rather than being shrunk to a prefix.
func TestAbbreviationReferenceAtParagraphStart(t *testing.T) {
	src := "*[HTML]: Hyper Text Markup Language\n\nHTML is great.\n"
	doc := parseWith(t, src, Abbreviation)
	ref := walkFindKind(doc, KindAbbreviationReference)
	if assert.NotNil(t, ref) {
		assert.Equal(t, "HTML", string(ref.(*AbbreviationReference).Term))
	}
	// The paragraph should also carry the trailing " is great." as
	// a sibling text node; ensure the number of refs is 1 (no
	// runaway duplication).
	assert.Equal(t, 1, countKind(doc, KindAbbreviationReference))
}

// TestAbbreviationMultipleTermsInOneParagraph covers the
// appendRestAfter branch that emits a gap text between two
// consecutive matches.
func TestAbbreviationMultipleTermsInOneParagraph(t *testing.T) {
	src := "*[HTML]: Hyper Text Markup Language\n*[CSS]: Cascading Style Sheets\n\n" +
		"HTML and CSS together.\n"
	doc := parseWith(t, src, Abbreviation)
	assert.Equal(t, 2, countKind(doc, KindAbbreviationReference))
}

// TestAbbreviationTransformerEmptyTable covers the branch where the
// context key is set to a zero-length table (e.g. an upstream
// package interacting with the same key) — transformer must still
// early-return.
func TestAbbreviationTransformerEmptyTable(t *testing.T) {
	transformer := &abbreviationTransformer{}
	doc := parseWith(t, "# Heading\n\nParagraph.\n", Abbreviation)
	pc := parser.NewContext()
	pc.Set(abbrTableKey, abbrTable{})
	reader := text.NewReader([]byte("# Heading\n\nParagraph.\n"))
	assert.NotPanics(t, func() {
		transformer.Transform(doc.(*ast.Document), reader, pc)
	})
}
