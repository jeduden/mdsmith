package listmarkerstyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuleMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS045", r.ID())
	assert.Equal(t, "list-marker-style", r.Name())
	assert.Equal(t, "list", r.Category())
	assert.False(t, r.EnabledByDefault(), "rule must be opt-in")
}

func TestCheck_DashStyle_GoodList(t *testing.T) {
	src := []byte("- item one\n- item two\n- item three\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_DashStyle_FlagsAsterisk(t *testing.T) {
	src := []byte("* item one\n* item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash}
	diags := r.Check(f)
	require.Len(t, diags, 2, "one diagnostic per mismatching item")
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 2, diags[1].Line)
	assert.Contains(t, diags[0].Message, "uses asterisk")
	assert.Contains(t, diags[0].Message, "configured style is dash")
}

func TestCheck_DashStyle_FlagsPlus(t *testing.T) {
	src := []byte("+ item one\n+ item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash}
	diags := r.Check(f)
	require.Len(t, diags, 2, "one diagnostic per mismatching item")
	assert.Contains(t, diags[0].Message, "uses plus")
	assert.Contains(t, diags[0].Message, "configured style is dash")
}

func TestCheck_AsteriskStyle_GoodList(t *testing.T) {
	src := []byte("* item one\n* item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleAsterisk}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_PlusStyle_GoodList(t *testing.T) {
	src := []byte("+ item one\n+ item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StylePlus}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_OrderedListNotFlagged(t *testing.T) {
	src := []byte("1. item one\n2. item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_NestedList_NoNestedConfig(t *testing.T) {
	src := []byte("- outer\n  - inner\n  - inner two\n- outer two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash}
	diags := r.Check(f)
	assert.Empty(t, diags, "both lists use dash, should pass")
}

func TestCheck_NestedList_WithNestedConfig_Good(t *testing.T) {
	src := []byte("- outer\n  * inner\n  * inner two\n- outer two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash, Nested: []string{StyleDash, StyleAsterisk}}
	diags := r.Check(f)
	assert.Empty(t, diags, "outer uses dash, inner uses asterisk as configured")
}

func TestCheck_NestedList_WithNestedConfig_Bad(t *testing.T) {
	src := []byte("- outer\n  - inner\n  - inner two\n- outer two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash, Nested: []string{StyleDash, StyleAsterisk}}
	diags := r.Check(f)
	require.Len(t, diags, 2, "one diagnostic per mismatching inner item")
	assert.Equal(t, 2, diags[0].Line)
	assert.Equal(t, 3, diags[1].Line)
	assert.Contains(t, diags[0].Message, "depth 1")
	assert.Contains(t, diags[0].Message, "uses dash")
	assert.Contains(t, diags[0].Message, "expected asterisk")
}

func TestCheck_DeeplyNestedList_CyclesNestedConfig(t *testing.T) {
	src := []byte("- depth 0\n  * depth 1\n    - depth 2\n      * depth 3\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Nested: []string{StyleDash, StyleAsterisk}}
	diags := r.Check(f)
	assert.Empty(t, diags, "depth 0,2 use dash; depth 1,3 use asterisk")
}

func TestFix_AsteriskToDash(t *testing.T) {
	src := []byte("* item one\n* item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash}
	got := r.Fix(f)
	want := "- item one\n- item two\n"
	assert.Equal(t, want, string(got))
}

func TestFix_PlusToDash(t *testing.T) {
	src := []byte("+ item one\n+ item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash}
	got := r.Fix(f)
	want := "- item one\n- item two\n"
	assert.Equal(t, want, string(got))
}

func TestFix_DashToAsterisk(t *testing.T) {
	src := []byte("- item one\n- item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleAsterisk}
	got := r.Fix(f)
	want := "* item one\n* item two\n"
	assert.Equal(t, want, string(got))
}

func TestFix_WithIndentation(t *testing.T) {
	src := []byte("* item one\n  * nested\n* item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash}
	got := r.Fix(f)
	want := "- item one\n  - nested\n- item two\n"
	assert.Equal(t, want, string(got))
}

func TestFix_NestedWithConfig(t *testing.T) {
	src := []byte("- outer\n  - inner should be asterisk\n- outer two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Nested: []string{StyleDash, StyleAsterisk}}
	got := r.Fix(f)
	want := "- outer\n  * inner should be asterisk\n- outer two\n"
	assert.Equal(t, want, string(got))
}

func TestFix_NoChangeNeeded(t *testing.T) {
	src := []byte("- item one\n- item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash}
	got := r.Fix(f)
	assert.Equal(t, string(src), string(got))
}

func TestApplySettings_ValidStyle(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"style": "asterisk"})
	require.NoError(t, err)
	assert.Equal(t, StyleAsterisk, r.Style)
}

func TestApplySettings_InvalidStyle(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"style": "invalid"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid style")
}

func TestApplySettings_ValidNested_StringSlice(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"nested": []string{"dash", "asterisk"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{StyleDash, StyleAsterisk}, r.Nested)
}

func TestCheck_MixedMarkers_InSequentialLists(t *testing.T) {
	// CommonMark creates a new list node when markers change, so each
	// run of same-marker items forms its own *ast.List. When style is
	// dash, all asterisk and plus items must be flagged regardless of
	// which list node they belong to.
	src := []byte("- correct\n* wrong asterisk\n+ wrong plus\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash}
	diags := r.Check(f)
	// line 1 is dash (correct), lines 2 and 3 are in separate lists with wrong markers
	require.Len(t, diags, 2, "one diagnostic per mismatching item across all lists")
	assert.Equal(t, 2, diags[0].Line)
	assert.Contains(t, diags[0].Message, "uses asterisk")
	assert.Equal(t, 3, diags[1].Line)
	assert.Contains(t, diags[1].Message, "uses plus")
}

func TestCheck_MixedMarkers_FixedPerItem(t *testing.T) {
	// When different items on consecutive lines use different markers,
	// Fix must correct each individual item to the configured style.
	src := []byte("* wrong\n+ also wrong\n- correct\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleDash}
	got := r.Fix(f)
	want := "- wrong\n- also wrong\n- correct\n"
	assert.Equal(t, want, string(got))
}

func TestApplySettings_ValidNested(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"nested": []any{"dash", "asterisk", "plus"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{StyleDash, StyleAsterisk, StylePlus}, r.Nested)
}

func TestApplySettings_InvalidNestedItem(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"nested": []any{"dash", "invalid"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid nested")
}

func TestApplySettings_UnknownSetting(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": "value"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	defaults := r.DefaultSettings()
	assert.Equal(t, StyleDash, defaults["style"])
	assert.Equal(t, []string{}, defaults["nested"])
}
