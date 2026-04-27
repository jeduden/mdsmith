package paragraphreadability

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hardText returns a paragraph with long, complex words that yields
// a high ARI score (well above 14.0).
func hardText() string {
	return "The implementation of concurrent distributed systems " +
		"requires sophisticated understanding of fundamental " +
		"computational paradigms and synchronization mechanisms " +
		"that must guarantee linearizability across heterogeneous " +
		"processing environments and architectural configurations."
}

// easyText returns a simple paragraph that yields a low ARI score.
func easyText() string {
	return "The cat sat on the mat and the dog lay on the rug. " +
		"They were both very happy to be at home on a warm day."
}

func TestCheck_OverThreshold(t *testing.T) {
	src := []byte(hardText() + "\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxIndex: 14.0, MinWords: 20, Index: ARI}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	d := diags[0]
	if d.RuleID != "MDS023" {
		t.Errorf("expected rule ID MDS023, got %s", d.RuleID)
	}
	if d.RuleName != "paragraph-readability" {
		t.Errorf(
			"expected rule name paragraph-readability, got %s",
			d.RuleName,
		)
	}
	if d.Severity != lint.Warning {
		t.Errorf("expected severity warning, got %s", d.Severity)
	}
	if d.Column != 1 {
		t.Errorf("expected column 1, got %d", d.Column)
	}
	if !strings.Contains(d.Message, "max 14.0") {
		t.Errorf(
			"expected message to contain 'max 14.0', got %q",
			d.Message,
		)
	}
	if !strings.Contains(d.Message, "avg sentence length") {
		t.Errorf(
			"expected message to contain avg sentence length hint, got %q",
			d.Message,
		)
	}
}

func TestCheck_UnderThreshold(t *testing.T) {
	src := []byte(easyText() + "\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxIndex: 14.0, MinWords: 20, Index: ARI}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_ShortParagraphSkipped(t *testing.T) {
	// Fewer than 20 words: should be skipped regardless of ARI.
	src := []byte("One two three four five six seven eight.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	// Use an index function that always returns a high score.
	alwaysHigh := func(_ string) float64 { return 99.0 }
	r := &Rule{MaxIndex: 14.0, MinWords: 20, Index: alwaysHigh}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf(
			"expected 0 diagnostics for short paragraph, got %d",
			len(diags),
		)
	}
}

func TestCheck_InlineMarkupStripped(t *testing.T) {
	// The same hard text but with inline markup should still trigger.
	src := []byte(
		"The **implementation** of *concurrent* distributed " +
			"systems requires `sophisticated` understanding of " +
			"fundamental computational paradigms and " +
			"synchronization mechanisms that must guarantee " +
			"linearizability across heterogeneous processing " +
			"environments and architectural configurations.\n",
	)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxIndex: 14.0, MinWords: 20, Index: ARI}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf(
			"expected 1 diagnostic for marked-up hard text, got %d",
			len(diags),
		)
	}
}

func TestCheck_DiagnosticLine(t *testing.T) {
	// The diagnostic should point to the correct line.
	src := []byte("# Heading\n\n" + hardText() + "\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxIndex: 14.0, MinWords: 20, Index: ARI}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_NilIndexUsesARI(t *testing.T) {
	src := []byte(hardText() + "\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxIndex: 14.0, MinWords: 20, Index: nil}
	diags := r.Check(f)
	// Hard text with ARI should trigger.
	if len(diags) != 1 {
		t.Fatalf(
			"expected 1 diagnostic with nil Index, got %d",
			len(diags),
		)
	}
}

func TestCheck_TableSkipped(t *testing.T) {
	// A markdown table parsed as a paragraph should be skipped.
	src := []byte("| Setting | Type | Default | Description |\n" +
		"|---------|------|---------|-------------|\n" +
		"| `max` | int | 80 | Maximum allowed line length |\n" +
		"| `heading-max` | int | -- | Max length for heading lines |\n" +
		"| `code-block-max` | int | -- | Max length for code block lines |\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	alwaysHigh := func(_ string) float64 { return 99.0 }
	r := &Rule{MaxIndex: 14.0, MinWords: 1, Index: alwaysHigh}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for table, got %d", len(diags))
}

// --- Configurable tests ---

func TestApplySettings_ValidMaxIndex(t *testing.T) {
	r := &Rule{MaxIndex: 14.0, MinWords: 20}
	err := r.ApplySettings(map[string]any{"max-index": 10.0})
	require.NoError(t, err, "unexpected error: %v", err)
	if r.MaxIndex != 10.0 {
		t.Errorf("expected MaxIndex=10.0, got %f", r.MaxIndex)
	}
}

func TestApplySettings_ValidMinWords(t *testing.T) {
	r := &Rule{MaxIndex: 14.0, MinWords: 20}
	err := r.ApplySettings(map[string]any{"min-words": 30})
	require.NoError(t, err, "unexpected error: %v", err)
	if r.MinWords != 30 {
		t.Errorf("expected MinWords=30, got %d", r.MinWords)
	}
}

func TestApplySettings_InvalidMaxIndexType(t *testing.T) {
	r := &Rule{MaxIndex: 14.0, MinWords: 20}
	err := r.ApplySettings(map[string]any{"max-index": "high"})
	require.Error(t, err, "expected error for non-number max-index")
}

func TestApplySettings_InvalidMinWordsType(t *testing.T) {
	r := &Rule{MaxIndex: 14.0, MinWords: 20}
	err := r.ApplySettings(map[string]any{"min-words": "many"})
	require.Error(t, err, "expected error for non-int min-words")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{MaxIndex: 14.0, MinWords: 20}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err, "expected error for unknown key")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["max-index"] != 14.0 {
		t.Errorf("expected max-index=14.0, got %v", ds["max-index"])
	}
	if ds["min-words"] != 20 {
		t.Errorf("expected min-words=20, got %v", ds["min-words"])
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS023" {
		t.Errorf("expected MDS023, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "paragraph-readability" {
		t.Errorf(
			"expected paragraph-readability, got %s", r.Name(),
		)
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() != "meta" {
		t.Errorf("expected meta, got %s", r.Category())
	}
}

// --- Placeholder tests ---

func TestCheck_Placeholder_VarToken_MaskedBelowMinWords(t *testing.T) {
	// A paragraph containing only a var-token is masked to a single word
	// ("word"), which is below min-words (20), so no diagnostic is produced.
	src := []byte("# Title\n\n{summary}\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxIndex: 14.0, MinWords: 20, Index: ARI, Placeholders: []string{"var-token"}}
	diags := r.Check(f)
	require.Empty(t, diags, "var-token paragraph masked below min-words should produce no diagnostic")
}

func TestCheck_Placeholder_EmptyList_PlaceholderTextFlagged(t *testing.T) {
	// Without placeholders configured, behavior is unchanged: a paragraph
	// with very few words never reaches the readability threshold anyway.
	// We verify the rule still runs normally when placeholders is empty.
	longPara := "The implementation of concurrent distributed systems requires sophisticated " +
		"understanding of fundamental computational paradigms and synchronization mechanisms " +
		"that must guarantee linearizability across heterogeneous processing environments " +
		"and architectural configurations."
	src := []byte("# Title\n\n" + longPara + "\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MaxIndex: 14.0, MinWords: 20, Index: ARI, Placeholders: []string{}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "long hard-to-read paragraph should still be flagged")
}

func TestApplySettings_Placeholders_ParagraphReadability(t *testing.T) {
	r := &Rule{MaxIndex: 14.0, MinWords: 20, Index: ARI}
	err := r.ApplySettings(map[string]any{
		"placeholders": []any{"var-token"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"var-token"}, r.Placeholders)
}

func TestApplySettings_Placeholders_UnknownToken_ParagraphReadability(t *testing.T) {
	r := &Rule{MaxIndex: 14.0, MinWords: 20, Index: ARI}
	err := r.ApplySettings(map[string]any{"placeholders": []any{"bad"}})
	require.Error(t, err)
}

func TestApplySettings_Placeholders_NonList_ParagraphReadability(t *testing.T) {
	r := &Rule{MaxIndex: 14.0, MinWords: 20, Index: ARI}
	err := r.ApplySettings(map[string]any{"placeholders": "not-a-list"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "list of strings")
}

func TestDefaultSettings_ParagraphReadability_IncludesPlaceholders(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	require.Equal(t, []string{}, ds["placeholders"])
}

func TestSettingMergeMode_ParagraphReadability(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("placeholders"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("max-index"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("unknown"))
}
