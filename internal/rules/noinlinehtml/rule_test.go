package noinlinehtml

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper creates a Rule with optional settings applied.
func newRule(settings map[string]any) *Rule {
	r := &Rule{AllowComments: true}
	if len(settings) > 0 {
		if err := r.ApplySettings(settings); err != nil {
			panic(err)
		}
	}
	return r
}

// helper parses markdown and runs Check.
func check(t *testing.T, src string, settings map[string]any) []lint.Diagnostic {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	r := newRule(settings)
	return r.Check(f)
}

// --- Tag extraction helper tests ---

func TestExtractTagName_Opening(t *testing.T) {
	assert.Equal(t, "div", extractTagName([]byte("<div>")))
}

func TestExtractTagName_Closing(t *testing.T) {
	assert.Equal(t, "div", extractTagName([]byte("</div>")))
}

func TestExtractTagName_SelfClosing(t *testing.T) {
	assert.Equal(t, "br", extractTagName([]byte("<br/>")))
}

func TestExtractTagName_Uppercase(t *testing.T) {
	assert.Equal(t, "span", extractTagName([]byte("<SPAN>")))
}

func TestExtractTagName_Hyphenated(t *testing.T) {
	assert.Equal(t, "my-tag", extractTagName([]byte("<my-tag>")))
}

func TestExtractTagName_Comment(t *testing.T) {
	assert.Equal(t, "<!--", extractTagName([]byte("<!-- comment -->")))
}

func TestExtractTagName_Malformed(t *testing.T) {
	assert.Equal(t, "", extractTagName([]byte("<123invalid>")))
}

func TestExtractTagName_Empty(t *testing.T) {
	assert.Equal(t, "", extractTagName([]byte("")))
}

func TestExtractTagName_Directive(t *testing.T) {
	// <?...?> directives should not be processed by tag extraction.
	// The rule skips them before reaching extractTagName.
	assert.Equal(t, "", extractTagName([]byte("<?foo?>")))
}

func TestExtractTagName_WithAttributes(t *testing.T) {
	assert.Equal(t, "div", extractTagName([]byte(`<div class="foo">`)))
}

// --- Rule metadata tests ---

func TestRule_ID(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS041", r.ID())
}

func TestRule_Name(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "no-inline-html", r.Name())
}

func TestRule_Category(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "meta", r.Category())
}

func TestRule_EnabledByDefault(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault())
}

// --- Block HTML tests ---

func TestCheck_BlockDiv(t *testing.T) {
	diags := check(t, "# Title\n\n<div>content</div>\n", nil)
	require.Len(t, diags, 1)
	assert.Equal(t, "inline HTML <div> is not allowed", diags[0].Message)
	assert.Equal(t, "MDS041", diags[0].RuleID)
}

func TestCheck_BlockDivLineNumber(t *testing.T) {
	diags := check(t, "# Title\n\n<div>content</div>\n", nil)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 1, diags[0].Column)
}

func TestCheck_BlockDivOnlyOneForOpenClose(t *testing.T) {
	// <div>...</div> block emits one diagnostic for the opening tag
	// (goldmark groups it as one HTMLBlock node).
	diags := check(t, "# Title\n\n<div>\ncontent\n</div>\n", nil)
	require.Len(t, diags, 1)
	assert.Equal(t, "inline HTML <div> is not allowed", diags[0].Message)
}

// --- Inline HTML tests ---

func TestCheck_InlineSpan(t *testing.T) {
	diags := check(t, "# Title\n\ntext <span>marked</span> text\n", nil)
	require.Len(t, diags, 1)
	assert.Equal(t, "inline HTML <span> is not allowed", diags[0].Message)
}

func TestCheck_InlineBr(t *testing.T) {
	diags := check(t, "# Title\n\nline break<br>\n", nil)
	require.Len(t, diags, 1)
	assert.Equal(t, "inline HTML <br> is not allowed", diags[0].Message)
}

func TestCheck_InlineBrSelfClosing(t *testing.T) {
	diags := check(t, "# Title\n\nline break<br/>\n", nil)
	require.Len(t, diags, 1)
	assert.Equal(t, "inline HTML <br> is not allowed", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 11, diags[0].Column)
}

// --- Comment tests ---

func TestCheck_CommentAllowed(t *testing.T) {
	// By default allow-comments: true
	diags := check(t, "# Title\n\n<!-- comment -->\n", nil)
	assert.Empty(t, diags)
}

func TestCheck_CommentFlagged(t *testing.T) {
	diags := check(t, "# Title\n\n<!-- comment -->\n",
		map[string]any{"allow-comments": false})
	require.Len(t, diags, 1)
	assert.Equal(t, "inline HTML <!-- is not allowed", diags[0].Message)
}

// --- Allowlist tests ---

func TestCheck_AllowedTag(t *testing.T) {
	diags := check(t, "# Title\n\n<kbd>Ctrl</kbd>\n",
		map[string]any{"allow": []string{"kbd"}})
	assert.Empty(t, diags)
}

func TestCheck_AllowedTagCaseInsensitive(t *testing.T) {
	// Tag in document is uppercase, allow list is lowercase
	diags := check(t, "# Title\n\n<KBD>Ctrl</KBD>\n",
		map[string]any{"allow": []string{"kbd"}})
	assert.Empty(t, diags)
}

func TestCheck_NotAllowedTag(t *testing.T) {
	diags := check(t, "# Title\n\n<div>content</div>\n",
		map[string]any{"allow": []string{"span"}})
	require.Len(t, diags, 1)
	assert.Equal(t, "inline HTML <div> is not allowed", diags[0].Message)
}

// --- Directive skip tests ---

func TestCheck_InlineDirectiveSkipped(t *testing.T) {
	// Inline <?...?> directives should not be flagged
	diags := check(t, "# Title\n\ntext <?foo?> text\n", nil)
	assert.Empty(t, diags)
}

// --- No-flag contexts ---

func TestCheck_FencedCodeBlock(t *testing.T) {
	diags := check(t, "# Title\n\n```html\n<div>content</div>\n```\n", nil)
	assert.Empty(t, diags)
}

func TestCheck_InlineCode(t *testing.T) {
	diags := check(t, "# Title\n\nUse `<div>` element\n", nil)
	assert.Empty(t, diags)
}

func TestCheck_Autolink(t *testing.T) {
	diags := check(t, "# Title\n\n<https://example.com>\n", nil)
	assert.Empty(t, diags)
}

func TestCheck_EmptyFile(t *testing.T) {
	diags := check(t, "", nil)
	assert.Empty(t, diags)
}

func TestCheck_PlainText(t *testing.T) {
	diags := check(t, "# Title\n\nJust plain text with no HTML.\n", nil)
	assert.Empty(t, diags)
}

// --- Default settings tests ---

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	assert.Equal(t, []string{}, ds["allow"])
	assert.Equal(t, true, ds["allow-comments"])
}

// --- ApplySettings tests ---

func TestApplySettings_Allow(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"allow": []string{"kbd", "sub"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"kbd", "sub"}, r.Allow)
}

func TestApplySettings_AllowCommentsFalse(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"allow-comments": false})
	require.NoError(t, err)
	assert.False(t, r.AllowComments)
}

func TestApplySettings_AllowCommentsTrue(t *testing.T) {
	r := &Rule{AllowComments: false}
	err := r.ApplySettings(map[string]any{"allow-comments": true})
	require.NoError(t, err)
	assert.True(t, r.AllowComments)
}

func TestApplySettings_InvalidAllow(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"allow": "not-a-list"})
	assert.Error(t, err)
}

func TestApplySettings_InvalidAllowComments(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"allow-comments": "yes"})
	assert.Error(t, err)
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown-key": "value"})
	assert.Error(t, err)
}

// --- SettingMergeMode tests ---

func TestSettingMergeMode_Allow(t *testing.T) {
	r := &Rule{}
	// allow is replace-mode per the plan
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("allow"))
}

func TestSettingMergeMode_AllowComments(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("allow-comments"))
}
