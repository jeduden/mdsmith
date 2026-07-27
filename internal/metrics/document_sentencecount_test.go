package metrics

// Pins the "skip work you don't need" fix in
// docs/development/high-performance-go.md: MET006 (conciseness,
// default-enabled), MET009 (sentences), and MET010
// (avg-words-per-sentence) each independently called
// mdtext.CountSentences on the same Document's plain text. Document
// now memoizes the result via SentenceCount, so a run computing more
// than one of these metrics on one file counts sentences once instead
// of up to three times.

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocument_SentenceCount_MemoizesAcrossCalls(t *testing.T) {
	orig := countSentencesFn
	t.Cleanup(func() { countSentencesFn = orig })

	calls := 0
	countSentencesFn = func(text string) int {
		calls++
		return orig(text)
	}

	doc := NewDocument("test.md", []byte("# Hello\n\nOne. Two. Three.\n"))

	n1, err := doc.SentenceCount()
	require.NoError(t, err)
	n2, err := doc.SentenceCount()
	require.NoError(t, err)
	n3, err := doc.SentenceCount()
	require.NoError(t, err)

	assert.Equal(t, n1, n2)
	assert.Equal(t, n1, n3)
	assert.Equal(t, 1, calls,
		"SentenceCount should call mdtext.CountSentences exactly once across repeated calls, not once per call")
}

// TestDocument_SentenceCount_PropagatesPlainTextError verifies that
// when PlainText() fails, SentenceCount propagates the error and
// caches it, matching PlainText's/WordCount's own cached-error tests.
func TestDocument_SentenceCount_PropagatesPlainTextError(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello\n"))
	sentinel := errors.New("injected plain-text error")
	doc.plainTextReady = true
	doc.plainTextErr = sentinel

	n, err := doc.SentenceCount()
	assert.Equal(t, 0, n)
	assert.ErrorIs(t, err, sentinel)

	// Cached error path: a second call must not re-derive plain text.
	n2, err2 := doc.SentenceCount()
	assert.Equal(t, 0, n2)
	assert.ErrorIs(t, err2, sentinel)
}

func TestMetrics_MET006AndMET009_ShareSentenceCount(t *testing.T) {
	orig := countSentencesFn
	t.Cleanup(func() { countSentencesFn = orig })

	calls := 0
	countSentencesFn = func(text string) int {
		calls++
		return orig(text)
	}

	doc := NewDocument("test.md", []byte("# Hello\n\nOne. Two. Three. Four.\n"))

	conciseness, ok := Lookup("MET006")
	require.True(t, ok)
	_, err := conciseness.Compute(doc)
	require.NoError(t, err)

	sentences, ok := Lookup("MET009")
	require.True(t, ok)
	_, err = sentences.Compute(doc)
	require.NoError(t, err)

	avgWords, ok := Lookup("MET010")
	require.True(t, ok)
	_, err = avgWords.Compute(doc)
	require.NoError(t, err)

	assert.Equal(t, 1, calls,
		"MET006+MET009+MET010 on one Document should count sentences once, not once per metric")
}
