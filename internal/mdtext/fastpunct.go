//go:build !mdtext_punkt_upstream

// fastpunct.go provides a drop-in for
// english.MultiPunctWordAnnotation. Two deltas from upstream:
// (1) the `reAbbr.FindAllString(...)` regex is swapped for
// matchAbbrPattern's hand-rolled DFA, and (2) upstream's
// unreachable `IsInitial` guard is elided. Both are documented
// on tokenAnnotation below. Plan 191 owns the rationale and the
// byte-equivalence guarantee — every other line in this file
// mirrors upstream's `english/main.go` v1.1.2 deliberately.

package mdtext

import (
	"strings"

	"github.com/neurosnap/sentences"
	"github.com/neurosnap/sentences/english"
)

// fastMultiPunctWordAnnotation is the swap-in for
// english.MultiPunctWordAnnotation. Upstream embeds *Storage,
// TokenParser, TokenGrouper, and Ortho, and its `Annotate` method
// loops over token pairs calling `tokenAnnotation`. tokenAnnotation
// here carries two deltas from upstream — the regex-vs-DFA swap on
// the abbreviation match, and the elision of upstream's unreachable
// IsInitial guard. Both are documented on tokenAnnotation itself.
// Every other branch and every other helper (IsListNumber,
// IsCoordinatePartOne, HasUnreliableEndChars, Ortho.Heuristic,
// FirstUpper, SentStarters, TypeNoSentPeriod) is the exact upstream
// helper, reached through the embedded interfaces — so no other
// branch can drift from upstream by construction.
//
// upstreamWord retains the English WordTokenizer for one purpose
// only: HasUnreliableEndChars. The upstream `english.WordTokenizer`
// overrides that method (and HasSentEndChars). Reusing
// `english.NewWordTokenizer` keeps the override active without
// reimplementing it.
type fastMultiPunctWordAnnotation struct {
	*sentences.Storage
	sentences.TokenParser
	sentences.TokenGrouper
	sentences.Ortho
	upstreamWord *english.WordTokenizer
}

// Annotate mirrors english.MultiPunctWordAnnotation.Annotate.
func (a *fastMultiPunctWordAnnotation) Annotate(tokens []*sentences.Token) []*sentences.Token {
	for _, tokPair := range a.Group(tokens) {
		if len(tokPair) < 2 || tokPair[1] == nil {
			continue
		}
		a.tokenAnnotation(tokPair[0], tokPair[1])
	}
	return tokens
}

// tokenAnnotation mirrors
// english.MultiPunctWordAnnotation.tokenAnnotation with two
// deltas from upstream:
//
//  1. The abbreviation-match branch swaps
//     `len(reAbbr.FindAllString(tokOne.Tok, 1)) == 0` for
//     `!matchAbbrPattern(tokOne.Tok)` — the equivalent boolean
//     form, proved by abbr_test.go's TestMatchAbbrPattern_* suite.
//  2. Upstream's `if a.IsInitial(tokOne) { return }` guard after
//     the abbr-match branch is elided. It is unreachable in the
//     current upstream (see the long comment above the body
//     below), and the project's defensive-code rule forbids
//     untestable branches. The equivalence harness catches any
//     drift if upstream ever weakens the preceding gate.
//
// Every other branch and every other helper (IsListNumber,
// IsCoordinatePartOne, HasUnreliableEndChars, Ortho.Heuristic,
// FirstUpper, SentStarters, TypeNoSentPeriod) is the exact
// upstream helper reached through the embedded interfaces.
func (a *fastMultiPunctWordAnnotation) tokenAnnotation(tokOne, tokTwo *sentences.Token) {
	if a.IsListNumber(tokOne) || a.IsCoordinatePartOne(tokOne) {
		tokOne.SentBreak = false
		return
	}

	if strings.HasSuffix(tokOne.Tok, ".") && tokTwo.Tok == "." {
		tokOne.SentBreak = false
		return
	}

	if !matchAbbrPattern(tokOne.Tok) &&
		tokOne.Tok != "." &&
		!a.upstreamWord.HasUnreliableEndChars(tokOne) &&
		!a.IsCoordinatePartTwo(tokOne) {
		return
	}

	// Upstream `english.MultiPunctWordAnnotation.tokenAnnotation`
	// has an `if a.IsInitial(tokOne) { return }` guard here.
	// `reInitial` matches `^[A-Za-z]\.$` only — single letter +
	// period — and every such token fails the preceding gate
	// (matchAbbrPattern is false, Tok != ".", HasUnreliableEndChars
	// is false, IsCoordinatePartTwo is false). The guard is dead in
	// current upstream, so we elide it: there is no input it would
	// catch that the preceding gate has not already returned on. If
	// upstream ever weakens that gate, the equivalence harness in
	// sentence_equivalence_test.go fails on the next run.

	tokOne.Abbr = true
	tokOne.SentBreak = false

	nextTyp := a.TypeNoSentPeriod(tokTwo)
	isSentStarter := a.Heuristic(tokTwo)
	if isSentStarter == 1 {
		tokOne.SentBreak = true
		return
	}

	if a.FirstUpper(tokTwo) &&
		(a.SentStarters[nextTyp] != 0 ||
			a.upstreamWord.HasUnreliableEndChars(tokOne) ||
			tokOne.Tok == "." ||
			a.IsCoordinatePartTwo(tokOne)) {
		tokOne.SentBreak = true
		return
	}
}
