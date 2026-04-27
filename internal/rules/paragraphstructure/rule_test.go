package paragraphstructure

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck_TooManySentences(t *testing.T) {
	// 8 sentences, default max is 6.
	src := []byte("One. Two. Three. Four. Five. Six. Seven. Eight.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %v", len(diags), diags)
	d := diags[0]
	if d.RuleID != "MDS024" {
		t.Errorf("expected rule ID MDS024, got %s", d.RuleID)
	}
	assert.Contains(t, d.Message, "too many sentences", "unexpected message: %s", d.Message)
	assert.Contains(t, d.Message, "8 > 6", "expected count in message, got: %s", d.Message)
}

func TestCheck_UnderSentenceLimit(t *testing.T) {
	src := []byte("One. Two. Three.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diags)
}

func TestCheck_SentenceTooLong(t *testing.T) {
	// Build a sentence with 45 words.
	words := make([]string, 45)
	for i := range words {
		words[i] = "word"
	}
	src := []byte(strings.Join(words, " ") + ".\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %v", len(diags), diags)
	assert.Contains(t, diags[0].Message, "sentence too long", "unexpected message: %s", diags[0].Message)
	assert.Contains(t, diags[0].Message, "45 > 40", "expected count in message, got: %s", diags[0].Message)
	assert.Contains(t, diags[0].Message, "word word word word word",
		"expected sentence preview in message, got: %s", diags[0].Message)
}

func TestCheck_SentenceTooLong_ShowsPreview(t *testing.T) {
	src := []byte("The quick brown fox jumped over the lazy dog " +
		"and kept running through the meadow until it reached " +
		"the very end of the long winding road that stretched " +
		"far beyond the hills. Short.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 10, MaxWords: 10}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %v", len(diags), diags)
	// Should show first ~10 words of the offending sentence as preview.
	assert.Contains(t, diags[0].Message, "\"The quick brown fox jumped over the lazy dog and ...\"",
		"expected truncated preview, got: %s", diags[0].Message)
}

func TestCheck_BothLimitsExceeded(t *testing.T) {
	// 8 sentences (exceeds max 6) and one sentence with 45 words (exceeds max 40).
	words := make([]string, 45)
	for i := range words {
		words[i] = "word"
	}
	longSent := strings.Join(words, " ") + "."
	src := []byte(longSent + " Two. Three. Four. Five. Six. Seven. Eight.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 2, "expected 2 diagnostics, got %d: %v", len(diags), diags)
	hasSentences := false
	hasWords := false
	for _, d := range diags {
		if strings.Contains(d.Message, "too many sentences") {
			hasSentences = true
		}
		if strings.Contains(d.Message, "sentence too long") {
			hasWords = true
		}
	}
	assert.True(t, hasSentences, "missing 'too many sentences' diagnostic")
	assert.True(t, hasWords, "missing 'sentence too long' diagnostic")
}

func TestCheck_CustomSettings(t *testing.T) {
	src := []byte("One. Two. Three. Four.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 3, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %v", len(diags), diags)
	assert.Contains(t, diags[0].Message, "4 > 3", "expected custom limit in message, got: %s", diags[0].Message)
}

func TestCheck_ShortParagraph(t *testing.T) {
	src := []byte("Short.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diags)
}

func TestCheck_DiagnosticFields(t *testing.T) {
	src := []byte("One. Two. Three. Four. Five. Six. Seven. Eight.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	d := diags[0]
	if d.Line != 1 {
		t.Errorf("expected line 1, got %d", d.Line)
	}
	if d.Column != 1 {
		t.Errorf("expected column 1, got %d", d.Column)
	}
	if d.RuleName != "paragraph-structure" {
		t.Errorf("expected rule name paragraph-structure, got %s", d.RuleName)
	}
	if d.Severity != lint.Warning {
		t.Errorf("expected severity warning, got %s", d.Severity)
	}
}

func TestCheck_TableSkipped(t *testing.T) {
	// A markdown table parsed as a paragraph should be skipped.
	src := []byte("| A | B | C | D | E | F | G | H |\n" +
		"|---|---|---|---|---|---|---|---|\n" +
		"| one | two | three | four | five | six | seven | eight |\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 1, MaxWords: 1}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for table, got %d", len(diags))
}

func TestApplySettings_Valid(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{
		"max-sentences":          10,
		"max-words-per-sentence": 50,
	})
	require.NoError(t, err, "unexpected error: %v", err)
	if r.MaxSentences != 10 {
		t.Errorf("expected MaxSentences=10, got %d", r.MaxSentences)
	}
	if r.MaxWords != 50 {
		t.Errorf("expected MaxWords=50, got %d", r.MaxWords)
	}
}

func TestApplySettings_InvalidType(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{"max-sentences": "not-a-number"})
	require.Error(t, err, "expected error for non-int max-sentences")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{"unknown": 1})
	require.Error(t, err, "expected error for unknown setting")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["max-sentences"] != 6 {
		t.Errorf("expected max-sentences=6, got %v", ds["max-sentences"])
	}
	if ds["max-words-per-sentence"] != 40 {
		t.Errorf("expected max-words-per-sentence=40, got %v", ds["max-words-per-sentence"])
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS024" {
		t.Errorf("expected MDS024, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "paragraph-structure" {
		t.Errorf("expected paragraph-structure, got %s", r.Name())
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() != "meta" {
		t.Errorf("expected meta, got %s", r.Category())
	}
}

// --- Placeholder tests ---

func TestCheck_Placeholder_VarToken_MaskedToWord(t *testing.T) {
	// A paragraph consisting only of a var-token placeholder is masked to
	// "word" (one word, no punctuation), well below max-sentences and max-words,
	// so no diagnostic is produced.
	src := []byte("# Title\n\n{body}\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40, Placeholders: []string{"var-token"}}
	diags := r.Check(f)
	require.Empty(t, diags, "var-token paragraph masked to neutral word should produce no diagnostic")
}

func TestCheck_Placeholder_EmptyList_StructureChecksRun(t *testing.T) {
	// Without placeholders configured, behavior is unchanged.
	// A paragraph with many sentences still gets flagged.
	src := []byte("# Title\n\nFirst. Second. Third. Fourth. Fifth. Sixth. Seventh.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxSentences: 6, MaxWords: 40, Placeholders: []string{}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "over-sentence paragraph should still be flagged without placeholders")
}

func TestApplySettings_Placeholders_ParagraphStructure(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{
		"placeholders": []any{"var-token"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"var-token"}, r.Placeholders)
}

func TestApplySettings_Placeholders_UnknownToken_ParagraphStructure(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{"placeholders": []any{"bad"}})
	require.Error(t, err)
}

func TestApplySettings_Placeholders_NonList_ParagraphStructure(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{"placeholders": "not-a-list"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "list of strings")
}

func TestDefaultSettings_ParagraphStructure_IncludesPlaceholders(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	require.Equal(t, []string{}, ds["placeholders"])
}

func TestSettingMergeMode_ParagraphStructure(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("placeholders"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("max-sentences"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("unknown"))
}
