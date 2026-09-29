package occurrence

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func mustApply(t *testing.T, r *Rule, s map[string]any) {
	t.Helper()
	require.NoError(t, r.ApplySettings(s))
}

// --- identity ---

func TestID(t *testing.T) {
	assert.Equal(t, "MDS060", (&Rule{}).ID())
}

func TestName(t *testing.T) {
	assert.Equal(t, "occurrence", (&Rule{}).Name())
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "prose", (&Rule{}).Category())
}

func TestEnabledByDefault(t *testing.T) {
	assert.False(t, (&Rule{}).EnabledByDefault())
}

func TestWordlistTarget(t *testing.T) {
	assert.Equal(t, "tokens", (&Rule{}).WordlistTarget())
}

// --- early exits ---

func TestCheck_NilAST_NoDiagnostic(t *testing.T) {
	r := &Rule{Tokens: []string{"x"}, Max: 1}
	assert.Empty(t, r.Check(&lint.File{}))
}

func TestCheck_NoTokensNoPattern_NoDiagnostic(t *testing.T) {
	r := &Rule{Max: 1}
	assert.Empty(t, r.Check(mustFile(t, "any text.\n")))
}

// --- paragraph scope (default) ---

func TestCheck_Paragraph_TokenUnderMax_NoDiagnostic(t *testing.T) {
	r := &Rule{Max: 2, Count: "each"}
	mustApply(t, r, map[string]any{"tokens": []any{"em dash"}, "max": 2, "count": "each"})
	// "em dash" appears once — within max 2
	assert.Empty(t, r.Check(mustFile(t, "# Title\n\nAn em dash here.\n")))
}

func TestCheck_Paragraph_TokenExceedsMax_Diagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"em"}, "max": 2, "count": "each"})
	src := "# Title\n\nem em em here.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS060", diags[0].RuleID)
	assert.Equal(t, 3, diags[0].Line)
	assert.Contains(t, diags[0].Message, `"em"`)
	assert.Contains(t, diags[0].Message, "3")
	assert.Contains(t, diags[0].Message, "max 2")
}

func TestCheck_Paragraph_TokenBelowMin_Diagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"required"}, "min": 1, "max": -1, "count": "each"})
	src := "# Title\n\nThis paragraph has no required word here.\n"
	diags := r.Check(mustFile(t, src))
	// "required" appears once, min=1 satisfied
	assert.Empty(t, diags)
}

func TestCheck_Paragraph_TokenBelowMin_Fires(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"keyword"}, "min": 2, "max": -1, "count": "each"})
	src := "# Title\n\nOnly keyword appears once.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "min 2")
}

func TestCheck_Paragraph_CaseInsensitive(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"WORD"}, "max": 1, "count": "each", "case-sensitive": false})
	// "word" and "Word" and "WORD" each count; total 3 in one paragraph
	src := "# T\n\nword Word WORD.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "3")
}

func TestCheck_Paragraph_CaseSensitive(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"word"}, "max": 1, "count": "each", "case-sensitive": true})
	// only lowercase "word" counts; "Word" and "WORD" are skipped → 1 match, no violation
	src := "# T\n\nword Word WORD.\n"
	diags := r.Check(mustFile(t, src))
	assert.Empty(t, diags)
}

func TestCheck_Paragraph_PatternExceedsMax_Diagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"pattern": "—", "max": 2, "count": "combined"})
	src := "# Title\n\nFirst — second — third — end.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "3")
	assert.Contains(t, diags[0].Message, "max 2")
}

func TestCheck_Paragraph_PatternUnderMax_NoDiagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"pattern": "—", "max": 2, "count": "combined"})
	src := "# Title\n\nFirst — second — end.\n"
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_Paragraph_Combined_Tokens(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"a", "b"}, "max": 3, "count": "combined"})
	// "a" appears 3 times, "b" appears 1 time → combined 4 > max 3
	src := "# Title\n\na a a b end.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "4")
}

func TestCheck_Paragraph_SkipsFencedCodeBlock(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"keyword"}, "max": 2, "count": "each"})
	// keyword inside fenced block should not count
	src := "# Title\n\n```\nkeyword keyword keyword\n```\n\nOnly one keyword here.\n"
	diags := r.Check(mustFile(t, src))
	assert.Empty(t, diags)
}

// --- section scope ---

func TestCheck_Section_TokenExceedsMax_Diagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"jargon"}, "max": 2, "scope": "section", "count": "each"})
	src := "# Title\n\njargon jargon jargon here.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Contains(t, diags[0].Message, "section")
}

func TestCheck_Section_MultipleHeadings(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"jargon"}, "max": 2, "scope": "section", "count": "each"})
	src := "# A\n\njargon jargon jargon.\n\n## B\n\njargon once.\n"
	diags := r.Check(mustFile(t, src))
	// Only section A exceeds max
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
}

func TestCheck_Section_FencedCodeBlockExcluded(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"keyword"}, "max": 2, "scope": "section", "count": "each"})
	src := "# Title\n\nkeyword keyword.\n\n```\nkeyword keyword keyword\n```\n"
	diags := r.Check(mustFile(t, src))
	// Only 2 in prose, 3 in fenced block (excluded)
	assert.Empty(t, diags)
}

// --- file scope ---

func TestCheck_File_CombinedAcrossParagraphs(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"word"}, "max": 3, "scope": "file", "count": "combined"})
	// "word" total: 2 + 2 = 4 > max 3
	src := "# Title\n\nword word.\n\nalso word word.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Contains(t, diags[0].Message, "file")
}

func TestCheck_File_UnderMax_NoDiagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"word"}, "max": 3, "scope": "file", "count": "combined"})
	src := "# Title\n\nword word.\n\nalso word here.\n"
	assert.Empty(t, r.Check(mustFile(t, src)))
}

// --- ApplySettings validation ---

func TestApplySettings_InvalidScope(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"scope": "block"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scope")
}

func TestApplySettings_InvalidPattern(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"pattern": "["})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pattern")
}

func TestApplySettings_InvalidCount(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"count": "all"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "count")
}

func TestApplySettings_TokensAndPatternMutuallyExclusive(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"tokens":  []any{"word"},
		"pattern": "word",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": "value"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown")
}

func TestApplySettings_MinNegative(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min": -1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min")
	assert.Contains(t, err.Error(), "0")
}

func TestCheck_DiagnosticHasFilePath(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"over"}, "max": 1, "count": "each"})
	f, err := lint.NewFile("myfile.md", []byte("# T\n\nover over.\n"))
	require.NoError(t, err)
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "myfile.md", diags[0].File)
}

func TestApplySettings_CaseSensitiveAfterPattern(t *testing.T) {
	// Verify that case-sensitive order relative to pattern does not matter.
	r1 := &Rule{}
	require.NoError(t, r1.ApplySettings(map[string]any{"pattern": "word", "case-sensitive": true}))
	r2 := &Rule{}
	require.NoError(t, r2.ApplySettings(map[string]any{"case-sensitive": true, "pattern": "word"}))
	assert.Equal(t, r1.Pattern.String(), r2.Pattern.String())
}

func TestDefaultSettings_Keys(t *testing.T) {
	d := (&Rule{}).DefaultSettings()
	assert.Contains(t, d, "scope")
	assert.Contains(t, d, "tokens")
	assert.Contains(t, d, "pattern")
	assert.Contains(t, d, "min")
	assert.Contains(t, d, "max")
	assert.Contains(t, d, "count")
	assert.Contains(t, d, "case-sensitive")
}

// --- file scope, each mode ---

func TestCheck_File_EachToken_ExceedsMax(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"word"}, "max": 3, "scope": "file", "count": "each"})
	// "word" total: 2 + 2 = 4 > max 3
	src := "# Title\n\nword word.\n\nalso word word.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "file")
}

func TestCheck_File_EachToken_UnderMax_NoDiagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"word"}, "max": 3, "scope": "file", "count": "each"})
	src := "# Title\n\nword word.\n\nalso word here.\n"
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_File_EachPattern_ExceedsMax(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"pattern": "—", "max": 2, "scope": "file", "count": "each"})
	// 3 em-dashes across two paragraphs → exceeds max 2
	src := "# T\n\nFirst — second.\n\nThird — fourth — end.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "file")
}

// --- section scope: combined and pattern ---

func TestCheck_Section_NoHeadings_NoDiagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"word"}, "max": 1, "scope": "section", "count": "each"})
	// no headings → no sections → no diagnostics
	src := "plain paragraph word word word.\n"
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_Section_Combined_ExceedsMax(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"a", "b"}, "max": 3, "scope": "section", "count": "combined"})
	// "a" × 3, "b" × 1 → combined 4 > max 3
	src := "# Title\n\na a a b.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "section")
}

func TestCheck_Section_Pattern_ExceedsMax(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"pattern": "—", "max": 2, "scope": "section", "count": "each"})
	// 3 em-dashes in the section → exceeds max 2
	src := "# Title\n\nFirst — second — third — end.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "section")
}

// --- ApplySettings type-error paths ---

func TestApplySettings_ScopeWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"scope": 42})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scope")
}

func TestApplySettings_TokensWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"tokens": "word"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tokens")
}

func TestApplySettings_PatternWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"pattern": 42})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pattern")
}

func TestApplySettings_MaxWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max": "two"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max")
}

func TestApplySettings_CountWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"count": 42})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "count")
}

func TestApplySettings_CaseSensitiveWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"case-sensitive": "yes"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "case-sensitive")
}

func TestApplySettings_MinWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min": "two"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min")
}

func TestApplySettings_MaxBelowNegativeOne(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max": -2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max")
	assert.Contains(t, err.Error(), "-1")
}

func TestApplySettings_MinGreaterThanMax(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"tokens": []any{"x"}, "min": 5, "max": 2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min")
	assert.Contains(t, err.Error(), "max")
	// A rejected band must not linger: an instance reused after the
	// error would otherwise flag every scope, since no count is both
	// >= 5 and <= 2.
	assert.Equal(t, 0, r.Min)
	assert.Equal(t, -1, r.Max)
}

func TestApplySettings_MinOnlyNoFalsePositive(t *testing.T) {
	// A zero-value Rule has Max=0, but max was never set, so a min-only
	// config must leave the upper bound open — in ApplySettings and in
	// Check alike.
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"tokens": []any{"x"}, "min": 1})
	require.NoError(t, err, "setting only min on a zero-value Rule must not error")
	diags := r.Check(mustFile(t, "# T\n\nx and x and x.\n"))
	assert.Empty(t, diags, "three matches satisfy min 1 with no max set")
}

func TestCheck_InlineCodeSpanNotCounted(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{
		"pattern": "—", "scope": "paragraph", "count": "combined", "max": 2,
	})
	f := mustFile(t, "# T\n\nUse `a—b—c` as the separator — see below.\n")
	assert.Empty(t, r.Check(f), "em dashes inside an inline code span must not count")
}

func TestCheck_InlineCodeSpanDoesNotJoinWords(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"foobar"}, "min": 1})
	f := mustFile(t, "# T\n\nfoo`x`bar here.\n")
	diags := r.Check(f)
	require.Len(t, diags, 1, "dropping a code span must not splice its neighbours into a match")
	assert.Contains(t, diags[0].Message, "min 1")
}

func TestCheck_Paragraph_EmptyToken_NoDiagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{""}, "max": 1, "count": "each"})
	// empty token always returns count 0 → within any max
	assert.Empty(t, r.Check(mustFile(t, "# T\n\nsome text.\n")))
}

// --- paragraph scope: pattern with count=each ---

func TestCheck_Paragraph_PatternEach_ExceedsMax(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"pattern": "—", "max": 2, "count": "each"})
	src := "# Title\n\nFirst — second — third — end.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "3")
	assert.Contains(t, diags[0].Message, "max 2")
}

// --- section scope: combined with multiple sections (exercises range-skip) ---

func TestCheck_Section_Combined_MultipleHeadings(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"a", "b"}, "max": 3, "scope": "section", "count": "combined"})
	// section A: "a"×3 + "b"×1 = 4 > max 3; section B: "a"×1 = 1 ≤ max 3
	src := "# A\n\na a a b.\n\n## B\n\na once.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
}

func TestCheck_Section_PatternMultipleHeadings(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"pattern": "—", "max": 2, "scope": "section", "count": "each"})
	// section A: 3 dashes > max 2; section B: 1 dash ≤ max 2
	src := "# A\n\nFirst — second — third — end.\n\n## B\n\nOnly — one.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
}

// TestCheck_Section_NestedHeadingViolatesIndependently pins the
// hierarchical section-window contract astutil.SectionEnd defines: a
// shallow heading's window extends through its nested subsections (so
// its own count legitimately includes their content), but each nested
// subsection must still be checked, and flagged, independently against
// its own narrower window. A cursor optimization that permanently
// consumes paragraphs once one heading's wider window has scanned them
// would silently lose every nested subsection's own violation — this
// was exactly the shape of a regression caught in review. Three
// headings (level 1, 2, 1) each carry their own violating count, so
// all three must be reported.
func TestCheck_Section_NestedHeadingViolatesIndependently(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{
		"tokens": []any{"process"}, "max": 3, "scope": "section", "count": "each",
	})
	src := "# A\n\nprocess process process process.\n\n" +
		"## B\n\nprocess process process process.\n\n" +
		"# C\n\nprocess process process process.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 3, "expected a violation for A, B, and C independently: %+v", diags)
	assert.Equal(t, 1, diags[0].Line, "section A")
	assert.Equal(t, 5, diags[1].Line, "section B")
	assert.Equal(t, 9, diags[2].Line, "section C")
}

// --- private helper unit tests ---

func TestTally(t *testing.T) {
	r := &Rule{Tokens: []string{"foo", "bar"}, CaseSensitive: true}
	totals := make([]int, 2)
	r.tally("foo bar foo", "foo bar foo", totals)
	r.tally("foo", "foo", totals)
	assert.Equal(t, []int{3, 1}, totals)

	re := regexp.MustCompile(`\d+`)
	rp := &Rule{Pattern: re, patternSource: `\d+`}
	ptotals := make([]int, 1)
	rp.tally("1 22 333", "1 22 333", ptotals)
	assert.Equal(t, []int{3}, ptotals)
}

func TestTally_LiteralPatternIgnoresCase(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"pattern": "Foo"})
	require.Equal(t, "Foo", r.literal, "a metacharacter-free pattern takes the literal path")
	totals := make([]int, 1)
	r.tally("foo FOO Foo", "foo foo foo", totals)
	assert.Equal(t, []int{3}, totals)
}

func TestApplySettings_RegexPatternIsNotLiteral(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"pattern": `a.c`})
	assert.Empty(t, r.literal)
}

func TestCheck_NonASCIILowercaseLengthChange(t *testing.T) {
	// "İ" (U+0130) lowercases to a longer byte sequence, so the joined
	// offsets no longer line up and each paragraph is lowercased alone.
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"beta"}, "max": 0})
	diags := r.Check(mustFile(t, "# T\n\nİİİ alpha.\n\nBETA here.\n"))
	require.Len(t, diags, 1)
	assert.Equal(t, 5, diags[0].Line)
}

func TestCountToken(t *testing.T) {
	// case-sensitive: uses Tokens directly
	r := &Rule{Tokens: []string{"foo"}, CaseSensitive: true}
	assert.Equal(t, 2, r.countToken("foo bar foo", 0))
	// case-insensitive: uses lowerTokens on already-lowercased text
	r2 := &Rule{Tokens: []string{"FOO"}, lowerTokens: []string{"foo"}, CaseSensitive: false}
	assert.Equal(t, 2, r2.countToken("foo bar foo", 0))
	// empty token always returns 0
	r3 := &Rule{Tokens: []string{""}, lowerTokens: []string{""}, CaseSensitive: false}
	assert.Equal(t, 0, r3.countToken("foo", 0))
}

func TestCountPattern(t *testing.T) {
	re := regexp.MustCompile(`\d+`)
	r := &Rule{Pattern: re}
	assert.Equal(t, 3, r.countPattern("one 1 two 22 three 333"))
	assert.Equal(t, 0, r.countPattern("no digits here"))
}

func TestDiagEach(t *testing.T) {
	r := &Rule{Min: 1, Max: 3}
	// within [min, max] — no diagnostic
	assert.Empty(t, r.diagEach(2, 5, "file", "foo", "test.md"))
	// exceeds max
	diags := r.diagEach(5, 5, "file", "foo", "test.md")
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "max 3")
	// below min
	diags2 := r.diagEach(0, 5, "file", "foo", "test.md")
	require.Len(t, diags2, 1)
	assert.Contains(t, diags2[0].Message, "min 1")
}

func TestDiagCombined(t *testing.T) {
	r := &Rule{Min: 1, Max: 3}
	// within [min, max] — no diagnostic
	assert.Empty(t, r.diagCombined(2, 1, "file", "test.md"))
	// exceeds max
	diags := r.diagCombined(5, 1, "file", "test.md")
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "max 3")
	assert.Contains(t, diags[0].Message, "tokens") // label defaults to "tokens" when patternSource empty
	// below min
	diags2 := r.diagCombined(0, 1, "file", "test.md")
	require.Len(t, diags2, 1)
	assert.Contains(t, diags2[0].Message, "min 1")
}

func TestBoundMessage(t *testing.T) {
	r := &Rule{Min: 1, Max: 3}
	msg := r.boundMessage("foo", 5, "file")
	assert.Contains(t, msg, "max 3")
	msg2 := r.boundMessage("foo", 0, "file")
	assert.Contains(t, msg2, "min 1")
}

func TestApplyScope(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.applyScope("file"))
	assert.Equal(t, "file", r.Scope)
	require.NoError(t, r.applyScope("section"))
	require.NoError(t, r.applyScope("paragraph"))
	require.Error(t, r.applyScope("block"))
	require.Error(t, r.applyScope(42))
}

func TestApplyTokens(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.applyTokens([]any{"foo", "bar"}))
	assert.Equal(t, []string{"foo", "bar"}, r.Tokens)
	require.Error(t, r.applyTokens("not-a-slice"))
}

func TestExtractPattern(t *testing.T) {
	s, err := extractPattern("foo+")
	require.NoError(t, err)
	assert.Equal(t, "foo+", s)
	_, err = extractPattern(42)
	require.Error(t, err)
}

func TestApplyMin(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.applyMin(3))
	assert.Equal(t, 3, r.Min)
	require.Error(t, r.applyMin(-1))
	require.Error(t, r.applyMin("two"))
}

func TestApplyMax(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.applyMax(5))
	assert.Equal(t, 5, r.Max)
	require.Error(t, r.applyMax("five"))
}

func TestApplyCount(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.applyCount("each"))
	assert.Equal(t, "each", r.Count)
	require.NoError(t, r.applyCount("combined"))
	require.Error(t, r.applyCount("all"))
	require.Error(t, r.applyCount(42))
}

func TestApplyCaseSensitive(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.applyCaseSensitive(true))
	assert.True(t, r.CaseSensitive)
	require.NoError(t, r.applyCaseSensitive(false))
	assert.False(t, r.CaseSensitive)
	require.Error(t, r.applyCaseSensitive("yes"))
}

func TestFinalizeSettings(t *testing.T) {
	// empty rawPattern, no tokens — no error
	r := &Rule{}
	require.NoError(t, r.finalizeSettings(""))

	// valid pattern — Pattern compiled and set
	r2 := &Rule{}
	require.NoError(t, r2.finalizeSettings("qux+"))
	assert.NotNil(t, r2.Pattern)

	// tokens + non-empty rawPattern — mutually exclusive error; Pattern must
	// not be left non-nil on the Rule after the error return (a second
	// ApplySettings call that omits "pattern" would not reset it otherwise).
	r3 := &Rule{Tokens: []string{"foo"}}
	require.Error(t, r3.finalizeSettings("foo+"))
	assert.Nil(t, r3.Pattern, "Pattern must be cleared on mutual-exclusion error")

	// invalid pattern — compile error
	r4 := &Rule{}
	require.Error(t, r4.finalizeSettings("[invalid"))

	// case-insensitive tokens — lowerTokens built
	r5 := &Rule{Tokens: []string{"FOO", "Bar"}, CaseSensitive: false}
	require.NoError(t, r5.finalizeSettings(""))
	assert.Equal(t, []string{"foo", "bar"}, r5.lowerTokens)
}

func TestCompileAndSetPattern(t *testing.T) {
	// happy path — pattern compiled, source stored
	r := &Rule{}
	require.NoError(t, r.compileAndSetPattern("uniquehelper1+"))
	assert.NotNil(t, r.Pattern)
	assert.Equal(t, "uniquehelper1+", r.patternSource)

	// invalid regex — error returned
	r2 := &Rule{}
	require.Error(t, r2.compileAndSetPattern("[bad"))

	// case-sensitive: compiled string has no (?i) prefix
	r3 := &Rule{CaseSensitive: true}
	require.NoError(t, r3.compileAndSetPattern("exactpat1"))
	assert.Equal(t, "exactpat1", r3.Pattern.String())

	// case-insensitive: compiled string gains (?i) prefix
	r4 := &Rule{CaseSensitive: false}
	require.NoError(t, r4.compileAndSetPattern("lowerpat1"))
	assert.Equal(t, "(?i)lowerpat1", r4.Pattern.String())
}

func TestCheck_Section_EmptyBodyUnderMin(t *testing.T) {
	// A section with no paragraphs still has a count of zero, which
	// falls below min.
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"scope"}, "min": 1, "scope": "section"})
	diags := r.Check(mustFile(t, "# Only a heading\n"))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "min 1")
}

func TestCheck_SoftLineBreakSeparatesWords(t *testing.T) {
	// A soft break joins two source lines with a space, so a token
	// spanning the break still matches as written.
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"alpha beta"}, "max": 0})
	diags := r.Check(mustFile(t, "# T\n\nalpha\nbeta here.\n"))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"alpha beta" appears 1 time(s)`)
}

func TestCheck_TableParagraphNotCounted(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": []any{"cell"}, "max": 0})
	f := mustFile(t, "# T\n\n| cell | cell |\n|------|------|\n| cell | cell |\n")
	assert.Empty(t, r.Check(f))
}

func TestAppendProse_StringNode(t *testing.T) {
	// Typographer-style extensions emit ast.String nodes whose payload
	// lives on the node, not in the source.
	p := ast.NewParagraph()
	p.AppendChild(p, ast.NewString([]byte("synergy")))
	assert.Equal(t, "synergy", string(appendProse(nil, p, nil)))
}

func TestCheck_ManyTokensBeyondStackTotals(t *testing.T) {
	toks := make([]any, 20)
	for i := range toks {
		toks[i] = fmt.Sprintf("t%02d", i)
	}
	r := &Rule{}
	mustApply(t, r, map[string]any{"tokens": toks, "max": 0})
	diags := r.Check(mustFile(t, "# T\n\nt19 appears.\n"))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"t19"`)
}

func TestCheck_LiteralPatternCaseSensitive(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"pattern": "Foo", "case-sensitive": true, "max": 1})
	assert.Empty(t, r.Check(mustFile(t, "# T\n\nFoo foo FOO.\n")), "only the exact-case match counts")
	diags := r.Check(mustFile(t, "# T\n\nFoo Foo.\n"))
	require.Len(t, diags, 1)
}
