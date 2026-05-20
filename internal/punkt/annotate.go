package punkt

import (
	"bytes"
	"strings"
)

// state is the per-Tokenize-call working memory pooled by
// DefaultSentenceTokenizer via sync.Pool. Putting every reusable
// buffer here means a single sync.Pool.Get / Put pair amortizes the
// per-call slice growth across all hot paths in the pipeline.
type state struct {
	// tokens is the backing storage for Tokenize output. The slice
	// values (not pointers) are reused across calls; ptrs[] below
	// holds the pointers the AnnotateTokens interface needs.
	tokens []Token
	// ptrs aliases tokens via &tokens[i]. It's the []*Token shape the
	// AnnotateTokens interface expects.
	ptrs []*Token
	// pairs is the [][2]*Token grouping buffer used by every
	// Annotate pass. Upstream's DefaultTokenGrouper allocates a fresh
	// one per pass; this fork reuses across passes.
	pairs [][2]*Token
	// typeBufA and typeBufB are two byte buffers for typeOf-family
	// calls. tokenAnnotation needs `typ` and `nextTyp` live at the
	// same time (the collocation key is the pair), so a single
	// buffer would force a per-call allocation for one of them. The
	// pair amortizes both via state reuse.
	typeBufA []byte
	typeBufB []byte
	// colKeyBuf is the byte buffer used to assemble the comma-joined
	// collocation key in tokenAnnotation. Sized at first use, reused
	// thereafter.
	colKeyBuf []byte
	// sents is the result Sentence slice, returned to the caller.
	sents []Sentence
}

// reset prepares the state for return to the sync.Pool. Slices are
// truncated to length 0 but their backing arrays may still hold
// references into the previous input (Token.Tok and Sentence.Text
// are substring slices of the input passed to Tokenize). Zeroing
// the used range of each backing array drops those references, so
// the previous input can be GC'd as soon as the caller is done with
// it — otherwise sync.Pool retention would keep large input
// buffers alive across Tokenize calls. The ptrs slice carries only
// *Token pointers into s.tokens, so clearing tokens covers it.
func (s *state) reset() {
	for i := range s.tokens {
		s.tokens[i] = Token{}
	}
	s.tokens = s.tokens[:0]
	clear(s.ptrs)
	s.ptrs = s.ptrs[:0]
	clear(s.pairs)
	s.pairs = s.pairs[:0]
	s.typeBufA = s.typeBufA[:0]
	s.typeBufB = s.typeBufB[:0]
	s.colKeyBuf = s.colKeyBuf[:0]
	for i := range s.sents {
		s.sents[i] = Sentence{}
	}
	s.sents = s.sents[:0]
}

// group writes adjacent token pairs into st.pairs and returns it.
// Mirrors upstream DefaultTokenGrouper.Group: emits [prev, tok] for
// every consecutive pair (skipping the trivial prev==tok at the head)
// and a final [last, nil] sentinel. Reuses st.pairs so a Tokenize
// call with multiple Annotate passes pays one allocation total for
// the pair slice (when its capacity grows).
func group(st *state, tokens []*Token) [][2]*Token {
	st.pairs = st.pairs[:0]
	if len(tokens) == 0 {
		return st.pairs
	}
	prev := tokens[0]
	for _, tok := range tokens {
		if prev == tok {
			continue
		}
		st.pairs = append(st.pairs, [2]*Token{prev, tok})
		prev = tok
	}
	st.pairs = append(st.pairs, [2]*Token{prev, nil})
	return st.pairs
}

// typeAnnotation is the first-pass annotator (upstream
// TypeBasedAnnotation.typeAnnotation). It marks `.`/`?`/`!`
// suffixes as sentence breaks and period-ending tokens as either
// abbreviations or sentence breaks per the trained AbbrevTypes set.
//
// Upstream uses strings.Split for the hyphenation suffix check:
//
//	tokNoPeriodHypen := strings.Split(tokNoPeriod, "-")
//	tokLastHyphEl := string(tokNoPeriodHypen[len(tokNoPeriodHypen)-1])
//
// The only consumer is `tokLastHyphEl`, the tail after the last `-`.
// This fork uses strings.LastIndexByte to find the same suffix
// without allocating the intermediate `[]string` (and without the
// `[]byte(tok)` copy bytes.LastIndexByte would force).
func typeAnnotation(s *Storage, token *Token) {
	if hasSentEndChars(token.Tok) {
		token.SentBreak = true
		return
	}
	if !HasPeriodFinal(token.Tok) || hasSuffix(token.Tok, "..") {
		return
	}

	// `tokNoPeriod := strings.ToLower(token.Tok[:len(chars)-1])`
	// upstream first computes []rune(token.Tok) and slices to
	// len(chars)-1 — i.e. drops the final period (which is one byte).
	// The byte form is `token.Tok[:len(token.Tok)-1]` then
	// strings.ToLower.
	tok := token.Tok[:len(token.Tok)-1]

	// Find the last "-" segment of tok. If present, the tail after
	// the last `-` is what the abbrev check sees in addition to the
	// whole. strings.LastIndexByte avoids the per-call
	// `[]byte(tok)` conversion bytes.LastIndexByte would force.
	tail := tok
	if i := strings.LastIndexByte(tok, '-'); i >= 0 {
		tail = tok[i+1:]
	}

	// Upstream lowercases tok with strings.ToLower; here we do the
	// abbrev check via a lowercase-aware lookup. AbbrevTypes is
	// keyed by lowercase tokens, so we lowercase into a tiny stack
	// buffer if the token has any uppercase ASCII letter.
	if hasAbbrLowered(s, tok) || hasAbbrLowered(s, tail) {
		token.Abbr = true
	} else {
		token.SentBreak = true
	}
}

// hasAbbrLowered reports whether the lowercase form of s appears in
// AbbrevTypes. ASCII-only fast path: tokens flowing through
// typeAnnotation are word fragments without their trailing period
// and rarely contain non-ASCII letters; even when they do, upstream's
// strings.ToLower handles them, so we forward the rare case to a
// helper.
func hasAbbrLowered(s *Storage, key string) bool {
	if isAllASCIILower(key) {
		return s.AbbrevTypes.Has(key)
	}
	// Lower into a small stack buffer (or grow if needed).
	var stack [32]byte
	buf := stack[:0]
	buf = appendStringsToLower(buf, key)
	return s.AbbrevTypes.Has(string(buf))
}

func isAllASCIILower(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			return false
		}
		if c >= 0x80 {
			return false
		}
	}
	return true
}

// tokenAnnotation is the second-pass annotator (upstream
// TokenBasedAnnotation.tokenAnnotation). It runs section 4.1.2
// (collocation heuristic), 4.1.1 (ortho heuristic), 4.1.3 (frequent
// sentence starter), 4.3 (initials and ordinals). The structure is
// 1:1 with upstream; the differences are:
//
//   - Collocation lookup assembles the `typ + "," + nextTyp` key into
//     a reusable byte buffer instead of running strings.Join, then
//     hits Storage.Collocations directly. The compiler's
//     `m[string(b)]` map-key elision keeps the lookup itself
//     allocation-free.
//   - Type strings come from typeNoPeriod/typeNoSentPeriod into the
//     pooled typeBufA / typeBufB so each Type call does not
//     allocate. Map lookups against the byte buffers use
//     `m[string(buf)]` to benefit from the same elision.
func tokenAnnotation(s *Storage, ortho *OrthoContext, tokOne, tokTwo *Token, st *state) {
	if tokTwo == nil {
		return
	}
	if !HasPeriodFinal(tokOne.Tok) {
		return
	}

	st.typeBufA = typeNoPeriod(tokOne.Tok, st.typeBufA[:0]) // typ
	st.typeBufB = typeNoSentPeriod(tokTwo, st.typeBufB[:0]) // nextTyp
	tokIsInitial := isInitial(tokOne.Tok)

	// 4.1.2 Collocation heuristic. Build `typ + "," + nextTyp` in
	// colKeyBuf and look it up. `m[string(b)]` is one of the two
	// documented compiler elisions for `string(b)` (the other is
	// equality compared to a string literal).
	st.colKeyBuf = append(st.colKeyBuf[:0], st.typeBufA...)
	st.colKeyBuf = append(st.colKeyBuf, ',')
	st.colKeyBuf = append(st.colKeyBuf, st.typeBufB...)
	if s.Collocations[string(st.colKeyBuf)] != 0 {
		tokOne.SentBreak = false
		tokOne.Abbr = true
		return
	}

	// 4.2 Token-based reclassification of abbreviations.
	if (tokOne.Abbr || isEllipsis(tokOne.Tok)) && !tokIsInitial {
		// heuristic uses typeBufA as scratch (typeBufB is "nextTyp"
		// and must survive for the map lookups below). typeBufA is
		// safe to clobber here because typ is no longer needed after
		// the collocation lookup above — equality-against-literal
		// uses are below and do not reach this branch.
		isSentStarter := ortho.heuristic(tokTwo, st.typeBufA[:0])
		if isSentStarter == 1 {
			tokOne.SentBreak = true
			return
		}
		if firstUpper(tokTwo.Tok) && s.SentStarters[string(st.typeBufB)] != 0 {
			tokOne.SentBreak = true
			return
		}
	}

	// Consecutive lone periods are part of a spaced ellipsis.
	if tokOne.Tok == "." && tokTwo.Tok == "." {
		tokOne.SentBreak = false
		tokTwo.SentBreak = false
		return
	}

	// 4.3 Initials and ordinals.
	// typeBufA still holds the original `typ`? No — the heuristic
	// call above may have clobbered it. Recompute when we still need
	// to compare typ against "##number##". `tokIsInitial` is already
	// known; we only need to recheck typ for the "##number##" case.
	if !tokIsInitial {
		// Re-derive typ — heuristic above may have clobbered typeBufA.
		st.typeBufA = typeNoPeriod(tokOne.Tok, st.typeBufA[:0])
	}
	isNumberType := !tokIsInitial && bytes.Equal(st.typeBufA, numberSentinelBytes)
	if !tokIsInitial && !isNumberType {
		return
	}
	isSentStarter := ortho.heuristic(tokTwo, st.typeBufA[:0])
	if isSentStarter == 0 {
		tokOne.SentBreak = false
		tokOne.Abbr = true
		return
	}
	if isSentStarter == -1 &&
		tokIsInitial &&
		firstUpper(tokTwo.Tok) &&
		s.OrthoContext[string(st.typeBufB)]&orthoLc == 0 {
		tokOne.SentBreak = false
		tokOne.Abbr = true
		return
	}
}

// numberSentinelBytes is the byte form of upstream's `##number##`
// sentinel. Compared via bytes.Equal so the comparison stays inside
// the byte domain (no string allocation).
var numberSentinelBytes = []byte("##number##")

// multiPunctAnnotation is the third-pass annotator (upstream
// english.MultiPunctWordAnnotation.tokenAnnotation), the
// abbreviation-aware classifier the fastpunct.go fork already runs.
// The fork's matchAbbrPattern is preserved (re-exported below as
// MatchAbbrPattern) so plan 191's DFA stays the abbreviation gate.
func multiPunctAnnotation(s *Storage, ortho *OrthoContext, tokOne, tokTwo *Token, st *state) {
	if isListNumber(tokOne.Tok) || isCoordinatePartOne(tokOne.Tok) {
		tokOne.SentBreak = false
		return
	}

	if hasSuffix(tokOne.Tok, ".") && tokTwo.Tok == "." {
		tokOne.SentBreak = false
		return
	}

	if !MatchAbbrPattern(tokOne.Tok) &&
		tokOne.Tok != "." &&
		!hasUnreliableEndChars(tokOne.Tok) &&
		!isCoordinateSecondPart(tokOne.Tok) {
		return
	}

	// Upstream's `if a.IsInitial(tokOne) { return }` guard is dead
	// here (see fastpunct.go for the dead-code analysis); elided.

	tokOne.Abbr = true
	tokOne.SentBreak = false

	st.typeBufB = typeNoSentPeriod(tokTwo, st.typeBufB[:0])
	isSentStarter := ortho.heuristic(tokTwo, st.typeBufA[:0])
	if isSentStarter == 1 {
		tokOne.SentBreak = true
		return
	}

	if firstUpper(tokTwo.Tok) &&
		(s.SentStarters[string(st.typeBufB)] != 0 ||
			hasUnreliableEndChars(tokOne.Tok) ||
			tokOne.Tok == "." ||
			isCoordinateSecondPart(tokOne.Tok)) {
		tokOne.SentBreak = true
		return
	}
}
