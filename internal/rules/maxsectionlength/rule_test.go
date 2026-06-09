package maxsectionlength

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/yuin/goldmark/ast"
	gtext "github.com/yuin/goldmark/text"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func TestCheck_NoHeadings_NoDiagnostic(t *testing.T) {
	f := mustFile(t, "just some text\nand more\n")
	r := &Rule{Max: 5}
	assert.Empty(t, r.Check(f))
}

func TestCheck_SectionUnderLimit_NoDiagnostic(t *testing.T) {
	src := "# Title\nline 1\nline 2\n"
	r := &Rule{Max: 5}
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_SectionOverLimit_Diagnostic(t *testing.T) {
	src := "# Title\na\nb\nc\nd\ne\n"
	r := &Rule{Max: 3}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	d := diags[0]
	assert.Equal(t, "MDS036", d.RuleID)
	assert.Equal(t, "max-section-length", d.RuleName)
	assert.Equal(t, lint.Warning, d.Severity)
	assert.Equal(t, 1, d.Line)
	assert.Equal(t, 1, d.Column)
	assert.Contains(t, d.Message, "# Title")
	assert.Contains(t, d.Message, "6 > 3")
}

func TestCheck_SectionEndsAtNextHeading(t *testing.T) {
	src := "# A\nx\ny\n# B\nz\n"
	r := &Rule{Max: 2}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "# A")
	assert.Contains(t, diags[0].Message, "3 > 2")
}

func TestCheck_SubsectionExcludedFromParent(t *testing.T) {
	src := "# H1\n## H2\na\nb\nc\nd\ne\n"
	r := &Rule{Max: 3}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "## H2")
	assert.Contains(t, diags[0].Message, "6 > 3")
}

func TestCheck_PerLevelOverridesDefault(t *testing.T) {
	src := "# H1\na\nb\nc\n## H2\nx\ny\nz\n"
	r := &Rule{Max: 10, PerLevel: map[int]int{2: 2}}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "## H2")
	assert.Contains(t, diags[0].Message, "4 > 2")
}

func TestCheck_PerHeadingOverridesLevel(t *testing.T) {
	src := "## Intro\na\nb\nc\nd\n## Other\nx\ny\nz\n"
	r := &Rule{
		PerLevel: map[int]int{2: 2},
		PerHeading: []HeadingPattern{
			{Pattern: "^Intro$", Regex: regexp.MustCompile("^Intro$"), Max: 10},
		},
	}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "## Other")
}

func TestCheck_ZeroMaxMeansNoLimit(t *testing.T) {
	src := "# Title\n" + strings.Repeat("x\n", 1000)
	r := &Rule{Max: 0}
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_HeadingInCodeBlockIgnored(t *testing.T) {
	src := "# Real\nline1\nline2\n```\n# Not a heading\n```\nline3\n"
	r := &Rule{Max: 3}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "# Real")
}

func TestApplySettings_Max(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max": 50})
	require.NoError(t, err)
	assert.Equal(t, 50, r.Max)
}

func TestApplySettings_PerLevel(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-level": map[string]any{"1": 100, "2": 50},
	})
	require.NoError(t, err)
	assert.Equal(t, 100, r.PerLevel[1])
	assert.Equal(t, 50, r.PerLevel[2])
}

func TestApplySettings_PerHeading(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-heading": []any{
			map[string]any{"pattern": "^Intro$", "max": 10},
		},
	})
	require.NoError(t, err)
	require.Len(t, r.PerHeading, 1)
	assert.Equal(t, 10, r.PerHeading[0].Max)
	assert.NotNil(t, r.PerHeading[0].Regex)
}

func TestApplySettings_InvalidMaxType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max": "abc"})
	assert.Error(t, err)
}

func TestApplySettings_InvalidPerLevelKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-level": map[string]any{"notanint": 5},
	})
	assert.Error(t, err)
}

func TestApplySettings_InvalidPerLevelRange(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-level": map[string]any{"7": 5},
	})
	assert.Error(t, err)
}

func TestApplySettings_InvalidPerHeadingRegex(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-heading": []any{
			map[string]any{"pattern": "[unterminated", "max": 10},
		},
	})
	assert.Error(t, err)
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": 1})
	assert.Error(t, err)
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	assert.Equal(t, 0, ds["max"])
	assert.Equal(t, map[string]any{}, ds["per-level"])
	assert.Equal(t, []any{}, ds["per-heading"])
	assert.Equal(t, 0, ds["max-words"])
	assert.Equal(t, 0, ds["min-words"])
	assert.Equal(t, 0, ds["max-paragraphs"])
}

func TestHeadingLine_FallbackToTextChild(t *testing.T) {
	src := []byte("# Hi\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	// Build a heading with no Lines() but with a Text child, to exercise
	// the defensive fallback.
	h := ast.NewHeading(1)
	text := ast.NewTextSegment(gtext.NewSegment(2, 4))
	h.AppendChild(h, text)
	assert.Equal(t, 1, headingLine(h, f))
}

func TestHeadingLine_NoLinesNoChildren(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte(""))
	require.NoError(t, err)
	h := ast.NewHeading(1)
	assert.Equal(t, 1, headingLine(h, f))
}

func TestApplyDefaultSettings_ClearsPerLevelAndPerHeading(t *testing.T) {
	r := &Rule{
		Max:      10,
		PerLevel: map[int]int{2: 3},
		PerHeading: []HeadingPattern{
			{Pattern: "x", Regex: regexp.MustCompile("x"), Max: 1},
		},
	}
	require.NoError(t, r.ApplySettings(r.DefaultSettings()))
	assert.Equal(t, 0, r.Max)
	assert.Empty(t, r.PerLevel)
	assert.Empty(t, r.PerHeading)
}

func TestID(t *testing.T) {
	assert.Equal(t, "MDS036", (&Rule{}).ID())
}

func TestName(t *testing.T) {
	assert.Equal(t, "max-section-length", (&Rule{}).Name())
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "heading", (&Rule{}).Category())
}

func TestEnabledByDefault(t *testing.T) {
	assert.False(t, (&Rule{}).EnabledByDefault())
}

func TestCheck_NilAST_NoDiagnostic(t *testing.T) {
	r := &Rule{Max: 5}
	assert.Empty(t, r.Check(&lint.File{}))
}

func TestCheck_FileWithoutTrailingNewline(t *testing.T) {
	// No trailing newline — last Lines entry is non-empty.
	f, err := lint.NewFile("t.md", []byte("# A\nline"))
	require.NoError(t, err)
	r := &Rule{Max: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "2 > 1")
}

func TestCheck_PerHeadingNoMatchFallsBackToLevel(t *testing.T) {
	src := "## Alpha\na\nb\nc\n"
	r := &Rule{
		PerLevel: map[int]int{2: 2},
		PerHeading: []HeadingPattern{
			{Pattern: "^Zeta$", Regex: regexp.MustCompile("^Zeta$"), Max: 100},
		},
	}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "## Alpha")
}

func TestApplySettings_NegativeMax(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max": -1})
	assert.Error(t, err)
}

func TestApplySettings_Int64Max(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max": int64(42)})
	require.NoError(t, err)
	assert.Equal(t, 42, r.Max)
}

func TestApplySettings_Float64IntegerMax(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max": float64(25)})
	require.NoError(t, err)
	assert.Equal(t, 25, r.Max)
}

func TestApplySettings_Float64NonIntegerMax(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max": 3.5})
	assert.Error(t, err)
}

func TestApplySettings_PerLevelNotMap(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"per-level": "not a map"})
	assert.Error(t, err)
}

func TestApplySettings_PerLevelValueNotInt(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-level": map[string]any{"2": "nope"},
	})
	assert.Error(t, err)
}

func TestApplySettings_PerLevelNegativeValue(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-level": map[string]any{"2": -3},
	})
	assert.Error(t, err)
}

func TestApplySettings_PerHeadingNotList(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"per-heading": "nope"})
	assert.Error(t, err)
}

func TestApplySettings_PerHeadingItemNotMap(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-heading": []any{"not a map"},
	})
	assert.Error(t, err)
}

func TestApplySettings_PerLevelFromInterfaceKeyedMap(t *testing.T) {
	// YAML decoded into `any` can produce map[any]any for nested maps.
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-level": map[any]any{2: 3},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, r.PerLevel[2])
}

func TestApplySettings_PerHeadingFromInterfaceKeyedMap(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-heading": []any{
			map[any]any{"pattern": "^Intro$", "max": 7},
		},
	})
	require.NoError(t, err)
	require.Len(t, r.PerHeading, 1)
	assert.Equal(t, 7, r.PerHeading[0].Max)
}

func TestApplySettings_PerHeadingMissingPattern(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-heading": []any{map[string]any{"max": 10}},
	})
	assert.Error(t, err)
}

func TestApplySettings_PerHeadingMissingMax(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-heading": []any{map[string]any{"pattern": "x"}},
	})
	assert.Error(t, err)
}

func TestApplySettings_PerHeadingNegativeMax(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"per-heading": []any{
			map[string]any{"pattern": "x", "max": -1},
		},
	})
	assert.Error(t, err)
}

// --- max-words / min-words / max-paragraphs ---

func TestCheck_MaxWordsOverLimit_Diagnostic(t *testing.T) {
	src := "# Title\n\n" + strings.Repeat("word ", 60) + "\n"
	r := &Rule{MaxWords: 50}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "# Title")
	assert.Contains(t, diags[0].Message, "too many words")
	assert.Contains(t, diags[0].Message, "60 > 50")
}

func TestCheck_MaxWordsUnderLimit_NoDiagnostic(t *testing.T) {
	src := "# Title\n\nshort body of a few words.\n"
	r := &Rule{MaxWords: 50}
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_MinWordsUnderLimit_Diagnostic(t *testing.T) {
	src := "# Title\n\nonly three words here.\n"
	r := &Rule{MinWords: 10}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "# Title")
	assert.Contains(t, diags[0].Message, "too few words")
	assert.Contains(t, diags[0].Message, "4 < 10")
}

func TestCheck_MinWordsMet_NoDiagnostic(t *testing.T) {
	src := "# Title\n\n" + strings.Repeat("word ", 12) + "\n"
	r := &Rule{MinWords: 10}
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_MaxParagraphsOverLimit_Diagnostic(t *testing.T) {
	src := "# Title\n\nfirst.\n\nsecond.\n\nthird.\n\nfourth.\n"
	r := &Rule{MaxParagraphs: 3}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "# Title")
	assert.Contains(t, diags[0].Message, "too many paragraphs")
	assert.Contains(t, diags[0].Message, "4 > 3")
}

func TestCheck_MaxParagraphsUnderLimit_NoDiagnostic(t *testing.T) {
	src := "# Title\n\nfirst.\n\nsecond.\n"
	r := &Rule{MaxParagraphs: 3}
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_SubsectionWordsNotCountedAgainstParent(t *testing.T) {
	src := "# H1\n\nparent short body.\n\n## H2\n\n" + strings.Repeat("word ", 60) + "\n"
	r := &Rule{MaxWords: 10}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "## H2")
}

func TestCheck_MaxAndWordsBothFire(t *testing.T) {
	src := "# T\n\n" + strings.Repeat("word\n", 60) + "\n"
	r := &Rule{Max: 3, MaxWords: 10}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 2)
	// Order: line-limit first, then word-limit (matches check order).
	assert.Contains(t, diags[0].Message, "too long")
	assert.Contains(t, diags[1].Message, "too many words")
}

func TestApplySettings_MaxWords(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-words": 100})
	require.NoError(t, err)
	assert.Equal(t, 100, r.MaxWords)
}

func TestApplySettings_MinWords(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-words": 5})
	require.NoError(t, err)
	assert.Equal(t, 5, r.MinWords)
}

func TestApplySettings_MaxParagraphs(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-paragraphs": 3})
	require.NoError(t, err)
	assert.Equal(t, 3, r.MaxParagraphs)
}

func TestApplySettings_NegativeMaxWords(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-words": -1})
	assert.Error(t, err)
}

func TestApplySettings_NegativeMinWords(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-words": -1})
	assert.Error(t, err)
}

func TestApplySettings_NegativeMaxParagraphs(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-paragraphs": -1})
	assert.Error(t, err)
}

func TestApplySettings_NonIntegerMinWords(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-words": "ten"})
	assert.Error(t, err)
}

func TestApplySettings_NonIntegerMaxParagraphs(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-paragraphs": "three"})
	assert.Error(t, err)
}

func TestApplyDefaultSettings_ClearsWordAndParagraphCaps(t *testing.T) {
	r := &Rule{MaxWords: 10, MinWords: 5, MaxParagraphs: 3}
	require.NoError(t, r.ApplySettings(r.DefaultSettings()))
	assert.Equal(t, 0, r.MaxWords)
	assert.Equal(t, 0, r.MinWords)
	assert.Equal(t, 0, r.MaxParagraphs)
}

func TestCheck_TableNotCountedAsParagraph(t *testing.T) {
	// Goldmark parses tables as paragraphs when the table extension
	// is absent. Without filtering, each row would inflate the
	// paragraph count and the cell text would inflate word counts.
	src := "# Title\n\nfirst.\n\n| col |\n| --- |\n| val |\n"
	r := &Rule{MaxParagraphs: 1}
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_TableWordsNotCounted(t *testing.T) {
	// Table cell contents should not contribute to the word count.
	src := "# Title\n\n| a | b | c | d | e | f |\n| - | - | - | - | - | - |\n| 1 | 2 | 3 | 4 | 5 | 6 |\n"
	r := &Rule{MaxWords: 5}
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestApplySettings_PerHeadingCachesCompiledPattern(t *testing.T) {
	t.Parallel()
	// Two sequential ApplySettings calls with the same pattern must return
	// the same *regexp.Regexp pointer — the second LoadOrStore call hits the
	// loaded=true branch and discards the freshly compiled regexp in favour
	// of the cached one.
	const pat = `^CacheTestPatternMSL\d+$`
	s := map[string]any{
		"per-heading": []any{
			map[string]any{"pattern": pat, "max": 5},
		},
	}
	r1, r2 := &Rule{}, &Rule{}
	require.NoError(t, r1.ApplySettings(s))
	require.NoError(t, r2.ApplySettings(s))
	require.Len(t, r1.PerHeading, 1)
	require.Len(t, r2.PerHeading, 1)
	assert.Same(t, r1.PerHeading[0].Regex, r2.PerHeading[0].Regex,
		"expected second call to reuse the cached *regexp.Regexp")
}
