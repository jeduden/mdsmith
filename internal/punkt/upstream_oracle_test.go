package punkt

import (
	sentlib "github.com/neurosnap/sentences"
)

// upstreamWordTokenizer is the upstream DefaultWordTokenizer the
// equivalence tests in this package use as the oracle.
var upstreamWordTokenizer = sentlib.NewWordTokenizer(sentlib.NewPunctStrings())

// upstreamTokens runs the upstream DefaultWordTokenizer against
// text and returns a (Tok, Position, ParaStart, LineStart) view
// for byte-level comparison against the fork's Tokenize. Used by
// TestTokenize_MatchesUpstreamShape and any other unit-level pin
// against upstream's word-token output.
func upstreamTokens(text string) []Token {
	src := upstreamWordTokenizer.Tokenize(text, false)
	out := make([]Token, 0, len(src))
	for _, s := range src {
		out = append(out, Token{
			Tok:       s.Tok,
			Position:  s.Position,
			ParaStart: s.ParaStart,
			LineStart: s.LineStart,
		})
	}
	return out
}
