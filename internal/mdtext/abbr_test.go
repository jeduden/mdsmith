package mdtext

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reAbbrReference mirrors `english/main.go:15` reAbbr from
// neurosnap/sentences v1.1.2. It is the regex we replace: the
// hand-rolled DFA in matchAbbrPattern must return the same membership
// answer as `reAbbrReference.FindAllString(tok, 1) != nil` for every
// possible input — that is the equivalence contract MDS024 depends on.
var reAbbrReference = regexp.MustCompile(`((?:[\w]\.)+[\w]*\.)`)

// abbrPatternRegex is the boolean-membership oracle: true iff the
// upstream regex finds at least one match. The DFA must agree.
func abbrPatternRegex(tok string) bool {
	return len(reAbbrReference.FindAllString(tok, 1)) > 0
}

// TestMatchAbbrPattern_Table cross-checks every documented case from
// plan 191 task 2 against both the regex oracle and the DFA. A
// disagreement on any row fails fast: the test reports which input
// diverged and what each side returned.
func TestMatchAbbrPattern_Table(t *testing.T) {
	cases := []struct {
		tok  string
		want bool
	}{
		// Positive cases from the plan.
		{"U.S.", true},
		{"p.m.", true},
		{"J.R.R.", true},
		{"a.b.c.", true},
		{"e.g.", true},
		// Negative cases from the plan.
		{"Mr.", false},
		{"hello.", false},
		{"3.14", false},
		{".", false},
		{"", false},
		// The plan's negative list also lists "a..b", but the regex
		// actually matches the prefix "a..": `(?:[\w]\.)+` = "a.",
		// `[\w]*` = "", final `\.` = ".". The oracle is the regex
		// itself (see require below), so we record the regex's truth.
		{"a..b", true},
	}
	for _, tc := range cases {
		// Cross-check the oracle agrees with what the plan says.
		require.Equalf(t, tc.want, abbrPatternRegex(tc.tok),
			"oracle (reAbbr) disagrees with plan expectation for %q", tc.tok)
		assert.Equalf(t, tc.want, matchAbbrPattern(tc.tok),
			"matchAbbrPattern(%q) = %v, want %v (oracle=%v)",
			tc.tok, matchAbbrPattern(tc.tok), tc.want,
			abbrPatternRegex(tc.tok))
	}
}

// TestMatchAbbrPattern_EquivalentToRegex sweeps the tokens that the
// Punkt pipeline actually feeds reAbbr — period-ending strings of
// various shapes — and proves the DFA returns the same boolean as the
// regex for every one. The corpus is small but covers the structural
// shapes the regex distinguishes: a single letter+period, runs of
// letter+period, trailing alphanumerics, decimals, ellipses, mixed
// digits, leading/trailing punctuation, and CJK runes (which Go's
// `\w` rejects — `\w` is ASCII-only in regexp). The cases below
// verify the DFA agrees with the regex on every one.
func TestMatchAbbrPattern_EquivalentToRegex(t *testing.T) {
	cases := []string{
		"", ".", "..", "...", "....", "a", "a.", "a.b", "ab.",
		"a.b.", "a.b.c", "a.b.c.", "a.b.c.d",
		"U.S.", "U.S.A.", "U.S.A.A.", "p.m.", "a.m.", "e.g.", "i.e.",
		"J.R.R.", "J.R.R.Tolkien.", "F.B.I.",
		"3.14", "3.14.", "1.2.3", "1.2.3.", "12.34.56.",
		"Mr.", "Dr.", "etc.", "vs.", "Jan.", "hello.", "hello",
		"a..b", "a...b", "...a.", ".a.b.",
		"a1.b2.", "x9.", "9.x.",
		"_.a.", "a._.", "_._.", // underscore counts as \w
		"中.文.", // CJK runes are not \w in Go (\w is ASCII-only)
		"ABC", "abc", "Abc.",
		"a.b..", "a.b.c..",
		"\n", " ", "\t", "a\n.b.",
		"-.a.b.",
		"a.b\x00.", // NUL byte (not \w in Go)
	}
	for _, tok := range cases {
		want := abbrPatternRegex(tok)
		got := matchAbbrPattern(tok)
		assert.Equalf(t, want, got,
			"matchAbbrPattern(%q) = %v; reAbbr.FindAllString says %v",
			tok, got, want)
	}
}

// TestMatchAbbrPattern_FuzzyAgainstRegex exhaustively enumerates
// every length-5 string over a structural alphabet that exercises
// every transition of the DFA: word characters (letters / digits /
// underscore), the dot, plus a few non-word disruptors (space,
// hyphen, NUL). Every string is checked against the regex oracle.
// If the DFA diverges on any one, the test fails with both answers
// and the input.
//
// "Fuzzy" in the test name refers to the property-style coverage
// the plan's Risk section calls for; the enumeration itself is
// deterministic (not RNG-driven), so a failure reproduces
// trivially.
func TestMatchAbbrPattern_FuzzyAgainstRegex(t *testing.T) {
	alphabet := []rune{'a', 'b', 'Z', '0', '9', '_', '.', '-', ' ', '\x00'}
	// 5-rune strings × 10^5 = 100_000 — exhaustive enough to land every
	// short-string transition, cheap enough to run in unit-test time.
	const n = 5
	indices := make([]int, n)
	buf := make([]rune, n)
	count := 0
	for {
		for i := 0; i < n; i++ {
			buf[i] = alphabet[indices[i]]
		}
		tok := string(buf)
		want := abbrPatternRegex(tok)
		got := matchAbbrPattern(tok)
		require.Equalf(t, want, got,
			"divergence on %q: matchAbbrPattern=%v, reAbbr=%v",
			tok, got, want)
		count++

		// Increment the base-len(alphabet) counter.
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
	require.Equal(t, len(alphabet)*len(alphabet)*len(alphabet)*len(alphabet)*len(alphabet), count,
		"loop must visit every length-%d string in the alphabet", n)
}

// BenchmarkMatchAbbrPattern_DFA is the per-call cost of the
// hand-rolled scanner over a representative mix of period-ending
// tokens. Paired with BenchmarkMatchAbbrPattern_Regex below, the two
// numbers show the per-call savings that motivate the plan.
func BenchmarkMatchAbbrPattern_DFA(b *testing.B) {
	tokens := abbrBenchTokens
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tok := range tokens {
			_ = matchAbbrPattern(tok)
		}
	}
}

// BenchmarkMatchAbbrPattern_Regex is the upstream behaviour, kept
// as a comparison baseline for the DFA. Same token mix.
func BenchmarkMatchAbbrPattern_Regex(b *testing.B) {
	tokens := abbrBenchTokens
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tok := range tokens {
			_ = abbrPatternRegex(tok)
		}
	}
}

// abbrBenchTokens is the representative mix MDS024 sees on prose:
// abbreviations, decimals, plain words ending in a period, and a few
// non-matches the cheap-bounds guard already lets through.
var abbrBenchTokens = func() []string {
	base := []string{
		"U.S.", "p.m.", "a.m.", "e.g.", "i.e.", "etc.", "vs.",
		"J.R.R.", "F.B.I.", "Mr.", "Dr.", "Jan.", "hello.", "world.",
		"3.14", "1.2.3", "section.", "value.", "boundary.",
	}
	// Repeat to amortize loop overhead and make the benchmark
	// stable across runs.
	out := make([]string, 0, 4*len(base))
	for i := 0; i < 4; i++ {
		out = append(out, base...)
	}
	return out
}()
