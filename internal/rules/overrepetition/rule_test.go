package overrepetition

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
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
	assert.Equal(t, "MDS075", (&Rule{}).ID())
}

func TestName(t *testing.T) {
	assert.Equal(t, "over-repetition", (&Rule{}).Name())
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "prose", (&Rule{}).Category())
}

func TestEnabledByDefault(t *testing.T) {
	assert.False(t, (&Rule{}).EnabledByDefault())
}

func TestWordlistTarget(t *testing.T) {
	assert.Equal(t, "stopwords", (&Rule{}).WordlistTarget())
}

// --- early exits ---

func TestCheck_NilAST_NoDiagnostic(t *testing.T) {
	r := &Rule{Max: 3, Scope: "section"}
	assert.Empty(t, r.Check(&lint.File{}))
}

func TestCheck_MaxZero_NoDiagnostic(t *testing.T) {
	// Max == 0 (zero value) means unconfigured — skip
	r := &Rule{}
	f := mustFile(t, "# Title\n\nword word word word word.\n")
	assert.Empty(t, r.Check(f))
}

func TestCheck_MaxNegative_NoDiagnostic(t *testing.T) {
	// Max == -1 (from DefaultSettings) means no ceiling — skip
	r := &Rule{Scope: "section"}
	mustApply(t, r, map[string]any{"max": -1})
	f := mustFile(t, "# Title\n\nword word word word word.\n")
	assert.Empty(t, r.Check(f))
}

// --- paragraph scope ---

func TestCheck_Paragraph_UnderMax_NoDiagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"scope": "paragraph", "max": 4, "min-length": 4})
	src := "# Title\n\nprocess process process result.\n"
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_Paragraph_ExceedsMax_Diagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"scope": "paragraph", "max": 3, "min-length": 4})
	src := "# Title\n\nprocess process process process result.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS075", diags[0].RuleID)
	assert.Equal(t, 3, diags[0].Line)
	assert.Contains(t, diags[0].Message, "process")
	assert.Contains(t, diags[0].Message, "4")
	assert.Contains(t, diags[0].Message, "max 3")
}

// --- section scope (default) ---

func TestCheck_Section_ExceedsMax_Diagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"scope": "section", "max": 3, "min-length": 4})
	// Heading is at line 1; first paragraph is at line 3. Diagnostic anchors at
	// the first paragraph, not the heading, so editors navigate to prose.
	src := "# Section\n\nprocess process.\n\nprocess process result.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	assert.Contains(t, diags[0].Message, "process")
}

func TestCheck_Section_NoHeadings_TreatsFileAsOneSection(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"scope": "section", "max": 2, "min-length": 4})
	// A headingless file is treated as one implicit preamble section, not skipped.
	src := "word word word.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Contains(t, diags[0].Message, "word")
}

func TestCheck_Section_NoHeadings_UnderMax_NoDiagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"scope": "section", "max": 3, "min-length": 4})
	src := "word word.\n"
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_Section_PreambleCounted(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"scope": "section", "max": 3, "min-length": 4})
	// Prose before the first heading must be counted as an implicit preamble section.
	src := "process process process process.\n\n# Section\n\nonly once.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Contains(t, diags[0].Message, "process")
}

func TestCheck_Section_AllParagraphsBeforeHeading(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"scope": "section", "max": 3, "min-length": 4})
	// All paragraphs are before the first heading: slices.IndexFunc returns -1,
	// so preambleEnd falls back to len(paragraphs) and the whole file is preamble.
	src := "word word word word.\n\n# Section\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Contains(t, diags[0].Message, "word")
}

// --- file scope ---

func TestCheck_File_ExceedsMax_Diagnostic(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"scope": "file", "max": 3, "min-length": 4})
	src := "# Title\n\nprocess process.\n\n## Next\n\nprocess process result.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Contains(t, diags[0].Message, "process")
}

// --- min-length ---

func TestCheck_MinLength_ExcludesShortWords(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"scope": "paragraph", "max": 2, "min-length": 5})
	// "word" has 4 runes, "words" has 5 — only "words" should be counted
	src := "# Title\n\nword word word words words words.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "words")
	for _, d := range diags {
		assert.NotContains(t, d.Message, `"word"`)
	}
}

// --- stopwords ---

func TestCheck_Stopwords_ExcludeFromCount(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{
		"scope":      "paragraph",
		"max":        2,
		"min-length": 4,
		"stopwords":  []any{"process"},
	})
	src := "# Title\n\nprocess process process result.\n"
	// "process" is stopword — should not be flagged
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_Stopwords_CaseInsensitive(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{
		"scope":      "paragraph",
		"max":        2,
		"min-length": 4,
		"stopwords":  []any{"PROCESS"},
	})
	src := "# Title\n\nprocess process process result.\n"
	// Stopword list is case-folded, so "PROCESS" matches "process"
	assert.Empty(t, r.Check(mustFile(t, src)))
}

// --- settings errors ---

func TestApplySettings_UnknownKey_Error(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"bogus": "x"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestApplySettings_BadScope_Error(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"scope": "word"})
	assert.Error(t, err)
}

func TestApplySettings_BadMax_Error(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max": "not-a-number"})
	assert.Error(t, err)
}

func TestApplySettings_MaxZero_Error(t *testing.T) {
	// max:0 is ambiguous (0 is the unconfigured Go zero value; the
	// disabled sentinel is -1). Reject it explicitly so callers get a
	// clear error instead of silently disabled behaviour.
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max": 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "got 0")
}

func TestApplySettings_MaxNegativeOtherThanMinusOne_Error(t *testing.T) {
	// Any negative value except -1 is undocumented; reject with a clear message.
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max": -5})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "got -5")
}

func TestApplySettings_BadMinLength_Error(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-length": -1})
	assert.Error(t, err)
}

func TestApplySettings_BadStopwords_Error(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"stopwords": "not-a-list"})
	assert.Error(t, err)
}

func TestApplySettings_ScopeNonString_Error(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"scope": 42})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scope must be a string")
}

func TestApplySettings_MinLengthNonInteger_Error(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-length": "four"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "min-length must be an integer")
}

func TestCheck_MultipleViolations_SortedByMessage(t *testing.T) {
	r := &Rule{}
	mustApply(t, r, map[string]any{"scope": "paragraph", "max": 1, "min-length": 4})
	// Both "alpha" and "bravo" exceed max=1; diagFromFreq must sort them
	// so cmpDiagMessage is actually invoked.
	src := "# Title\n\nalpha alpha bravo bravo.\n"
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 2)
	assert.Less(t, diags[0].Message, diags[1].Message)
}

func TestDefaultSettings(t *testing.T) {
	defaults := (&Rule{}).DefaultSettings()
	assert.Equal(t, "section", defaults["scope"])
	assert.Equal(t, -1, defaults["max"])
	assert.Equal(t, 4, defaults["min-length"])
}
