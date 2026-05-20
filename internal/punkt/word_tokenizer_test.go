package punkt

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// referenceType is the upstream DefaultWordTokenizer.Type
// implementation, inlined here as the typeOf oracle. typeOf must
// produce the same bytes for every input.
//
// Source: github.com/neurosnap/sentences@v1.1.2/word_tokenizer.go:153
var (
	refReNumericTail = regexp.MustCompile(`-?[\.,]?\d[\d,\.-]*\.?$`)
)

func referenceType(tok string) string {
	typ := refReNumericTail.ReplaceAllString(strings.ToLower(tok), "##number##")
	if len(typ) == 1 {
		return typ
	}
	return strings.ReplaceAll(typ, ",", "")
}

// typeOfCorpus is the input universe for the typeOf equivalence
// tests. Covers every shape the regex distinguishes — empty, ASCII
// letter, digit, decimal, multi-period, abbreviation, comma-inside,
// hyphen-prefix, dot-prefix, non-ASCII letter, single-char specials.
var typeOfCorpus = []string{
	"", "a", "A", "z", "Z", "0", "9", ".", ",", "-",
	"abc", "ABC", "Abc", "aBcD",
	"3", "3.14", "3.14.", "12", "12.", "12,345", "12,345.",
	"-3", "-3.14", "-.5", ".5", ",5",
	"1.2.3", "1.2.3.", "1..2", "1.2.",
	"a1", "a1.", "abc123", "abc123.", "abc,123.",
	"U.S.", "U.S.A.", "p.m.", "Mr.", "hello.",
	"café", "café.", "café1.", "Café",
	"a,b", "a,b,c.",
	"_", "_abc", "abc_",
	"##number##", "##number##.",
}

func TestTypeOf_MatchesUpstream(t *testing.T) {
	buf := make([]byte, 0, 64)
	for _, tok := range typeOfCorpus {
		want := referenceType(tok)
		buf = typeOf(tok, buf[:0])
		got := string(buf)
		assert.Equalf(t, want, got, "typeOf(%q) mismatch", tok)
	}
}

// TestTypeOf_FuzzyAgainstUpstream is the property-style sweep: every
// length-3 string over a structural alphabet (letters, digits,
// period, comma, hyphen, non-ASCII byte). 8^3 = 512 inputs.
func TestTypeOf_FuzzyAgainstUpstream(t *testing.T) {
	alphabet := []byte{'a', 'A', '0', '9', '.', ',', '-', 0xc3}
	const n = 3
	indices := make([]int, n)
	buf := make([]byte, n)
	scanBuf := make([]byte, 0, 32)
	count := 0
	for {
		for i := 0; i < n; i++ {
			buf[i] = alphabet[indices[i]]
		}
		tok := string(buf)
		want := referenceType(tok)
		scanBuf = typeOf(tok, scanBuf[:0])
		got := string(scanBuf)
		require.Equalf(t, want, got, "typeOf(%q) mismatch", tok)
		count++

		j := n - 1
		for j >= 0 {
			indices[j]++
			if indices[j] < len(alphabet) {
				break
			}
			indices[j] = 0
			j--
		}
		if j < 0 {
			break
		}
	}
	require.Equal(t, len(alphabet)*len(alphabet)*len(alphabet), count)
}

func TestTypeOf_AllocFree(t *testing.T) {
	buf := make([]byte, 0, 64)
	allocs := testing.AllocsPerRun(100, func() {
		buf = typeOf("123.45.6.", buf[:0])
		_ = buf
		buf = typeOf("p.m.", buf[:0])
		_ = buf
		buf = typeOf("hello,world.", buf[:0])
		_ = buf
		buf = typeOf("U.S.A.", buf[:0])
		_ = buf
	})
	// AllocsPerRun returns the average over the run; for the typical
	// short tokens above the buf has already grown to fit, so steady
	// state is 0.
	assert.InDeltaf(t, 0.0, allocs, 0.5,
		"typeOf must be allocation-free on a steady-state buffer; got %.2f",
		allocs)
}

func TestNumericTail_MatchesRegex(t *testing.T) {
	for _, tok := range typeOfCorpus {
		want := refReNumericTail.FindStringIndex(tok)
		start, end, ok := numericTail(tok)
		if want == nil {
			assert.Falsef(t, ok,
				"numericTail(%q) should miss but reported [%d,%d)",
				tok, start, end)
			continue
		}
		require.Truef(t, ok,
			"numericTail(%q) should hit; regex says [%d,%d)",
			tok, want[0], want[1])
		assert.Equalf(t, want[0], start,
			"numericTail(%q) start: got %d, regex says %d",
			tok, start, want[0])
		assert.Equalf(t, want[1], end,
			"numericTail(%q) end: got %d, regex says %d",
			tok, end, want[1])
	}
}

func TestHasSentencePunct(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"hello": false,
		"a.":    true,
		"a?":    true,
		"a!":    true,
		"abc。":  false, // CJK punctuation dropped per doc.go
		"abc？":  false,
		"abc！":  false,
	}
	for s, want := range cases {
		assert.Equalf(t, want, hasSentencePunct(s),
			"hasSentencePunct(%q)", s)
	}
}

func TestFirstUpper_FirstLower(t *testing.T) {
	cases := []struct {
		tok                string
		wantUpper, wantLow bool
	}{
		{"", false, false},
		{"a", false, true},
		{"A", true, false},
		{"1", false, false},
		{".", false, false},
		{"Hello", true, false},
		{"hello", false, true},
		{"Éclair", true, false},
		{"éclair", false, true},
	}
	for _, c := range cases {
		assert.Equalf(t, c.wantUpper, firstUpper(c.tok),
			"firstUpper(%q)", c.tok)
		assert.Equalf(t, c.wantLow, firstLower(c.tok),
			"firstLower(%q)", c.tok)
	}
}

func TestIsCoordinatePartOne(t *testing.T) {
	assert.True(t, isCoordinatePartOne("N°."))
	assert.False(t, isCoordinatePartOne("n°."))
	assert.False(t, isCoordinatePartOne("N°"))
	assert.False(t, isCoordinatePartOne(""))
}

func TestTokenize_EmptyText(t *testing.T) {
	assert.Nil(t, Tokenize("", false),
		"empty input must produce no tokens")
}

func TestTokenize_BasicWords(t *testing.T) {
	toks := Tokenize("Hello world.", false)
	require.Len(t, toks, 2)
	assert.Equal(t, "Hello", toks[0].Tok)
	assert.Equal(t, "world.", toks[1].Tok)
}

// TestTokenize_MatchesUpstreamShape pins the per-token output of
// Tokenize against upstream by reproducing upstream's
// DefaultWordTokenizer.Tokenize on the same input via the
// neurosnap/sentences package — sentence_equivalence_test.go is the
// integration gate, this is the unit-level pin. The check is on
// (Tok, Position, ParaStart, LineStart) since those are the fields
// downstream annotators read.
func TestTokenize_MatchesUpstreamShape(t *testing.T) {
	for _, text := range []string{
		"Hello world.",
		"Hello\nworld.",
		"Hello.\n\nworld.",
		"A. B. C.",
		"  leading space",
		"trailing.",
		"x",
		"",
	} {
		got := Tokenize(text, false)
		want := upstreamTokens(text)
		require.Equalf(t, len(want), len(got),
			"token count mismatch on %q: want %d (%v), got %d (%v)",
			text, len(want), want, len(got), got)
		for i := range got {
			assert.Equalf(t, want[i].Tok, got[i].Tok,
				"token[%d].Tok mismatch on %q", i, text)
			assert.Equalf(t, want[i].Position, got[i].Position,
				"token[%d].Position mismatch on %q", i, text)
			assert.Equalf(t, want[i].ParaStart, got[i].ParaStart,
				"token[%d].ParaStart mismatch on %q", i, text)
			assert.Equalf(t, want[i].LineStart, got[i].LineStart,
				"token[%d].LineStart mismatch on %q", i, text)
		}
	}
}

func TestTokenize_OnlyPeriodContext_FiltersNonPunctWords(t *testing.T) {
	toks := Tokenize("Hello world. Foo bar baz", true)
	// Upstream behaviour: emits `world.` (has period) and `Foo`
	// (token immediately after), then stops because no further punct
	// triggers getNextWord. `Hello`, `bar`, and `baz` are filtered.
	got := make([]string, 0, len(toks))
	for _, t := range toks {
		got = append(got, t.Tok)
	}
	require.Equal(t, []string{"world.", "Foo"}, got)
}

func TestTokenize_TokenizeInto_AppendsToProvidedSlice(t *testing.T) {
	dst := make([]Token, 0, 8)
	dst = TokenizeInto(dst, "alpha beta.", false)
	require.Len(t, dst, 2)
	assert.Equal(t, "alpha", dst[0].Tok)
	assert.Equal(t, "beta.", dst[1].Tok)

	// A second call into the same slice resets to dst[:0] in callers,
	// but TokenizeInto itself appends. Drive the bare-append shape.
	dst = TokenizeInto(dst, "gamma.", false)
	require.Len(t, dst, 3, "TokenizeInto must append, not overwrite")
	assert.Equal(t, "gamma.", dst[2].Tok)
}

func TestTokenize_WhitespaceOnlyEmitsSingleToken(t *testing.T) {
	// Upstream branch: `if len(tokens) == 0 { append NewToken(text) }`.
	// A text of pure whitespace runs through the loop, the trim drops
	// every word, and the fallback emits one token equal to the whole
	// input. Drive that branch red/green.
	toks := Tokenize("   \n\t  ", false)
	require.Len(t, toks, 1)
	assert.Equal(t, "   \n\t  ", toks[0].Tok)
	assert.Equal(t, len("   \n\t  "), toks[0].Position)
}

func TestTokenizeInto_EmptyText(t *testing.T) {
	// TokenizeInto is exported, so an external caller could pass an
	// empty string. The early-return branch keeps the per-rune loop
	// from running against zero bytes.
	dst := make([]Token, 0, 4)
	got := TokenizeInto(dst, "", false)
	assert.Empty(t, got, "empty text must produce no tokens")
}

// TestTokenize_TrailingMultiByteRuneMatchesUpstream pins the
// matched behaviour between fork and upstream for an input that
// ends in a multi-byte rune without trailing whitespace. The
// per-byte `i == textLength-1` end-of-input check in upstream's
// loop misses the start byte of the final multi-byte rune, so the
// last word is not emitted into the token slice. The fork
// inherits that quirk verbatim to preserve byte-equivalence; the
// downstream sentence emitter's trailing-fallback (text[lastBreak:])
// still includes the tail bytes in the final Sentence, so the
// user-visible output is unaffected.
func TestTokenize_TrailingMultiByteRuneMatchesUpstream(t *testing.T) {
	for _, text := range []string{
		"Hello 世",
		"abc 世界",
		"line one\nworld 世",
	} {
		got := Tokenize(text, false)
		want := upstreamTokens(text)
		require.Equalf(t, len(want), len(got),
			"trailing-multi-byte token count mismatch on %q", text)
		for i := range got {
			assert.Equalf(t, want[i].Tok, got[i].Tok,
				"trailing-multi-byte token[%d].Tok mismatch on %q", i, text)
		}
	}
}

func TestUpdateLineFlags_DoubleNewlineMarksParaStart(t *testing.T) {
	// Drives the `if c.lineStart { c.paragraphStart = true }` branch.
	// Upstream's logic only flips paragraphStart when a '\n' is seen
	// while lineStart is already true — which happens when a previous
	// '\n' fired without an intervening token emit (i.e. consecutive
	// blank-line newlines). Three '\n' in a row do it: the first
	// emits the prior token and resets flags; the second sets
	// lineStart=true (empty emit, no reset); the third sees
	// lineStart=true and promotes to paragraphStart=true.
	// Cross-checked against upstream by upstreamTokens.
	const text = "A.\n\n\nB."
	got := Tokenize(text, false)
	want := upstreamTokens(text)
	require.Equal(t, want, got,
		"fork's ParaStart promotion must match upstream byte-for-byte")
	require.Len(t, got, 2)
	assert.Equal(t, "B.", got[1].Tok)
	assert.True(t, got[1].ParaStart,
		"the token after two blank lines must have ParaStart=true")
}
