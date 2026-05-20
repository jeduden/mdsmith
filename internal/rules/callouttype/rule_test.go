package callouttype

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
)

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func TestRuleMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS067", r.ID())
	assert.Equal(t, "callout-type", r.Name())
	assert.Equal(t, "structural", r.Category())
	assert.False(t, r.EnabledByDefault())
}

func TestCheck_BaseTypeAllowed(t *testing.T) {
	r := &Rule{}
	for _, ty := range []string{
		"note", "abstract", "summary", "tldr",
		"info", "todo",
		"tip", "hint", "important",
		"success", "check", "done",
		"question", "help", "faq",
		"warning", "caution", "attention",
		"failure", "fail", "missing",
		"danger", "error",
		"bug", "example",
		"quote", "cite",
	} {
		src := "> [!" + ty + "]\n> body\n"
		f := newFile(t, src)
		diags := r.Check(f)
		assert.Emptyf(t, diags, "type %q should be allowed", ty)
	}
}

func TestCheck_CaseInsensitive(t *testing.T) {
	f := newFile(t, "> [!NOTE]\n> body\n")
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_UnknownType(t *testing.T) {
	f := newFile(t, "> [!REVIEW]\n> body\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS067", diags[0].RuleID)
	assert.Contains(t, diags[0].Message, "REVIEW")
	assert.Contains(t, diags[0].Message, "note")
	assert.Contains(t, diags[0].Message, "allow-unknown")
}

func TestCheck_AllowList(t *testing.T) {
	f := newFile(t, "> [!custom]\n> body\n")
	r := &Rule{Allow: []string{"custom"}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_AllowUnknownDisablesValidation(t *testing.T) {
	f := newFile(t, "> [!anything]\n> body\n")
	r := &Rule{AllowUnknown: true}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_PlainBlockquoteIgnored(t *testing.T) {
	f := newFile(t, "> just a quote\n> no callout marker here\n")
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_ReportsLineAndColumn(t *testing.T) {
	f := newFile(t, "# Heading\n\n> [!REVIEW]\n> body\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	// The "[" sits after "> " on the line, so col 3.
	assert.Equal(t, 3, diags[0].Column)
}

func TestCheck_NestedBlockquote(t *testing.T) {
	// A blockquote whose first paragraph does not start with `[!type]`
	// is not a callout — even if a deeper child contains the token.
	f := newFile(t, "> not a callout\n>\n> > [!REVIEW]\n> > body\n")
	diags := (&Rule{}).Check(f)
	// The inner blockquote is itself walked; if its first paragraph
	// holds the unknown token it still triggers.
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "REVIEW")
}

func TestApplySettings_AllowList(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"allow": []any{"custom", "other"}}))
	assert.Equal(t, []string{"custom", "other"}, r.Allow)
}

func TestApplySettings_AllowUnknown(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"allow-unknown": true}))
	assert.True(t, r.AllowUnknown)
}

func TestApplySettings_BadTypes(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"allow": "string-not-list"})
	require.Error(t, err)

	err = r.ApplySettings(map[string]any{"allow-unknown": "yes"})
	require.Error(t, err)

	err = r.ApplySettings(map[string]any{"unknown-key": true})
	require.Error(t, err)
}

func TestDefaultSettings(t *testing.T) {
	ds := (&Rule{}).DefaultSettings()
	assert.Equal(t, []string{}, ds["allow"])
	assert.Equal(t, false, ds["allow-unknown"])
}

func TestSettingMergeMode(t *testing.T) {
	r := &Rule{}
	assert.NotEqual(t, r.SettingMergeMode("allow"), r.SettingMergeMode("allow-unknown"))
}
