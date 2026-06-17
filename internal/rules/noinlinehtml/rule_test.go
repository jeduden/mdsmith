package noinlinehtml

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	goldmarktext "github.com/jeduden/mdsmith/pkg/goldmark/text"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRawHTMLNode() *ast.RawHTML {
	return ast.NewRawHTML()
}

// --- extractTag unit tests ---

func TestExtractTag_Opening(t *testing.T) {
	assert.Equal(t, "div", extractTag([]byte("<div>")))
}

func TestExtractTag_Closing(t *testing.T) {
	assert.Equal(t, "div", extractTag([]byte("</div>")))
}

func TestExtractTag_SelfClosing(t *testing.T) {
	assert.Equal(t, "br", extractTag([]byte("<br/>")))
	assert.Equal(t, "br", extractTag([]byte("<br />")))
	assert.Equal(t, "img", extractTag([]byte("<img src=\"x\"/>")))
}

func TestExtractTag_Uppercase(t *testing.T) {
	assert.Equal(t, "div", extractTag([]byte("<DIV>")))
}

func TestExtractTag_Hyphenated(t *testing.T) {
	assert.Equal(t, "my-tag", extractTag([]byte("<my-tag>")))
}

func TestExtractTag_Comment(t *testing.T) {
	assert.Equal(t, "<!--", extractTag([]byte("<!-- comment -->")))
	assert.Equal(t, "<!--", extractTag([]byte("<!--comment-->")))
}

func TestExtractTag_PI(t *testing.T) {
	assert.Equal(t, "", extractTag([]byte("<?include file: foo.md ?>")))
	assert.Equal(t, "", extractTag([]byte("<?catalog?>")))
}

func TestExtractTag_Malformed(t *testing.T) {
	assert.Equal(t, "", extractTag([]byte("<")))
	assert.Equal(t, "", extractTag(nil))
	assert.Equal(t, "", extractTag([]byte("")))
}

func TestExtractTag_WithAttributes(t *testing.T) {
	assert.Equal(t, "span", extractTag([]byte(`<span class="foo">`)))
}

// --- isClosingTag unit tests ---

func TestIsClosingTag_Closing(t *testing.T) {
	assert.True(t, isClosingTag([]byte("</div>")))
	assert.True(t, isClosingTag([]byte("</SPAN>")))
}

func TestIsClosingTag_Opening(t *testing.T) {
	assert.False(t, isClosingTag([]byte("<div>")))
}

func TestIsClosingTag_SelfClosing(t *testing.T) {
	assert.False(t, isClosingTag([]byte("<br/>")))
}

// --- Rule.Check integration tests ---

func newRule(t *testing.T, settings map[string]any) *Rule {
	t.Helper()
	r := &Rule{}
	require.NoError(t, r.ApplySettings(r.DefaultSettings()))
	if settings != nil {
		require.NoError(t, r.ApplySettings(settings))
	}
	return r
}

func parse(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func TestCheck_BlockHTML_EmitsDiag(t *testing.T) {
	r := newRule(t, nil)
	f := parse(t, "# Title\n\n<div>block</div>\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS041", diags[0].RuleID)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 1, diags[0].Column)
	assert.Equal(t, "inline HTML <div> is not allowed", diags[0].Message)
}

func TestCheck_IndentedBlockHTML_CorrectColumn(t *testing.T) {
	r := newRule(t, nil)
	// CommonMark allows up to 3 spaces of indentation for block HTML.
	// Column should point at '<', not at the indentation start.
	f := parse(t, "# Title\n\n   <div>indented</div>\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 4, diags[0].Column) // 3 spaces + '<' = column 4
}

func TestCheck_InlineHTML_EmitsDiag(t *testing.T) {
	r := newRule(t, nil)
	// "<span>" starts at column 6 ("text " = 5 chars)
	f := parse(t, "# Title\n\ntext <span>marked</span> text\n")
	diags := r.Check(f)
	require.Len(t, diags, 1, "closing tag must not produce extra diagnostic")
	assert.Equal(t, "inline HTML <span> is not allowed", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 6, diags[0].Column)
}

func TestCheck_SelfClosingBr_OneDiag(t *testing.T) {
	r := newRule(t, nil)
	// "<br/>" starts at column 5 ("text" = 4 chars)
	f := parse(t, "# Title\n\ntext<br/>more\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "inline HTML <br> is not allowed", diags[0].Message)
	assert.Equal(t, 5, diags[0].Column)
}

func TestCheck_AllowedTag_NoDiag(t *testing.T) {
	r := newRule(t, map[string]any{"allow": []any{"kbd"}})
	f := parse(t, "# Title\n\nPress <kbd>Enter</kbd>.\n")
	diags := r.Check(f)
	require.Len(t, diags, 0)
}

func TestCheck_AllowedTagCaseInsensitive(t *testing.T) {
	r := newRule(t, map[string]any{"allow": []any{"KBD"}})
	f := parse(t, "# Title\n\nPress <kbd>Enter</kbd>.\n")
	diags := r.Check(f)
	require.Len(t, diags, 0)
}

func TestCheck_Comment_AllowedByDefault(t *testing.T) {
	r := newRule(t, nil)
	f := parse(t, "# Title\n\n<!-- comment -->\n")
	diags := r.Check(f)
	require.Len(t, diags, 0)
}

func TestCheck_Comment_FlaggedWhenDisallowed(t *testing.T) {
	r := newRule(t, map[string]any{"allow-comments": false})
	f := parse(t, "# Title\n\n<!-- comment -->\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "inline HTML <!-- is not allowed", diags[0].Message)
	assert.Equal(t, 1, diags[0].Column)
}

func TestCheck_PIDirective_NoDiag(t *testing.T) {
	// Block PI directives become ProcessingInstruction nodes, not HTMLBlock.
	// This test verifies inline PI also produces no diagnostic.
	r := newRule(t, map[string]any{"allow-comments": false})
	// Inline PI in a paragraph becomes RawHTML in goldmark; must be skipped.
	f := parse(t, "# Title\n\nSee <?include file: foo.md ?> for details.\n")
	diags := r.Check(f)
	require.Len(t, diags, 0)
}

func TestCheck_Autolink_NoDiag(t *testing.T) {
	r := newRule(t, nil)
	f := parse(t, "# Title\n\nSee <https://example.com>.\n")
	diags := r.Check(f)
	require.Len(t, diags, 0)
}

func TestCheck_FencedCodeBlock_NoDiag(t *testing.T) {
	r := newRule(t, nil)
	f := parse(t, "# Title\n\n```html\n<div>hello</div>\n```\n")
	diags := r.Check(f)
	require.Len(t, diags, 0)
}

func TestCheck_DisabledByDefault(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault())
}

func TestMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS041", r.ID())
	assert.Equal(t, "no-inline-html", r.Name())
	assert.Equal(t, "structural", r.Category())
}

func TestCheck_NoHTML_NoDiag(t *testing.T) {
	r := newRule(t, nil)
	f := parse(t, "# Title\n\nJust plain text.\n")
	diags := r.Check(f)
	require.Len(t, diags, 0)
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestApplySettings_AllowBadType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"allow": "notalist"})
	require.Error(t, err)
}

func TestApplySettings_AllowCommentsBadType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"allow-comments": "yes"})
	require.Error(t, err)
}

// TestCachedAllowSet pins the per-Check memoization contract:
// subsequent calls on the same *lint.File return the same cached
// map (reference identity); a fresh *lint.File builds a separate
// map. Memoising via File.Memo keeps the cache off the shared
// rule instance (the LSP path reuses rule.All() across goroutines),
// so this also functions as a regression guard against the
// previous race-prone rule-level cache.
func TestCachedAllowSet(t *testing.T) {
	r := &Rule{Allow: []string{"Span", "div", "STRONG"}}
	f, err := lint.NewFile("t.md", []byte("# t\n"))
	require.NoError(t, err)

	first := r.cachedAllowSet(f)
	require.Equal(t, map[string]bool{"span": true, "div": true, "strong": true}, first,
		"lookup keys must be lowercase normalisations of r.Allow")

	second := r.cachedAllowSet(f)
	assert.Equal(t,
		reflect.ValueOf(first).Pointer(),
		reflect.ValueOf(second).Pointer(),
		"subsequent calls on the same File must return the same cached map")

	g, err := lint.NewFile("t.md", []byte("# t\n"))
	require.NoError(t, err)
	third := r.cachedAllowSet(g)
	assert.NotEqual(t,
		reflect.ValueOf(first).Pointer(),
		reflect.ValueOf(third).Pointer(),
		"a fresh File must build a separate cached map (memo is per-Check, not shared on the rule)")

	// An empty Allow yields a non-nil empty map.
	empty := &Rule{}
	h, err := lint.NewFile("t.md", []byte("# t\n"))
	require.NoError(t, err)
	got := empty.cachedAllowSet(h)
	require.NotNil(t, got, "empty Allow must still return a non-nil map")
	assert.Empty(t, got)
}

// TestRegisteredDefault_AllowCommentsTrue pins that the
// init()-registered rule instance carries AllowComments=true so
// that enabling the rule via the bare boolean form
// (`no-inline-html: true`) matches DefaultSettings's documented
// allow-comments default. ConfigureRule short-circuits when
// cfg.Settings is nil, so the registered instance is what runs.
func TestRegisteredDefault_AllowCommentsTrue(t *testing.T) {
	r := rule.ByID("MDS041")
	require.NotNil(t, r, "MDS041 must be registered")
	hr, ok := r.(*Rule)
	require.True(t, ok)
	assert.True(t, hr.AllowComments,
		"registered MDS041 must have AllowComments=true to match DefaultSettings")
}

func TestRawHTMLBytes_ZeroSegments(t *testing.T) {
	// A freshly allocated RawHTML node has no segments; rawHTMLBytes must
	// return nil rather than a non-nil empty slice.
	node := newRawHTMLNode()
	assert.Nil(t, rawHTMLBytes(node, []byte("source")))
}

func TestCheckNode_ZeroSegmentRawHTML_NoPanic(t *testing.T) {
	// A synthetic *ast.RawHTML with no segments must not panic in CheckNode.
	// Segments.At(0) would panic on the empty slice before rawHTMLBytes is
	// reached, so CheckNode must guard with Len() == 0 first.
	r := newRule(t, nil)
	f := parse(t, "# Title\n")
	node := newRawHTMLNode() // 0 segments
	assert.NotPanics(t, func() {
		diags := r.CheckNode(node, true, f)
		assert.Nil(t, diags)
	})
}

func TestRawHTMLBytes_ForceNewline(t *testing.T) {
	// When a segment carries ForceNewline=true, seg.Value() appends a trailing
	// '\n' that the capacity formula must account for. Verify that rawHTMLBytes
	// returns the full content including the forced newline without a panic or
	// truncation.
	source := []byte("<br>")
	node := newRawHTMLNode()
	seg := goldmarktext.Segment{Start: 0, Stop: 4, ForceNewline: true}
	node.Segments.Append(seg)
	got := rawHTMLBytes(node, source)
	assert.Equal(t, []byte("<br>\n"), got)
}

// TestCheck_NilASTMatchesAST pins the parse-skipped path byte-identical to
// the AST path over both HTML shapes: an HTML block, an inline span, a
// self-closing inline tag, an inline tag nested inside emphasis, and a
// comment. The nil-AST path reconstructs block HTML from the Layer 0 spans
// and inline HTML from the shared inline parse, so any divergence surfaces
// as a different diagnostic set.
func TestCheck_NilASTMatchesAST(t *testing.T) {
	cases := map[string]string{
		"html block":          "# Title\n\n<div>block content</div>\n",
		"inline span":         "Some text <span>x</span> tail\n",
		"self-closing inline": "before<br/>after\n",
		"inline in emphasis":  "a *strong <b>x</b>* tail\n",
		"comment block":       "# Title\n\n<!-- a comment -->\n",
		"clean prose":         "Just plain prose with no HTML.\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			r := &Rule{AllowComments: false}

			fAST, err := lint.NewFile("test.md", []byte(src))
			require.NoError(t, err)
			astDiags := r.Check(fAST)

			fNil, err := lint.NewFile("test.md", []byte(src))
			require.NoError(t, err)
			fNil.AST = nil
			nilDiags := r.Check(fNil)

			assert.Equal(t, astDiags, nilDiags)
		})
	}
}
