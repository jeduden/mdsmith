//go:build !mdtext_punkt_upstream

package mdtext

import (
	"testing"

	"github.com/neurosnap/sentences/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMustLoadTraining drives the three branches of mustLoadTraining
// red/green so the helper's defensive panics satisfy CLAUDE.md's
// "drive it red/green" rule.
func TestMustLoadTraining(t *testing.T) {
	t.Run("happy path returns non-nil storage", func(t *testing.T) {
		storage := mustLoadTraining(data.MustAsset("data/english.json"))
		require.NotNil(t, storage)
		assert.NotNil(t, storage.AbbrevTypes)
	})

	t.Run("malformed JSON panics with descriptive message", func(t *testing.T) {
		var got any
		func() {
			defer func() { got = recover() }()
			mustLoadTraining([]byte("not json"))
		}()
		require.NotNil(t, got,
			"mustLoadTraining must panic on malformed bytes, not "+
				"silently return a half-built tokenizer")
		msg, ok := got.(string)
		require.Truef(t, ok, "panic value should be a string, got %T", got)
		assert.Contains(t, msg,
			"mdtext: failed to load Punkt training data:",
			"panic message should name the loader so the cause is "+
				"obvious without a stack walk")
	})

	t.Run("empty input panics", func(t *testing.T) {
		assert.Panics(t,
			func() { mustLoadTraining(nil) },
			"nil/empty input must not silently return a half-built "+
				"tokenizer")
	})
}

// Plan 191 / test-pyramid: dedicated unit test for buildTokenizer.
// The test pins the success-path contract — every component the
// downstream code relies on must be present in the returned
// tokenizer, including the fast-path MultiPunctWordAnnotation that
// replaces upstream's regex.

func TestBuildTokenizer(t *testing.T) {
	tok := buildTokenizer()

	t.Run("returns non-nil tokenizer", func(t *testing.T) {
		require.NotNil(t, tok)
	})

	t.Run("storage carries supervised abbreviations", func(t *testing.T) {
		require.NotNil(t, tok.Storage)
		// The three supervised abbreviations english.NewSentenceTokenizer
		// adds to the trained data; buildTokenizer must add the same.
		for _, abbr := range []string{"sgt", "gov", "no"} {
			assert.Truef(t, tok.AbbrevTypes.Has(abbr),
				"AbbrevTypes must contain %q after buildTokenizer", abbr)
		}
	})

	t.Run("has three annotators with fastMulti third", func(t *testing.T) {
		require.Len(t, tok.Annotations, 3,
			"expect TypeBasedAnnotation, TokenBasedAnnotation, "+
				"and the fast-path multi-punct annotator")
		_, ok := tok.Annotations[2].(*fastMultiPunctWordAnnotation)
		assert.Truef(t, ok,
			"the third annotator must be *fastMultiPunctWordAnnotation; "+
				"got %T", tok.Annotations[2])
	})

	t.Run("fast multi annotator carries the same storage", func(t *testing.T) {
		fast, ok := tok.Annotations[2].(*fastMultiPunctWordAnnotation)
		require.True(t, ok)
		assert.Same(t, tok.Storage, fast.Storage,
			"the fast annotator must share the tokenizer's Storage, "+
				"otherwise AbbrevTypes additions would not propagate")
	})

	t.Run("tokenizes a known abbreviation case correctly", func(t *testing.T) {
		// A spot check that the assembled pipeline actually works
		// end to end. The full equivalence corpus is gated by
		// TestSplitSentences_IsItsOwnReference; here we only need
		// to confirm buildTokenizer's output is a functioning
		// tokenizer, not a half-wired one.
		sents := tok.Tokenize("Dr. Smith went home. She did not.")
		require.Len(t, sents, 2,
			"Dr. should be classified as an abbreviation, not a "+
				"sentence break")
		assert.Equal(t, "Dr. Smith went home.", sents[0].Text)
	})
}
