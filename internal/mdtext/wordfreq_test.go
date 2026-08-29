package mdtext_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/stretchr/testify/assert"
)

func TestWordFrequency_Empty(t *testing.T) {
	freq := mdtext.WordFrequency("", 4)
	assert.Empty(t, freq)
}

func TestWordFrequency_MinLength_FiltersShort(t *testing.T) {
	freq := mdtext.WordFrequency("a bb ccc dddd", 4)
	assert.Equal(t, map[string]int{"dddd": 1}, freq)
}

func TestWordFrequency_MinLengthZero_IncludesAll(t *testing.T) {
	freq := mdtext.WordFrequency("a bb", 0)
	assert.Equal(t, map[string]int{"a": 1, "bb": 1}, freq)
}

func TestWordFrequency_CaseFolded(t *testing.T) {
	freq := mdtext.WordFrequency("Word word WORD", 4)
	assert.Equal(t, map[string]int{"word": 3}, freq)
}

func TestWordFrequency_PunctuationBoundary(t *testing.T) {
	freq := mdtext.WordFrequency("hello, world! hello.", 4)
	assert.Equal(t, map[string]int{"hello": 2, "world": 1}, freq)
}

func TestWordFrequency_HyphenBoundary(t *testing.T) {
	// hyphens split words
	freq := mdtext.WordFrequency("well-known approach", 4)
	assert.Equal(t, map[string]int{"well": 1, "known": 1, "approach": 1}, freq)
}

func TestWordFrequency_Digits(t *testing.T) {
	freq := mdtext.WordFrequency("plan1 plan1 plan2", 5)
	assert.Equal(t, map[string]int{"plan1": 2, "plan2": 1}, freq)
}

func TestWordFrequency_RepetitionCount(t *testing.T) {
	text := "content content content data data"
	freq := mdtext.WordFrequency(text, 4)
	assert.Equal(t, 3, freq["content"])
	assert.Equal(t, 2, freq["data"])
}

func TestMaxWordFrequency_Empty(t *testing.T) {
	assert.Equal(t, 0, mdtext.MaxWordFrequency(nil))
	assert.Equal(t, 0, mdtext.MaxWordFrequency(map[string]int{}))
}

func TestMaxWordFrequency_Single(t *testing.T) {
	assert.Equal(t, 5, mdtext.MaxWordFrequency(map[string]int{"word": 5}))
}

func TestMaxWordFrequency_Multiple(t *testing.T) {
	freq := map[string]int{"alpha": 3, "beta": 7, "gamma": 1}
	assert.Equal(t, 7, mdtext.MaxWordFrequency(freq))
}

// TestWordFrequency_SharedTokenizer verifies that the metric and the
// rule share identical tokenization: the max count returned by
// WordFrequency+MaxWordFrequency matches what a per-word check finds.
func TestWordFrequency_SharedTokenizer(t *testing.T) {
	text := "process process process result result"
	freq := mdtext.WordFrequency(text, 4)
	max := mdtext.MaxWordFrequency(freq)
	assert.Equal(t, 3, max)
	assert.Equal(t, 3, freq["process"])
	assert.Equal(t, 2, freq["result"])
}

func TestWordFrequency_LastWordAtMinLength(t *testing.T) {
	// A word whose rune count exactly equals minLength at end-of-string must
	// be flushed by the sentinel iteration (i == len(text)). Verify it is
	// emitted and not dropped by the rune-counter guard.
	freq := mdtext.WordFrequency("hello", 5)
	assert.Equal(t, map[string]int{"hello": 1}, freq)
}

func TestWordFrequency_LastWordBelowMinLength(t *testing.T) {
	// A word shorter than minLength at end-of-string must not be emitted.
	freq := mdtext.WordFrequency("hi", 5)
	assert.Empty(t, freq)
}
