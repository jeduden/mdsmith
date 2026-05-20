//go:build !mdtext_punkt_upstream

package mdtext

import (
	"testing"

	sentlib "github.com/neurosnap/sentences"
	"github.com/neurosnap/sentences/english"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Plan 191 / test-pyramid: dedicated unit tests for the private
// fastpunct.go symbols. fastMultiPunctWordAnnotation drives the
// abbreviation classifier inside trained Punkt; correctness across
// each branch of tokenAnnotation is what keeps the fast-path
// SplitSentences byte-identical to upstream.
//
// These tests construct the annotator directly with a synthetic
// Storage so each branch is hit deterministically without depending
// on the bundled training data. The black-box equivalence harness
// in sentence_equivalence_test.go remains the integration gate.

// orthoBegLcBit is the same value as the unexported orthoBegLc in
// upstream/ortho.go: a flag in Storage.OrthoContext that says the
// type has been seen sentence-initial with a lowercase first letter.
// Combined with FirstUpper(tokTwo) and the absence of orthoMidUc,
// it makes Ortho.Heuristic return 1.
const orthoBegLcBit = 1 << 4

// newTestFastAnnotator constructs a fastMultiPunctWordAnnotation
// against a fresh empty Storage. The caller seeds OrthoContext,
// SentStarters, etc. to drive specific branches. Independent of
// the bundled English training data, so the tests are fast and
// hermetic.
func newTestFastAnnotator(t *testing.T) *fastMultiPunctWordAnnotation {
	t.Helper()
	storage := sentlib.NewStorage()
	lang := sentlib.NewPunctStrings()
	word := english.NewWordTokenizer(lang)
	ortho := &sentlib.OrthoContext{
		Storage:      storage,
		PunctStrings: lang,
		TokenType:    word,
		TokenFirst:   word,
	}
	return &fastMultiPunctWordAnnotation{
		Storage:      storage,
		TokenParser:  word,
		TokenGrouper: &sentlib.DefaultTokenGrouper{},
		Ortho:        ortho,
		upstreamWord: word,
	}
}

// TestFastMultiPunctWordAnnotation_Annotate pins the outer loop:
// it groups tokens into adjacent pairs and skips any pair whose
// second slot is nil (the trailing pair the grouper appends to
// mark end-of-stream). Without this skip, tokenAnnotation would
// dereference a nil tokTwo on the last pair.
func TestFastMultiPunctWordAnnotation_Annotate(t *testing.T) {
	t.Run("empty input returns empty slice", func(t *testing.T) {
		a := newTestFastAnnotator(t)
		got := a.Annotate(nil)
		assert.Nil(t, got)
	})

	t.Run("single token skipped via nil-second guard", func(t *testing.T) {
		a := newTestFastAnnotator(t)
		tok := sentlib.NewToken("hello.")
		tok.SentBreak = true // unchanged after Annotate
		got := a.Annotate([]*sentlib.Token{tok})
		require.Len(t, got, 1)
		assert.True(t, got[0].SentBreak,
			"single-token pass should not mutate the token; "+
				"the trailing (tok, nil) pair must be skipped")
	})

	t.Run("multi-token pairs feed tokenAnnotation", func(t *testing.T) {
		a := newTestFastAnnotator(t)
		// "U.S." is an abbr-pattern match; "and" has lowercase
		// orthotype, so Heuristic returns -1 and FirstUpper is
		// false → tokenAnnotation sets Abbr=true, SentBreak=false.
		// The trailing (and, nil) pair is skipped.
		t1 := sentlib.NewToken("U.S.")
		t2 := sentlib.NewToken("and")
		got := a.Annotate([]*sentlib.Token{t1, t2})
		require.Len(t, got, 2)
		assert.True(t, got[0].Abbr,
			"first token reached the abbr-classifier body")
		assert.False(t, got[0].SentBreak,
			"first token's SentBreak should be cleared by the body")
		assert.False(t, got[1].Abbr,
			"second token only appears as tokTwo; never mutated by "+
				"this annotator")
	})
}

// tokenAnnotationCase drives one subtest of
// TestFastMultiPunctWordAnnotation_tokenAnnotation. `setup` may
// seed Storage maps (OrthoContext, SentStarters) before the call.
// `wantAbbr` and `wantSentBreak` are the expected post-call flags
// on tokOne. `note` documents which branch the case exercises and
// what its assertion proves; failures include it as test context.
type tokenAnnotationCase struct {
	name          string
	setup         func(*fastMultiPunctWordAnnotation)
	tokOne        string
	tokTwo        string
	seedSentBreak bool
	wantAbbr      bool
	wantSentBreak bool
	note          string
}

// tokenAnnotationCases enumerates every reachable branch of
// fastMultiPunctWordAnnotation.tokenAnnotation. Held at package
// scope so the parent test stays under funlen.
var tokenAnnotationCases = []tokenAnnotationCase{
	{
		name:          "list_number_clears_sentbreak",
		tokOne:        "1.",
		tokTwo:        "The",
		seedSentBreak: true,
		wantAbbr:      false,
		wantSentBreak: false,
		note:          "IsListNumber must demote the sentence break",
	},
	{
		name:          "coordinate_part_one_clears_sentbreak",
		tokOne:        "N°.",
		tokTwo:        "1026.253.553.",
		seedSentBreak: true,
		wantSentBreak: false,
		note:          "IsCoordinatePartOne must demote the sentence break",
	},
	{
		name:          "period_followed_by_period_clears_sentbreak",
		tokOne:        "abc.",
		tokTwo:        ".",
		seedSentBreak: true,
		wantSentBreak: false,
		note: "period-terminated tok + lone period is a spaced " +
			"ellipsis fragment, never a sentence break",
	},
	{
		name:   "no_abbr_indicators_returns_without_mutation",
		tokOne: "hello.",
		tokTwo: "World.",
		note: "hello. matches no abbr indicator; the early return " +
			"must skip the body and leave both flags false",
	},
	{
		name: "sent_starter_heuristic_sets_sentbreak",
		setup: func(a *fastMultiPunctWordAnnotation) {
			// Heuristic returns 1 when tokTwo is capitalized AND its
			// orthotype has any lowercase-position bit set AND the
			// orthoMidUc bit is clear. Seeding OrthoContext["next"]
			// with orthoBegLc satisfies all three.
			a.OrthoContext["next"] = orthoBegLcBit
		},
		tokOne:        "U.S.",
		tokTwo:        "Next",
		wantAbbr:      true,
		wantSentBreak: true,
		note: "Ortho.Heuristic==1 must elevate this token back to " +
			"a sentence break",
	},
	{
		name: "first_upper_with_sent_starter_sets_sentbreak",
		setup: func(a *fastMultiPunctWordAnnotation) {
			// No orthotype set → Heuristic returns -1. FirstUpper
			// true + SentStarters[next]≠0 triggers the final gate.
			a.SentStarters["world"] = 1
		},
		tokOne:        "U.S.",
		tokTwo:        "World",
		wantAbbr:      true,
		wantSentBreak: true,
		note: "FirstUpper(tokTwo) + SentStarters[type] must restore " +
			"the sentence break",
	},
	{
		name:          "first_upper_with_unreliable_end_chars_sets_sentbreak",
		tokOne:        `It."`,
		tokTwo:        "World",
		wantAbbr:      true,
		wantSentBreak: true,
		note: `HasUnreliableEndChars (suffix ."): the gate lets ` +
			"the token through and the final gate's second disjunct " +
			"restores the sentence break",
	},
	{
		name:          "lone_period_with_first_upper_sets_sentbreak",
		tokOne:        ".",
		tokTwo:        "World",
		wantAbbr:      true,
		wantSentBreak: true,
		note: `tokOne.Tok=="." + FirstUpper(tokTwo) must restore ` +
			"the sentence break (third disjunct of the final gate)",
	},
	{
		name:          "coordinate_part_two_with_first_upper_sets_sentbreak",
		tokOne:        "1.2.3.",
		tokTwo:        "World",
		wantAbbr:      true,
		wantSentBreak: true,
		note: "IsCoordinatePartTwo + FirstUpper(tokTwo) must " +
			"restore the sentence break (fourth disjunct)",
	},
	{
		name:          "lowercase_tok_two_falls_through_without_sentbreak",
		tokOne:        "U.S.",
		tokTwo:        "then",
		seedSentBreak: true,
		wantAbbr:      true,
		wantSentBreak: false,
		note: "no heuristic or starter condition fires; the body's " +
			"SentBreak=false assignment is the final state",
	},
}

// TestFastMultiPunctWordAnnotation_tokenAnnotation walks every
// reachable branch of tokenAnnotation, with each subtest pinning
// the branch noted in its name. Held in a table so a new branch
// (or an upstream-mirror branch reactivation) adds one entry, not
// a new function.
func TestFastMultiPunctWordAnnotation_tokenAnnotation(t *testing.T) {
	for _, tc := range tokenAnnotationCases {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestFastAnnotator(t)
			if tc.setup != nil {
				tc.setup(a)
			}
			tokOne := sentlib.NewToken(tc.tokOne)
			tokOne.SentBreak = tc.seedSentBreak
			tokTwo := sentlib.NewToken(tc.tokTwo)
			a.tokenAnnotation(tokOne, tokTwo)
			assert.Equalf(t, tc.wantAbbr, tokOne.Abbr,
				"Abbr flag — %s", tc.note)
			assert.Equalf(t, tc.wantSentBreak, tokOne.SentBreak,
				"SentBreak flag — %s", tc.note)
		})
	}
}
