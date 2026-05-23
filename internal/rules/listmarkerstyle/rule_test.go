package listmarkerstyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
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

// --- replaceMarker ---

// TestReplaceMarker covers the three branches: marker at the line
// start, marker after whitespace, and the no-marker short-circuit
// (a non-whitespace, non-marker first char returns the line
// unchanged). Each path is reached via Fix in production, but only
// the success path was directly pinned.
func TestReplaceMarker(t *testing.T) {
	t.Run("dash at start swaps to plus", func(t *testing.T) {
		got := replaceMarker([]byte("- item"), '+')
		assert.Equal(t, []byte("+ item"), got)
	})
	t.Run("asterisk after indent swaps to dash", func(t *testing.T) {
		got := replaceMarker([]byte("    * nested"), '-')
		assert.Equal(t, []byte("    - nested"), got)
	})
	t.Run("non-marker first char returns line unchanged", func(t *testing.T) {
		in := []byte("plain text")
		got := replaceMarker(in, '+')
		assert.Equal(t, in, got)
	})
	t.Run("blank line returns unchanged", func(t *testing.T) {
		in := []byte("")
		got := replaceMarker(in, '+')
		assert.Equal(t, in, got)
	})
}

// --- markerOnLine ---

// TestMarkerOnLine pins each branch the helper takes: out-of-range
// (negative and past-EOL), whitespace skip, non-marker char short
// circuits, and each of the three valid markers. Check-path tests
// only ever hit the dash-on-a-real-list case, so the rest of the
// matrix was uncovered.
func TestMarkerOnLine(t *testing.T) {
	src := []byte("- dash\n  * indent-asterisk\n\t+ tab-plus\nplain text\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)
	r := &Rule{}
	assert.Equal(t, byte('-'), r.markerOnLine(f, 1))
	assert.Equal(t, byte('*'), r.markerOnLine(f, 2),
		"leading spaces are skipped")
	assert.Equal(t, byte('+'), r.markerOnLine(f, 3),
		"leading tabs are skipped")
	assert.Equal(t, byte(0), r.markerOnLine(f, 4),
		"non-marker first char returns 0")
	assert.Equal(t, byte(0), r.markerOnLine(f, 0),
		"line 0 is out of range")
	assert.Equal(t, byte(0), r.markerOnLine(f, 999),
		"line past EOF is out of range")
}

// --- firstLineOfListItem ---

// TestFirstLineOfListItem_ZeroForEmpty pins the fallback branch when
// a synthetic ListItem has neither Lines() nor any child block with
// resolvable lines.
func TestFirstLineOfListItem_ZeroForEmpty(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte(""))
	require.NoError(t, err)
	li := ast.NewListItem(0)
	r := &Rule{}
	assert.Equal(t, 0, r.firstLineOfListItem(f, li))
}

// --- styleToMarker / markerToStyle ---

// TestStyleToMarker pins every documented style branch including
// the default-case fallback. Check-path tests exercise the
// successful branches but not the unknown-style guard, which is
// the safety net for a config that survived ApplySettings but had
// the style mutated afterward (e.g. by a future merge bug).
func TestStyleToMarker(t *testing.T) {
	cases := map[string]byte{
		StyleDash:     '-',
		StyleAsterisk: '*',
		StylePlus:     '+',
		"":            '-', // default fallback
		"bogus":       '-',
	}
	for style, want := range cases {
		assert.Equalf(t, want, styleToMarker(style), "style %q", style)
	}
}

// TestMarkerToStyle pins every documented marker plus the unknown
// fallback. The Check loop only ever calls markerToStyle on bytes
// it already validated, so the default path was uncovered without
// a unit test.
func TestMarkerToStyle(t *testing.T) {
	cases := map[byte]string{
		'-': StyleDash,
		'*': StyleAsterisk,
		'+': StylePlus,
		'x': "unknown",
	}
	for marker, want := range cases {
		assert.Equalf(t, want, markerToStyle(marker), "marker %q", marker)
	}
}

// --- blockFirstLine ---

// TestBlockFirstLine_RecursesIntoChildren pins the recursion branch
// of blockFirstLine: container blocks (List, ListItem, etc.) have an
// empty Lines() and must be walked through their children to find
// the first source line. The Check-path tests above only exercise
// the direct-hit branch (nodes whose Lines() has length > 0), so
// the recursion was uncovered without a unit test.
func TestBlockFirstLine_RecursesIntoChildren(t *testing.T) {
	src := []byte("- one\n- two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	// The top-level List node has no Lines(); recursion through
	// its ListItem children reaches the inline paragraph at line 1.
	var list *ast.List
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if l, ok := n.(*ast.List); ok {
			list = l
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	require.NotNil(t, list, "fixture must contain a List node")
	assert.Equal(t, 0, list.Lines().Len(),
		"List nodes have empty Lines(); the recursion branch is "+
			"what reaches the first source line")
	assert.Equal(t, 1, blockFirstLine(f, list))
}

// TestBlockFirstLine_ReturnsZeroForEmptyTree pins the fallback path:
// a synthetic block tree with no descendants that has a Lines()
// entry must return 0. The Check loop guards against this by only
// emitting a diagnostic when line > 0, so the fallback was
// unreachable from real Markdown.
func TestBlockFirstLine_ReturnsZeroForEmptyTree(t *testing.T) {
	// Synthetic empty paragraph: no Lines, no children.
	empty := ast.NewParagraph()
	f, err := lint.NewFile("test.md", []byte(""))
	require.NoError(t, err)
	assert.Equal(t, 0, blockFirstLine(f, empty))
}

// TestBlockFirstLine_DirectLines pins the direct-hit branch with an
// explicit assertion, mirroring the production path the Check loop
// drives. A synthetic paragraph with a non-empty Lines() returns
// the line of its first segment, without recursing.
func TestBlockFirstLine_DirectLines(t *testing.T) {
	src := []byte("line one\nline two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	p := ast.NewParagraph()
	// Point the segment at "line two" → starts at offset 9, on line 2.
	p.Lines().Append(text.NewSegment(9, 17))
	assert.Equal(t, 2, blockFirstLine(f, p))
}
