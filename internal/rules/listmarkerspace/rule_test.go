package listmarkerspace

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuleMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS061", r.ID())
	assert.Equal(t, "list-marker-space", r.Name())
	assert.Equal(t, "list", r.Category())
}

func TestCheck_OneSpace_Good(t *testing.T) {
	src := []byte("- item one\n- item two\n- item three\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_TwoSpaces_Flagged(t *testing.T) {
	src := []byte("-  item one\n-  item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	diags := r.Check(f)
	require.Len(t, diags, 2, "one diagnostic per offending item")
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 2, diags[1].Line)
	assert.Contains(t, diags[0].Message, "2 spaces")
	assert.Contains(t, diags[0].Message, "expected 1")
}

func TestCheck_OrderedTwoSpaces_Flagged(t *testing.T) {
	src := []byte("1.  first\n2.  second\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	diags := r.Check(f)
	require.Len(t, diags, 2)
	assert.Contains(t, diags[0].Message, "2 spaces")
	assert.Contains(t, diags[0].Message, "expected 1")
}

func TestCheck_OrderedOneSpace_Good(t *testing.T) {
	src := []byte("1. first\n2. second\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_ULVsOL_SeparateKnobs(t *testing.T) {
	// ul-single=1, ol-single=2: ordered needs 2 spaces, unordered needs 1
	src := []byte("- ul item\n\n1. ol item\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 2, OLMulti: 2}
	diags := r.Check(f)
	require.Len(t, diags, 1, "ordered list item should be flagged")
	assert.Equal(t, 3, diags[0].Line)
	assert.Contains(t, diags[0].Message, "expected 2")
}

func TestCheck_MultiItem_UsesULMulti(t *testing.T) {
	// Multi-paragraph item: ul-multi=2, item has 1 space → flagged
	src := []byte("- First paragraph\n\n  Second paragraph\n\n- Single item\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 2, OLSingle: 1, OLMulti: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "only multi-paragraph item flagged")
	assert.Equal(t, 1, diags[0].Line)
	assert.Contains(t, diags[0].Message, "expected 2")
}

func TestCheck_MultiItem_Good(t *testing.T) {
	// Multi-paragraph item with 2 spaces: ul-multi=2 → no diagnostic
	src := []byte("-  First paragraph\n\n   Second paragraph\n\n- Single item\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 2, OLSingle: 1, OLMulti: 1}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_NestedList_PerLevel(t *testing.T) {
	// Nested list: outer has 1 space, inner has 1 space, both correct
	src := []byte("- outer\n  - inner\n  - inner two\n- outer two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_NestedList_InnerFlagged(t *testing.T) {
	// Inner items have 2 spaces, expect 1
	src := []byte("- outer\n  -  inner wrong\n- outer two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 2, diags[0].Line)
}

func TestCheck_AsteriskAndPlusMarkers(t *testing.T) {
	src := []byte("*  asterisk item\n+  plus item\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	diags := r.Check(f)
	require.Len(t, diags, 2)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 2, diags[1].Line)
}

func TestFix_TwoSpacesToOne(t *testing.T) {
	src := []byte("-  item one\n-  item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	got := r.Fix(f)
	want := "- item one\n- item two\n"
	assert.Equal(t, want, string(got))
}

func TestFix_OneSpaceToTwo(t *testing.T) {
	src := []byte("- item one\n- item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 2, ULMulti: 2, OLSingle: 1, OLMulti: 1}
	got := r.Fix(f)
	want := "-  item one\n-  item two\n"
	assert.Equal(t, want, string(got))
}

func TestFix_OrderedTwoSpacesToOne(t *testing.T) {
	src := []byte("1.  first\n2.  second\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	got := r.Fix(f)
	want := "1. first\n2. second\n"
	assert.Equal(t, want, string(got))
}

func TestFix_MultiItemSkipped(t *testing.T) {
	// Multi-paragraph items are skipped by Fix to avoid misaligning continuation
	// lines whose indentation depends on the marker-space count. Only single items
	// are rewritten.
	src := []byte("- First paragraph\n\n  Second paragraph\n\n- Single item\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 2, OLSingle: 1, OLMulti: 1}
	got := r.Fix(f)
	// Multi item unchanged; single item already correct.
	assert.Equal(t, string(src), string(got))
}

func TestFix_NoChangeNeeded(t *testing.T) {
	src := []byte("- item one\n- item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	got := r.Fix(f)
	assert.Equal(t, string(src), string(got))
}

func TestFix_Idempotent(t *testing.T) {
	src := []byte("-  item one\n-  item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	first := r.Fix(f)
	f2, err := lint.NewFile("test.md", first)
	require.NoError(t, err)
	second := r.Fix(f2)
	assert.Equal(t, string(first), string(second))
}

func TestFix_WithIndentation(t *testing.T) {
	src := []byte("- outer\n  -  inner wrong\n- outer two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	got := r.Fix(f)
	want := "- outer\n  - inner wrong\n- outer two\n"
	assert.Equal(t, want, string(got))
}

func TestApplySettings_Valid(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"ul-single": 2,
		"ul-multi":  3,
		"ol-single": 1,
		"ol-multi":  2,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, r.ULSingle)
	assert.Equal(t, 3, r.ULMulti)
	assert.Equal(t, 1, r.OLSingle)
	assert.Equal(t, 2, r.OLMulti)
}

func TestApplySettings_FloatCoerced(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"ul-single": float64(2)})
	require.NoError(t, err)
	assert.Equal(t, 2, r.ULSingle)
}

func TestApplySettings_ZeroRejected(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"ul-single": 0})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), ">= 1")
}

func TestApplySettings_NonIntRejected(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"ul-single": "one"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "integer")
}

func TestApplySettings_UnknownSetting(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": 1})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	d := r.DefaultSettings()
	assert.Equal(t, 1, d["ul-single"])
	assert.Equal(t, 1, d["ul-multi"])
	assert.Equal(t, 1, d["ol-single"])
	assert.Equal(t, 1, d["ol-multi"])
}

func TestParseMarkerAndSpaces_Unordered(t *testing.T) {
	cases := []struct {
		line      string
		markerEnd int
		spaces    int
	}{
		{"- item", 1, 1},
		{"-  item", 1, 2},
		{"* item", 1, 1},
		{"+ item", 1, 1},
		{"  - item", 3, 1},
		{"  -  item", 3, 2},
	}
	for _, tc := range cases {
		me, sp := parseMarkerAndSpaces([]byte(tc.line))
		assert.Equal(t, tc.markerEnd, me, "line=%q markerEnd", tc.line)
		assert.Equal(t, tc.spaces, sp, "line=%q spaces", tc.line)
	}
}

func TestParseMarkerAndSpaces_Ordered(t *testing.T) {
	cases := []struct {
		line      string
		markerEnd int
		spaces    int
	}{
		{"1. item", 2, 1},
		{"1.  item", 2, 2},
		{"10. item", 3, 1},
		{"10.  item", 3, 2},
		{"1) item", 2, 1},
	}
	for _, tc := range cases {
		me, sp := parseMarkerAndSpaces([]byte(tc.line))
		assert.Equal(t, tc.markerEnd, me, "line=%q markerEnd", tc.line)
		assert.Equal(t, tc.spaces, sp, "line=%q spaces", tc.line)
	}
}

func TestParseMarkerAndSpaces_NoMarker(t *testing.T) {
	cases := []string{"  text", "# heading", "", "123x"}
	for _, line := range cases {
		me, sp := parseMarkerAndSpaces([]byte(line))
		assert.Equal(t, 0, me, "line=%q", line)
		assert.Equal(t, 0, sp, "line=%q", line)
	}
}

func TestParseMarkerAndSpaces_TabIndent(t *testing.T) {
	// Tab before the marker is treated as whitespace to skip.
	me, sp := parseMarkerAndSpaces([]byte("\t- item"))
	assert.Equal(t, 2, me)
	assert.Equal(t, 1, sp)
}

func TestCheck_OrderedMultiItem_UsesOLMulti(t *testing.T) {
	// Ordered multi-paragraph item: OLMulti=2, got=1 → diagnostic.
	src := []byte("1. First paragraph\n\n   Second paragraph\n\n2. Single item\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 2}
	diags := r.Check(f)
	require.Len(t, diags, 1, "only the multi ordered item is flagged")
	assert.Equal(t, 1, diags[0].Line)
	assert.Contains(t, diags[0].Message, "expected 2")
}

func TestAdjustSpaces_NoMarkerLine(t *testing.T) {
	// adjustSpaces returns the line unchanged when it has no list marker.
	line := []byte("plain text")
	got := adjustSpaces(line, 2)
	assert.Equal(t, line, got)
}

func TestCheck_LooseList_FallbackPath(t *testing.T) {
	// A loose list (blank lines between items) exercises the firstLineOfListItem
	// fallback that walks child nodes when li.Lines().Len() == 0.
	src := []byte("-  item one\n\n-  item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	diags := r.Check(f)
	require.Len(t, diags, 2)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 3, diags[1].Line)
}

func TestCheck_ParagraphlessOuterItemSkipped(t *testing.T) {
	// A "paragraphless" outer item (-\n  - inner) has a *ast.List as its
	// only child (lines=0). firstLineOfListItem skips children with no
	// source lines, so the outer item returns 0 and is not checked.
	// The inner item is checked normally and produces no diagnostic when
	// spacing is correct.
	src := []byte("-\n  - nested item\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	diags := r.Check(f)
	// Inner item has 1 space (want 1) → no diagnostic.
	assert.Empty(t, diags)
}

func TestCheck_NoLineItems(t *testing.T) {
	// "- \n" → no children → firstLineOfListItem returns 0 directly.
	// "- \n  ---\n" → ThematicBreak child (lines=0) is skipped →
	// firstLineOfListItem returns 0.
	// Both exercise the line<=0 bounds check in checkList and Fix.
	for _, raw := range []string{"- \n", "- \n  ---\n"} {
		f, err := lint.NewFile("test.md", []byte(raw))
		require.NoError(t, err)
		r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
		_ = r.Check(f)
		_ = r.Fix(f)
	}
}

func TestPluralSpace(t *testing.T) {
	assert.Equal(t, "space", pluralSpace(1))
	assert.Equal(t, "spaces", pluralSpace(0))
	assert.Equal(t, "spaces", pluralSpace(2))
	assert.Equal(t, "spaces", pluralSpace(3))
}

func TestCheck_MessageGrammar(t *testing.T) {
	// "1 space" singular, "2 spaces" plural.
	src := []byte("-  item\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "2 spaces")

	src2 := []byte("- item\n")
	f2, err := lint.NewFile("test.md", src2)
	require.NoError(t, err)
	r2 := &Rule{ULSingle: 2, ULMulti: 2, OLSingle: 1, OLMulti: 1}
	diags2 := r2.Check(f2)
	require.Len(t, diags2, 1)
	assert.Contains(t, diags2[0].Message, "1 space")
	assert.NotContains(t, diags2[0].Message, "1 spaces")
}
