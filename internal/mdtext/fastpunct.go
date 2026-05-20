// Package mdtext's "fast Punkt" annotator: a drop-in for
// english.MultiPunctWordAnnotation that swaps the
// `reAbbr.FindAllString(...)` regex for matchAbbrPattern's
// hand-rolled DFA. Plan 191 owns the rationale and the equivalence
// guarantee — every other line in this file mirrors upstream's
// `english/main.go` tokenAnnotation function in v1.1.2, deliberately,
// so the only behavioural difference is the regex → DFA swap.

//go:build !mdtext_punkt_upstream

package mdtext

import (
	"strings"

	"github.com/neurosnap/sentences"
	"github.com/neurosnap/sentences/english"
)

// fastMultiPunctWordAnnotation is the swap-in for
// english.MultiPunctWordAnnotation. Upstream embeds *Storage,
// TokenParser, TokenGrouper, and Ortho, and its `Annotate` method
// loops over token pairs calling `tokenAnnotation`. The only line
// that changes here is the abbreviation match: we call
// matchAbbrPattern(tok) where upstream calls
// `len(reAbbr.FindAllString(tok, 1)) == 0`. Every other branch and
// every other helper (IsListNumber, IsCoordinatePartOne,
// HasUnreliableEndChars, IsInitial, Ortho.Heuristic, FirstUpper,
// SentStarters, TypeNoSentPeriod) is the *exact* upstream helper,
// reached through the embedded interfaces — so no other branch can
// drift from upstream by construction.
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
// english.MultiPunctWordAnnotation.tokenAnnotation, line for line.
// The ONE change is the abbreviation-match branch: upstream calls
//
//	len(reAbbr.FindAllString(tokOne.Tok, 1)) == 0
//
// which is true iff the regex does NOT match. We call
//
//	!matchAbbrPattern(tokOne.Tok)
//
// which is the equivalent boolean form against the same regex
// (proved by abbr_test.go's TestMatchAbbrPattern_* suite).
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

	if a.IsInitial(tokOne) {
		return
	}

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
