package emphasisstyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func parseFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func newRule(bold, italic string, forbid bool) *Rule {
	return &Rule{Bold: bold, Italic: italic, ForbidMixedNesting: forbid}
}

// --- disabled / unconfigured ------------------------------------------------

func TestNoSettings_NoCheck(t *testing.T) {
	r := &Rule{}
	f := parseFile(t, "# Heading\n\n__bold__ and _italic_\n")
	assert.Empty(t, r.Check(f))
}

// --- bold -------------------------------------------------------------------

func TestBoldAsterisk_Asterisk_OK(t *testing.T) {
	r := newRule("asterisk", "", false)
	f := parseFile(t, "# Heading\n\nSome **bold** text.\n")
	assert.Empty(t, r.Check(f))
}

func TestBoldAsterisk_Underscore_Diag(t *testing.T) {
	r := newRule("asterisk", "", false)
	f := parseFile(t, "# Heading\n\nSome __bold__ text.\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "bold uses underscore; configured style is asterisk", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
}

func TestBoldUnderscore_Underscore_OK(t *testing.T) {
	r := newRule("underscore", "", false)
	f := parseFile(t, "# Heading\n\n__bold__\n")
	assert.Empty(t, r.Check(f))
}

func TestBoldUnderscore_Asterisk_Diag(t *testing.T) {
	r := newRule("underscore", "", false)
	f := parseFile(t, "# Heading\n\n**bold**\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "bold uses asterisk; configured style is underscore", diags[0].Message)
}

// --- italic -----------------------------------------------------------------

func TestItalicUnderscore_Underscore_OK(t *testing.T) {
	r := newRule("", "underscore", false)
	f := parseFile(t, "# Heading\n\n_italic_\n")
	assert.Empty(t, r.Check(f))
}

func TestItalicUnderscore_Asterisk_Diag(t *testing.T) {
	r := newRule("", "underscore", false)
	f := parseFile(t, "# Heading\n\n*italic*\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "italic uses asterisk; configured style is underscore", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
}

func TestItalicAsterisk_Asterisk_OK(t *testing.T) {
	r := newRule("", "asterisk", false)
	f := parseFile(t, "# Heading\n\n*italic*\n")
	assert.Empty(t, r.Check(f))
}

func TestItalicAsterisk_Underscore_Diag(t *testing.T) {
	r := newRule("", "asterisk", false)
	f := parseFile(t, "# Heading\n\n_italic_\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "italic uses underscore; configured style is asterisk", diags[0].Message)
}

// --- mixed nesting ----------------------------------------------------------

func TestMixedNesting_SameDelimiterNested_NoDiag(t *testing.T) {
	// ***x*** = outer italic(*) wrapping inner bold(*); parent and child use
	// the same delimiter, so no mixed-nesting diagnostic.
	r := newRule("", "", true)
	f := parseFile(t, "# Heading\n\n***x***\n")
	assert.Empty(t, r.Check(f))
}

func TestMixedNesting_UnderscoreWrapsAsterisk_Diag(t *testing.T) {
	r := newRule("", "", true)
	f := parseFile(t, "# Heading\n\n_*x*_\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "mixed emphasis delimiters: underscore wraps asterisk", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
}

func TestMixedNesting_AsteriskWrapsUnderscore_Diag(t *testing.T) {
	r := newRule("", "", true)
	f := parseFile(t, "# Heading\n\n*_x_*\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "mixed emphasis delimiters: asterisk wraps underscore", diags[0].Message)
}

func TestMixedNesting_SameDelimiter_NoDiag(t *testing.T) {
	r := newRule("", "", true)
	f := parseFile(t, "# Heading\n\n*italic* and _italic_\n")
	assert.Empty(t, r.Check(f))
}

func TestMixedNesting_Disabled_NoDiag(t *testing.T) {
	r := newRule("", "", false)
	f := parseFile(t, "# Heading\n\n_*x*_\n")
	assert.Empty(t, r.Check(f))
}

// --- triple-delimiter runs --------------------------------------------------

func TestTripleAsterisk_ItalicUnderscore_DiagNoFix(t *testing.T) {
	r := newRule("asterisk", "underscore", false)
	f := parseFile(t, "# Heading\n\n***x***\n")
	diags := r.Check(f)
	// outer italic(level=1) uses asterisk but expected underscore → diagnostic
	require.NotEmpty(t, diags)
	hasItalic := false
	for _, d := range diags {
		if d.Message == "italic uses asterisk; configured style is underscore" {
			hasItalic = true
		}
	}
	assert.True(t, hasItalic, "expected italic diagnostic for triple-asterisk run")
	// Fix must not change the source (triple-run is skipped)
	assert.Equal(t, string(f.Source), string(r.Fix(f)))
}

func TestTripleUnderscore_DiagNoFix(t *testing.T) {
	r := newRule("asterisk", "asterisk", false)
	f := parseFile(t, "# Heading\n\n___x___\n")
	// both levels use underscore but config expects asterisk
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	// Fix must not change the source
	assert.Equal(t, string(f.Source), string(r.Fix(f)))
}

// --- metadata ---------------------------------------------------------------

func TestCategory(t *testing.T) {
	assert.Equal(t, "meta", (&Rule{}).Category())
}

// --- code span / fenced code (must not flag) --------------------------------

func TestCodeSpan_NoFlag(t *testing.T) {
	r := newRule("asterisk", "underscore", true)
	f := parseFile(t, "# Heading\n\n`*italic*` and `__bold__`\n")
	assert.Empty(t, r.Check(f))
}

func TestFencedCode_NoFlag(t *testing.T) {
	r := newRule("asterisk", "underscore", true)
	src := "# Heading\n\n```\n__bold__ *italic*\n```\n"
	f := parseFile(t, src)
	assert.Empty(t, r.Check(f))
}

// --- Check: emphasis containing link/image (emphInfo default branch) --------

func TestCheck_BoldWithLinkFirstChild(t *testing.T) {
	// Emphasis wrapping a link as first child: source[pos:pos+2] = "*[" which
	// fails isDelimRun, so emphInfo returns (0,-1) and no diagnostic is emitted.
	r := newRule("underscore", "", false)
	f := parseFile(t, "# Heading\n\n**[link](url)**\n")
	assert.Empty(t, r.Check(f))
}

func TestCheck_UndetectableDelimiter_NoDiag(t *testing.T) {
	// Image with empty alt text produces an emphasis child with no text
	// leaf — emphDelim returns 0 and checkEmphasis returns nil.
	r := newRule("underscore", "", false)
	f := parseFile(t, "# Heading\n\n**![](img.png)**\n")
	// No diagnostic expected because delimiter is undetectable.
	assert.Empty(t, r.Check(f))
}

func TestCheck_LinkLastChild_Diag(t *testing.T) {
	// **text [link](url)** has Text as first child (delimiter detectable) but
	// Link as last child (emphCloseStart returns -1). Check emits a diagnostic;
	// Fix must not touch the source.
	r := newRule("underscore", "", false)
	f := parseFile(t, "# Heading\n\n**text [link](url)**\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "bold uses asterisk; configured style is underscore", diags[0].Message)
}

func TestFix_LinkLastChild_NoChange(t *testing.T) {
	r := newRule("underscore", "", false)
	f := parseFile(t, "# Heading\n\n**text [link](url)**\n")
	assert.Equal(t, string(f.Source), string(r.Fix(f)))
}

func TestFix_OuterEmphInnerLinkLastChild_NoChange(t *testing.T) {
	// **_text [link](url)_** — outer bold wraps inner italic whose last child
	// is a Link; emphCloseStart recurses into inner and returns -1, propagating
	// up so Fix skips the outer node too.
	r := newRule("underscore", "", false)
	f := parseFile(t, "# Heading\n\n**_text [link](url)_**\n")
	assert.Equal(t, string(f.Source), string(r.Fix(f)))
}

func TestFix_LinkFirstChild_NoChange(t *testing.T) {
	// emphDelim returns 0 for **[link](url)** because the inferred offset
	// lands on "*[" not "**"; Fix must leave source unchanged.
	r := newRule("underscore", "", false)
	f := parseFile(t, "# Heading\n\n**[link](url)**\n")
	assert.Equal(t, string(f.Source), string(r.Fix(f)))
}

func TestFix_UnderscoreLinkFirstChild_NoChange(t *testing.T) {
	// __[link](url)__ similarly has source[pos:pos+2] = "_["; undetectable.
	r := newRule("asterisk", "", false)
	f := parseFile(t, "# Heading\n\n__[link](url)__\n")
	assert.Equal(t, string(f.Source), string(r.Fix(f)))
}

// --- Fix -------------------------------------------------------------------

func TestFix_Unconfigured_NoChange(t *testing.T) {
	r := &Rule{}
	f := parseFile(t, "# Heading\n\n__bold__ and *italic*\n")
	assert.Equal(t, string(f.Source), string(r.Fix(f)))
}

func TestFix_WantZeroSkipsUnconfiguredRole(t *testing.T) {
	// Only bold configured; italic emphasis in source must be skipped.
	r := newRule("asterisk", "", false)
	f := parseFile(t, "# Heading\n\n__bold__ and *italic*\n")
	got := string(r.Fix(f))
	assert.Equal(t, "# Heading\n\n**bold** and *italic*\n", got)
}

func TestFix_NestedEmphasisClose(t *testing.T) {
	// _**x**_ has outer italic using underscore and inner bold using asterisk.
	// With italic:asterisk the outer is fixed; this exercises the
	// emphCloseStart inner-emphasis branch.
	r := newRule("", "asterisk", false)
	f := parseFile(t, "# Heading\n\n_**x**_\n")
	require.Len(t, r.Check(f), 1)
	got := string(r.Fix(f))
	assert.Equal(t, "# Heading\n\n***x***\n", got)
}

func TestFix_UnderscoreBold_ToAsterisk(t *testing.T) {
	r := newRule("asterisk", "", false)
	f := parseFile(t, "# Heading\n\nSome __bold__ text.\n")
	require.Len(t, r.Check(f), 1)
	got := string(r.Fix(f))
	assert.Equal(t, "# Heading\n\nSome **bold** text.\n", got)
}

func TestFix_AsteriskItalic_ToUnderscore(t *testing.T) {
	r := newRule("", "underscore", false)
	f := parseFile(t, "# Heading\n\nSome *italic* text.\n")
	require.Len(t, r.Check(f), 1)
	got := string(r.Fix(f))
	assert.Equal(t, "# Heading\n\nSome _italic_ text.\n", got)
}

func TestFix_Multiple(t *testing.T) {
	r := newRule("asterisk", "underscore", false)
	f := parseFile(t, "# Heading\n\n__bold__ and *italic*\n")
	require.Len(t, r.Check(f), 2)
	got := string(r.Fix(f))
	assert.Equal(t, "# Heading\n\n**bold** and _italic_\n", got)
}

func TestFix_NoViolations_Unchanged(t *testing.T) {
	r := newRule("asterisk", "underscore", false)
	f := parseFile(t, "# Heading\n\n**bold** and _italic_\n")
	assert.Empty(t, r.Check(f))
	assert.Equal(t, string(f.Source), string(r.Fix(f)))
}

// --- ApplySettings ----------------------------------------------------------

func TestApplySettings_Valid(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"bold":                 "asterisk",
		"italic":               "underscore",
		"forbid-mixed-nesting": true,
	}))
	assert.Equal(t, "asterisk", r.Bold)
	assert.Equal(t, "underscore", r.Italic)
	assert.True(t, r.ForbidMixedNesting)
}

func TestApplySettings_BoldNotString(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"bold": 42})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bold must be a string")
}

func TestApplySettings_InvalidBold(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"bold": "wrong"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid bold")
}

func TestApplySettings_ItalicNotString(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"italic": 42})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "italic must be a string")
}

func TestApplySettings_InvalidItalic(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"italic": "wrong"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid italic")
}

func TestApplySettings_InvalidForbid(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"forbid-mixed-nesting": "yes"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a bool")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	d := r.DefaultSettings()
	assert.Equal(t, "", d["bold"])
	assert.Equal(t, "", d["italic"])
	assert.Equal(t, false, d["forbid-mixed-nesting"])
}

func TestEnabledByDefault(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault())
}

// --- Direct helper-function tests (defensive branch coverage) ---------------

// TestEmphCloseStart_NoLastChild covers the nil-last-child guard (line 317).
func TestEmphCloseStart_NoLastChild(t *testing.T) {
	em := ast.NewEmphasis(1)
	assert.Equal(t, -1, emphCloseStart(em))
}

// TestEmphInfo_NegativePos covers line 282: text whose Segment.Start <
// totalLevels produces pos < 0, so emphInfo returns (0, -1).
func TestEmphInfo_NegativePos(t *testing.T) {
	em := ast.NewEmphasis(1)
	txt := ast.NewText()
	txt.Segment = text.NewSegment(0, 5) // pos = 0 − 1 = −1
	em.AppendChild(em, txt)
	d, pos := emphInfo(em, []byte("hello"))
	assert.Equal(t, byte(0), d)
	assert.Equal(t, -1, pos)
}

// TestEmphInfo_NoChildren covers line 301: the loop is never entered when
// the emphasis has no children, falling through to the final return.
func TestEmphInfo_NoChildren(t *testing.T) {
	em := ast.NewEmphasis(1)
	d, pos := emphInfo(em, []byte("*x*"))
	assert.Equal(t, byte(0), d)
	assert.Equal(t, -1, pos)
}

// TestEmphInfo_DefaultBranchSuccess covers lines 294-296: the default branch
// of emphInfo succeeds when firstTextStart(child) − totalLevels points to a
// valid delimiter run.  *[*text*](url)* is a concrete case: the outer italic
// wraps a Link whose subtree's first Text lands exactly one byte after a '*'.
func TestEmphInfo_DefaultBranchSuccess(t *testing.T) {
	r := newRule("", "underscore", false)
	f := parseFile(t, "# Heading\n\n*[*text*](url)*\n")
	// emphInfo for the outer italic hits the default branch and finds '*' →
	// a diagnostic is emitted for both the outer and the inner italic.
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	// The inner *text* is fully fixable; the outer is not (closeStart = -1).
	got := string(r.Fix(f))
	assert.Equal(t, "# Heading\n\n*[_text_](url)*\n", got)
}

// TestEmphReplacements_ClosingNotDelim covers lines 204-206: emphReplacements
// returns nil when the bytes at closeStart are not the expected delimiter.
func TestEmphReplacements_ClosingNotDelim(t *testing.T) {
	r := &Rule{Italic: "underscore"}
	src := []byte("*hello.")
	em := ast.NewEmphasis(1)
	txt := ast.NewText()
	txt.Segment = text.NewSegment(1, 6) // "hello"; Stop=6 → source[6]='.'
	em.AppendChild(em, txt)
	reps := r.emphReplacements(em, src)
	assert.Nil(t, reps)
}

// TestIsTripleRun_InnerNoChildren covers lines 359-361: isTripleRun returns
// false when emphOpenStart cannot be determined (inner has no children →
// emphInfo exhausts the loop and returns (0, -1)).
func TestIsTripleRun_InnerNoChildren(t *testing.T) {
	outer := ast.NewEmphasis(1)
	inner := ast.NewEmphasis(2)
	outer.AppendChild(outer, inner)
	assert.False(t, isTripleRun(outer, []byte("***")))
}

// TestIsTripleRun_OutOfBounds covers lines 363-365: isTripleRun returns false
// when openStart + total exceeds the source length.
func TestIsTripleRun_OutOfBounds(t *testing.T) {
	// outer(level=1) → inner(level=2) → text at Segment.Start=3
	// source "**" (2 bytes): pos=0, openStart=0, total=3 → 0+3 > 2
	src := []byte("**")
	outer := ast.NewEmphasis(1)
	inner := ast.NewEmphasis(2)
	txt := ast.NewText()
	txt.Segment = text.NewSegment(3, 4)
	inner.AppendChild(inner, txt)
	outer.AppendChild(outer, inner)
	assert.False(t, isTripleRun(outer, src))
}

// TestIsDelimRun_NegativeStart covers lines 377-379 (start < 0 branch).
func TestIsDelimRun_NegativeStart(t *testing.T) {
	assert.False(t, isDelimRun([]byte("*hello*"), -1, 1, '*'))
}

// TestIsDelimRun_StartPlusLengthOutOfBounds covers lines 377-379
// (start+length > len(source) branch).
func TestIsDelimRun_StartPlusLengthOutOfBounds(t *testing.T) {
	assert.False(t, isDelimRun([]byte("*"), 0, 2, '*'))
}
