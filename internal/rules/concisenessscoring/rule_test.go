package concisenessscoring

import (
	"errors"
	"sync"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/concisenessscoring/classifier"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func modelConciseness(t *testing.T) float64 {
	t.Helper()
	m, err := classifier.LoadEmbedded()
	require.NoError(t, err)
	return 1.0 - m.Threshold()
}

func verboseParagraph() string {
	return "Basically, it seems that we are just trying to explain the " +
		"same idea in order to make it very clear, and it appears that " +
		"we are really saying very little new information overall."
}

func conciseParagraph() string {
	return "The release process validates changelog links, updates " +
		"version tags, and publishes checksums so reviewers can verify " +
		"artifacts before promoting a build."
}

func TestCheck_LowScore(t *testing.T) {
	src := []byte(verboseParagraph() + "\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	threshold := modelConciseness(t)
	r := &Rule{
		MinScore: threshold,
		MinWords: 20,
	}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))

	d := diags[0]
	assert.Equal(t, "MDS029", d.RuleID)
	assert.Equal(t, "conciseness-scoring", d.RuleName)
	assert.Equal(t, lint.Warning, d.Severity)
	assert.Contains(t, d.Message, "conciseness score too low")
	assert.Contains(t, d.Message, "target >=")
}

func TestCheck_HighScore(t *testing.T) {
	src := []byte(conciseParagraph() + "\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{
		MinScore: 0.10,
		MinWords: 20,
	}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_ShortParagraphSkipped(t *testing.T) {
	src := []byte("This is short and intentionally skipped.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{
		MinScore: 0.95,
		MinWords: 20,
	}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_DiagnosticLine(t *testing.T) {
	src := []byte("# Heading\n\n" + verboseParagraph() + "\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	threshold := modelConciseness(t)
	r := &Rule{
		MinScore: threshold,
		MinWords: 20,
	}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_TableSkipped(t *testing.T) {
	src := []byte(
		"| Setting | Value |\n" +
			"|---------|-------|\n" +
			"| alpha   | beta  |\n",
	)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{
		MinScore: 0.95,
		MinWords: 1,
	}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for table, got %d", len(diags))
}

func TestApplySettings_Valid(t *testing.T) {
	r := &Rule{MinScore: defaultMinScore, MinWords: defaultMinWords}
	err := r.ApplySettings(map[string]any{
		"min-score": 0.5,
		"min-words": 30,
	})
	require.NoError(t, err, "unexpected error: %v", err)
	if r.MinScore != 0.5 {
		t.Errorf("expected MinScore=0.5, got %.2f", r.MinScore)
	}
	if r.MinWords != 30 {
		t.Errorf("expected MinWords=30, got %d", r.MinWords)
	}
}

func TestApplySettings_RemovedListSettings(t *testing.T) {
	r := &Rule{MinScore: defaultMinScore, MinWords: defaultMinWords}
	err := r.ApplySettings(map[string]any{"filler-words": []any{"test"}})
	require.Error(t, err, "filler-words should be unknown after removal")
}

func TestApplySettings_InvalidMinScoreType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-score": "high"})
	require.Error(t, err, "expected error for non-number min-score")
}

func TestApplySettings_InvalidMinScoreRange(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-score": 1.2})
	require.Error(t, err, "expected error for out-of-range min-score")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err, "expected error for unknown setting")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["min-score"] != defaultMinScore {
		t.Errorf(
			"expected min-score=%.2f, got %v",
			defaultMinScore, ds["min-score"],
		)
	}
	if ds["min-words"] != defaultMinWords {
		t.Errorf(
			"expected min-words=%d, got %v",
			defaultMinWords, ds["min-words"],
		)
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS029" {
		t.Errorf("expected MDS029, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "conciseness-scoring" {
		t.Errorf("expected conciseness-scoring, got %s", r.Name())
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() != "meta" {
		t.Errorf("expected meta, got %s", r.Category())
	}
}

func TestEnabledByDefault(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault(), "conciseness-scoring should be disabled by default")
}

// --- formatExamples branch coverage ---

// TestFormatExamples_Empty exercises the len==0 branch of formatExamples.
func TestFormatExamples_Empty(t *testing.T) {
	result := formatExamples([]string{})
	assert.Equal(t, "", result)
}

// TestFormatExamples_SingleExample exercises the `len < limit` branch of
// formatExamples, which caps at min(2, len).
func TestFormatExamples_SingleExample(t *testing.T) {
	result := formatExamples([]string{"basically"})
	assert.Contains(t, result, "basically")
	// Only one example, so no comma separator.
	assert.NotContains(t, result, ", ")
}

// --- setMinWords error branches ---

// TestApplySettings_InvalidMinWordsType exercises the non-int path in setMinWords.
func TestApplySettings_InvalidMinWordsType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-words": "twenty"})
	require.Error(t, err, "expected error for non-integer min-words")
	assert.Contains(t, err.Error(), "min-words must be an integer")
}

// TestApplySettings_MinWordsZero exercises the n<=0 path in setMinWords.
func TestApplySettings_MinWordsZero(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-words": 0})
	require.Error(t, err, "expected error for min-words=0")
	assert.Contains(t, err.Error(), "min-words must be > 0")
}

// TestApplySettings_MinWordsNegative exercises the n<=0 path in setMinWords
// with a negative value.
func TestApplySettings_MinWordsNegative(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-words": -5})
	require.Error(t, err, "expected error for negative min-words")
	assert.Contains(t, err.Error(), "min-words must be > 0")
}

// TestApplySettings_MinWordsValid exercises the success path when setMinWords
// is called via ApplySettings so that the err!=nil return is also covered.
func TestApplySettings_MinWordsValid(t *testing.T) {
	r := &Rule{MinScore: defaultMinScore, MinWords: defaultMinWords}
	err := r.ApplySettings(map[string]any{"min-words": 10})
	require.NoError(t, err)
	assert.Equal(t, 10, r.MinWords)
}

// =====================================================================
// Phase 5: additional branch coverage
// =====================================================================

// TestCheck_LoadError exercises the loadErrorDiag path by injecting a
// scorer load error via package-level state reset.
func TestCheck_LoadError(t *testing.T) {
	// Save non-Once state and restore after the test.
	origScorer := globalScorer
	origErr := scorerErr
	t.Cleanup(func() {
		// Reset Once objects (cannot copy sync.Once) and restore scorer state.
		scorerOnce = sync.Once{}
		errReportedOnce = sync.Once{}
		globalScorer = origScorer
		scorerErr = origErr
		// Mark scorerOnce done with the restored values already set.
		scorerOnce.Do(func() {})
	})

	// Inject a fake error so loadScorer() returns it.
	scorerOnce = sync.Once{}
	errReportedOnce = sync.Once{}
	globalScorer = nil
	scorerErr = errors.New("injected classifier load failure")
	// Pre-consume the once so loadScorer returns the error immediately.
	scorerOnce.Do(func() {}) // no-op; scorerErr is already set

	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{MinScore: defaultMinScore, MinWords: defaultMinWords}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 error diagnostic")
	assert.Equal(t, lint.Error, diags[0].Severity)
	assert.Contains(t, diags[0].Message, "classifier load failed")

	// Second call: errReportedOnce suppresses the error.
	diags2 := r.Check(f)
	assert.Empty(t, diags2, "second call should not repeat the error diagnostic")
}

// TestNewScorer_Success verifies NewScorer succeeds with the embedded artifact.
func TestNewScorer_Success(t *testing.T) {
	s, err := NewScorer()
	require.NoError(t, err)
	assert.NotNil(t, s)
}
