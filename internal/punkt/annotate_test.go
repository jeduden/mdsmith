package punkt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestState constructs a freshly-initialized state with capacities
// matching New()'s pool factory, so tests can drive annotator
// branches without going through a Tokenizer.
func newTestState() *state {
	return &state{
		tokens:    make([]Token, 0, 16),
		ptrs:      make([]*Token, 0, 16),
		pairs:     make([][2]*Token, 0, 16),
		typeBufA:  make([]byte, 0, 32),
		typeBufB:  make([]byte, 0, 32),
		colKeyBuf: make([]byte, 0, 64),
		sents:     make([]Sentence, 0, 8),
	}
}

func TestGroup_EmptyTokensYieldsEmptyPairs(t *testing.T) {
	st := newTestState()
	pairs := group(st, nil)
	assert.Empty(t, pairs, "group must produce no pairs for empty input")
}

func TestGroup_AdjacentPairsAndTrailingNil(t *testing.T) {
	st := newTestState()
	a := &Token{Tok: "a"}
	b := &Token{Tok: "b"}
	c := &Token{Tok: "c"}
	pairs := group(st, []*Token{a, b, c})
	require.Len(t, pairs, 3,
		"group must emit (a,b), (b,c), (c,nil)")
	assert.Equal(t, [2]*Token{a, b}, pairs[0])
	assert.Equal(t, [2]*Token{b, c}, pairs[1])
	assert.Equal(t, [2]*Token{c, nil}, pairs[2],
		"trailing pair must be (last, nil) — Annotate uses it as the "+
			"end-of-stream sentinel")
}

func TestTypeAnnotation_SentEndCharsTriggerBreak(t *testing.T) {
	s := NewStorage()
	tok := &Token{Tok: `hello!"`}
	typeAnnotation(s, tok)
	assert.True(t, tok.SentBreak,
		"a token ending in `!\"` matches hasSentEndChars and must be "+
			"flagged as a sentence break")
}

func TestTypeAnnotation_DoubleDotIsIgnored(t *testing.T) {
	s := NewStorage()
	tok := &Token{Tok: "a.."}
	typeAnnotation(s, tok)
	assert.False(t, tok.SentBreak,
		"upstream's `hasSuffix(.., '..')` guard short-circuits the "+
			"abbrev branch and must not flip SentBreak")
	assert.False(t, tok.Abbr)
}

func TestTypeAnnotation_HyphenatedAbbrev(t *testing.T) {
	s := NewStorage()
	s.AbbrevTypes.Add("usa") // tail after the last hyphen
	tok := &Token{Tok: "U.S.-USA."}
	typeAnnotation(s, tok)
	assert.True(t, tok.Abbr,
		"strings.LastIndexByte path: the hyphen tail must hit the "+
			"AbbrevTypes set")
}

// TestTypeAnnotation_CjkPeriodStrippingDropsFullRune pins the
// rune-correct period stripping. Upstream uses
// `token.Tok[:len(chars)-1]` which slices by rune-count interpreted
// as a BYTE offset — for "中文。" (3 runes, 9 bytes) it produces the
// first 2 bytes, an invalid UTF-8 prefix of 中. A byte-only strip
// (`token.Tok[:len(token.Tok)-1]`) is similarly wrong: it keeps two
// bytes of the CJK period 。 (also invalid UTF-8). The correct
// behaviour drops one full rune via utf8.DecodeLastRuneInString.
//
// Drive the contract red/green: seed AbbrevTypes with the
// stripped form "中文" and verify the lookup hits (Abbr=true,
// SentBreak=false). With the byte-strip both bugs above produce
// invalid UTF-8 that AbbrevTypes never contains, so the rule
// would mark the token as a sentence break instead.
func TestTypeAnnotation_CjkPeriodStrippingDropsFullRune(t *testing.T) {
	s := NewStorage()
	s.AbbrevTypes.Add("中文")
	tok := &Token{Tok: "中文。"}
	typeAnnotation(s, tok)
	assert.True(t, tok.Abbr,
		"a CJK token ending in 。 with the stripped form in "+
			"AbbrevTypes must classify as Abbr — proves the rune-"+
			"aware period strip works")
	assert.False(t, tok.SentBreak,
		"a recognised CJK abbreviation must not be flagged as a "+
			"sentence break")
}

// TestTypeAnnotation_AsciiPeriodStrippingUnchanged is the regression
// counterpart: the rune-aware strip must also still work for ASCII
// `.`-ending tokens (the most common case). Drops one rune of size
// 1, identical to the previous byte-strip behaviour.
func TestTypeAnnotation_AsciiPeriodStrippingUnchanged(t *testing.T) {
	s := NewStorage()
	s.AbbrevTypes.Add("dr")
	tok := &Token{Tok: "Dr."}
	typeAnnotation(s, tok)
	assert.True(t, tok.Abbr,
		"the ASCII case must still match after the rune-aware "+
			"strip — `Dr.` → strip 1 byte → lowercase → `dr`")
}

func TestTypeAnnotation_UppercaseTokenLowercasesForAbbrevLookup(t *testing.T) {
	// hasAbbrLowered's non-ASCII / uppercase branch.
	s := NewStorage()
	s.AbbrevTypes.Add("dr")
	tok := &Token{Tok: "Dr."}
	typeAnnotation(s, tok)
	assert.True(t, tok.Abbr,
		"uppercase token must lowercase before AbbrevTypes lookup")
}

func TestIsAllASCIILower_NonASCII(t *testing.T) {
	// Drive the c >= 0x80 branch in isAllASCIILower.
	assert.False(t, isAllASCIILower("café"),
		"non-ASCII byte (≥ 0x80) breaks the all-ASCII-lower fast path")
}

func TestIsAllASCIILower_AsciiUpper(t *testing.T) {
	assert.False(t, isAllASCIILower("ABc"),
		"ASCII uppercase breaks the all-lower fast path")
}

func TestIsAllASCIILower_AsciiLower(t *testing.T) {
	assert.True(t, isAllASCIILower("abc"))
	assert.True(t, isAllASCIILower(""),
		"empty string is trivially all-ASCII-lower")
}

func TestTokenAnnotation_NilTokTwoReturnsEarly(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "U.S."}
	tokenAnnotation(s, o, tokOne, nil, st)
	assert.False(t, tokOne.SentBreak,
		"nil tokTwo must short-circuit before any annotation")
	assert.False(t, tokOne.Abbr)
}

func TestTokenAnnotation_NoPeriodFinalReturnsEarly(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "no-period"}
	tokTwo := &Token{Tok: "next"}
	tokenAnnotation(s, o, tokOne, tokTwo, st)
	assert.False(t, tokOne.SentBreak)
	assert.False(t, tokOne.Abbr)
}

func TestTokenAnnotation_CollocationHitDemotesBreak(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	// upstream key shape: `typ + "," + nextTyp`
	s.Collocations["u.s,supreme"] = 1
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "U.S.", SentBreak: true}
	tokTwo := &Token{Tok: "Supreme"}
	tokenAnnotation(s, o, tokOne, tokTwo, st)
	assert.False(t, tokOne.SentBreak,
		"collocation hit must demote the sentence break")
	assert.True(t, tokOne.Abbr,
		"collocation hit must mark as abbreviation")
}

func TestTokenAnnotation_OrthoHeuristicElevatesAbbrev(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	// Heuristic returns 1 when tokTwo is capitalized AND its
	// orthotype has any lowercase bit AND no orthoMidUc bit.
	s.OrthoContext["next"] = orthoBegLc
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "etc.", Abbr: true}
	tokTwo := &Token{Tok: "Next"}
	tokenAnnotation(s, o, tokOne, tokTwo, st)
	assert.True(t, tokOne.SentBreak,
		"Ortho.Heuristic==1 must promote an abbreviation back to a "+
			"sentence break")
}

func TestTokenAnnotation_FirstUpperSentStarterPromotes(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	s.SentStarters["world"] = 1
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "etc.", Abbr: true}
	tokTwo := &Token{Tok: "World"}
	tokenAnnotation(s, o, tokOne, tokTwo, st)
	assert.True(t, tokOne.SentBreak,
		"FirstUpper + SentStarters hit must restore the sentence break")
}

func TestTokenAnnotation_LoneDotsClearBothBreaks(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: ".", SentBreak: true}
	tokTwo := &Token{Tok: ".", SentBreak: true}
	tokenAnnotation(s, o, tokOne, tokTwo, st)
	assert.False(t, tokOne.SentBreak,
		"consecutive lone periods are part of a spaced ellipsis")
	assert.False(t, tokTwo.SentBreak,
		"the second lone period must also drop its break")
}

func TestTokenAnnotation_InitialSentStarterZeroDemotes(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	// Heuristic returns 0 when tokTwo is lowercase AND its orthotype
	// has the uppercase-internal bit set.
	s.OrthoContext["then"] = orthoMidUc
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "J.", SentBreak: true}
	tokTwo := &Token{Tok: "then"}
	tokenAnnotation(s, o, tokOne, tokTwo, st)
	assert.False(t, tokOne.SentBreak,
		"initial + sent-starter == 0 must demote the break")
	assert.True(t, tokOne.Abbr,
		"initial + sent-starter == 0 must promote to abbreviation")
}

func TestTokenAnnotation_InitialUnknownButCapitalizedDemotes(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	o := &OrthoContext{Storage: s} // no ortho data → heuristic returns -1
	tokOne := &Token{Tok: "J.", SentBreak: true}
	tokTwo := &Token{Tok: "Bach"}
	tokenAnnotation(s, o, tokOne, tokTwo, st)
	assert.False(t, tokOne.SentBreak,
		"initial + unknown ortho + capitalized next must demote "+
			"(special initials rule)")
	assert.True(t, tokOne.Abbr)
}

func TestMultiPunctAnnotation_ListNumberClears(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "1.", SentBreak: true}
	tokTwo := &Token{Tok: "Item"}
	multiPunctAnnotation(s, o, tokOne, tokTwo, st)
	assert.False(t, tokOne.SentBreak,
		"a list-number token must lose its SentBreak")
}

func TestMultiPunctAnnotation_CoordinatePartOneClears(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "N°.", SentBreak: true}
	tokTwo := &Token{Tok: "1.2.3."}
	multiPunctAnnotation(s, o, tokOne, tokTwo, st)
	assert.False(t, tokOne.SentBreak,
		"IsCoordinatePartOne (`N°.`) must clear the break")
}

func TestMultiPunctAnnotation_PeriodFollowedByPeriodClears(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "abc.", SentBreak: true}
	tokTwo := &Token{Tok: "."}
	multiPunctAnnotation(s, o, tokOne, tokTwo, st)
	assert.False(t, tokOne.SentBreak,
		"period-final tok followed by a lone period is a spaced "+
			"ellipsis fragment, never a sentence break")
}

func TestMultiPunctAnnotation_EarlyReturnOnNoTriggers(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "hello."}
	tokTwo := &Token{Tok: "World"}
	multiPunctAnnotation(s, o, tokOne, tokTwo, st)
	assert.False(t, tokOne.SentBreak,
		"`hello.` matches no abbr indicator and must skip the body")
	assert.False(t, tokOne.Abbr)
}

func TestMultiPunctAnnotation_OrthoHeuristicElevates(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	s.OrthoContext["next"] = orthoBegLc
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "U.S."}
	tokTwo := &Token{Tok: "Next"}
	multiPunctAnnotation(s, o, tokOne, tokTwo, st)
	assert.True(t, tokOne.Abbr)
	assert.True(t, tokOne.SentBreak,
		"Ortho.Heuristic==1 must elevate this to a sentence break")
}

func TestMultiPunctAnnotation_FirstUpperUnreliableEndsElevate(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: `It."`}
	tokTwo := &Token{Tok: "World"}
	multiPunctAnnotation(s, o, tokOne, tokTwo, st)
	assert.True(t, tokOne.SentBreak,
		`hasUnreliableEndChars (suffix ."): the final gate's second `+
			"disjunct must restore the sentence break")
}

func TestMultiPunctAnnotation_FirstUpperLonePeriodElevates(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "."}
	tokTwo := &Token{Tok: "World"}
	multiPunctAnnotation(s, o, tokOne, tokTwo, st)
	assert.True(t, tokOne.SentBreak,
		`tokOne == "." + FirstUpper(tokTwo) must restore the break `+
			"(third disjunct of the final gate)")
}

func TestMultiPunctAnnotation_FirstUpperCoordinatePartTwoElevates(t *testing.T) {
	st := newTestState()
	s := NewStorage()
	o := &OrthoContext{Storage: s}
	tokOne := &Token{Tok: "1.2.3."}
	tokTwo := &Token{Tok: "World"}
	multiPunctAnnotation(s, o, tokOne, tokTwo, st)
	assert.True(t, tokOne.SentBreak,
		"IsCoordinatePartTwo + FirstUpper must restore the break "+
			"(fourth disjunct of the final gate)")
}

func TestState_ResetClearsTokAndTextReferences(t *testing.T) {
	// The whole point of reset() is to drop the Tok and Text string
	// references so the previous input can be GC'd while the pooled
	// state is dormant. Pin that contract explicitly.
	st := newTestState()
	// Use a string literal we can identify in the backing arrays.
	const sentinel = "MDS024-PIN"
	st.tokens = append(st.tokens, Token{Tok: sentinel, Position: 1})
	st.tokens = append(st.tokens, Token{Tok: "another", Position: 2})
	st.ptrs = append(st.ptrs, &st.tokens[0], &st.tokens[1])
	st.pairs = append(st.pairs, [2]*Token{&st.tokens[0], &st.tokens[1]})
	st.sents = append(st.sents, Sentence{Start: 0, End: 1, Text: sentinel})
	st.sents = append(st.sents, Sentence{Start: 1, End: 2, Text: "another"})
	st.typeBufA = append(st.typeBufA, 'x')
	st.typeBufB = append(st.typeBufB, 'y')
	st.colKeyBuf = append(st.colKeyBuf, 'z')

	st.reset()

	require.Empty(t, st.tokens, "len(tokens) must be 0 after reset")
	require.Empty(t, st.ptrs)
	require.Empty(t, st.pairs)
	require.Empty(t, st.sents)
	require.Empty(t, st.typeBufA)
	require.Empty(t, st.typeBufB)
	require.Empty(t, st.colKeyBuf)

	// The backing arrays must be cleared so the sentinel string is
	// not retained.
	full := st.tokens[:cap(st.tokens)]
	for i := range full {
		assert.Emptyf(t, full[i].Tok,
			"tokens[%d].Tok must be cleared so the previous input can be GC'd", i)
	}
	fullSents := st.sents[:cap(st.sents)]
	for i := range fullSents {
		assert.Emptyf(t, fullSents[i].Text,
			"sents[%d].Text must be cleared", i)
	}
	fullPtrs := st.ptrs[:cap(st.ptrs)]
	for i := range fullPtrs {
		assert.Nilf(t, fullPtrs[i],
			"ptrs[%d] must be cleared so its referent can be GC'd", i)
	}
	fullPairs := st.pairs[:cap(st.pairs)]
	for i := range fullPairs {
		assert.Equalf(t, [2]*Token{}, fullPairs[i],
			"pairs[%d] must be cleared", i)
	}
}
