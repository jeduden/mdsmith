package noinlinehtml

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Equal(t, "inline HTML <div> is not allowed", diags[0].Message)
}

func TestCheck_InlineHTML_EmitsDiag(t *testing.T) {
	r := newRule(t, nil)
	f := parse(t, "# Title\n\ntext <span>marked</span> text\n")
	diags := r.Check(f)
	require.Len(t, diags, 1, "closing tag must not produce extra diagnostic")
	assert.Equal(t, "span", extractTag([]byte("<span>")))
	assert.Equal(t, "inline HTML <span> is not allowed", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
}

func TestCheck_SelfClosingBr_OneDiag(t *testing.T) {
	r := newRule(t, nil)
	f := parse(t, "# Title\n\ntext<br/>more\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "inline HTML <br> is not allowed", diags[0].Message)
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
	assert.Equal(t, "inline HTML <<!--> is not allowed", diags[0].Message)
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
