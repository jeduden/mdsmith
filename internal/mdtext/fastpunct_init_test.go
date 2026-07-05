//go:build !mdtext_punkt_upstream

package mdtext

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Plan 193 task 10 / test-pyramid: dedicated unit test for the
// default-build initialization. initTokenizer must wire up the
// internal/punkt singleton with the trained English data and the
// three supervised abbreviations upstream applies, otherwise
// downstream SplitSentences calls would silently drift from
// upstream.

func TestInitTokenizer_DefaultBuild(t *testing.T) {
	// Ensure the singleton is constructed even if no other test has
	// run yet. SplitSentences triggers initTokenizer via initOnce;
	// calling it once with a known sample warms the path.
	_ = SplitSentences("warm.")
	require.NotNil(t, forkTokenizer,
		"default-build tokenizer must be non-nil after init")

	t.Run("storage carries supervised abbreviations", func(t *testing.T) {
		// upstream english.NewSentenceTokenizer adds three: sgt, gov, no.
		// initTokenizer must apply the same.
		require.NotNil(t, forkTokenizer.Storage)
		for _, abbr := range []string{"sgt", "gov", "no"} {
			assert.Truef(t, forkTokenizer.Storage.AbbrevTypes.Has(abbr),
				"AbbrevTypes must contain %q after initTokenizer", abbr)
		}
	})

	t.Run("tokenizes a known abbreviation case correctly", func(t *testing.T) {
		// End-to-end smoke check that the assembled pipeline works.
		// Full byte-equivalence is gated by
		// TestSplitSentences_IsItsOwnReference and the golden-rules
		// suite. Here we only need to confirm initTokenizer's output
		// is a functioning tokenizer, not a half-wired one.
		got := SplitSentences("Dr. Smith went home. She did not.")
		require.Len(t, got, 2,
			"Dr. should be classified as an abbreviation, not a "+
				"sentence break")
		assert.Equal(t, "Dr. Smith went home.", got[0])
		assert.Equal(t, "She did not.", got[1])
	})
}

// TestInitTokenizer_SharesAbbrevStorage locks the single-parse
// contract: forkTokenizer must be built from abbrevStorage's cached
// Storage (abbrev.go), not a second punkt.NewEnglish() call, whichever
// of SplitSentences/IsAbbrevToken a caller reaches first. A regression
// here (e.g. reverting to punkt.NewEnglish() in initTokenizer) would
// silently reintroduce the double english.json parse the 2026-07-05
// architecture audit fixed.
func TestInitTokenizer_SharesAbbrevStorage(t *testing.T) {
	_ = SplitSentences("warm.")
	require.Same(t, abbrevStorage(), forkTokenizer.Storage,
		"forkTokenizer.Storage must be the same instance abbrevStorage caches")
}
