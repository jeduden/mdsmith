package punkt

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upstream reference regexes — verbatim from
// neurosnap/sentences@v1.1.2/token.go. The scanners in token.go must
// produce the exact same membership answers; these regexes are the
// oracle and every scanner test cross-checks against them.
var (
	refReEllipsis             = regexp.MustCompile(`\.\.+$`)
	refReInitial              = regexp.MustCompile(`^[A-Za-z]\.$`)
	refReListNumber           = regexp.MustCompile(`^\d+.?\)?$`)
	refReAlpha                = regexp.MustCompile(`^[A-Za-z]+$`)
	refReCoordinateSecondPart = regexp.MustCompile(`^[0-9]*\.[0-9]*\.[0-9]*\.$`)
	refReNumericPrefix        = regexp.MustCompile(`^##number##`)
)

// scannerCorpus is the universe of inputs every is*-scanner is
// checked against — combinations of letters, digits, periods, parens,
// the underscore, plus separators that the scanners must reject.
// Multi-byte runes are included so the `.?` semantics of isListNumber
// (regex `.` matches a rune, not a byte) stay pinned. Held at package
// scope so multiple tests share the same coverage. Each
// scanner-vs-regex test below asserts byte-for-byte equivalence on
// every entry.
var scannerCorpus = []string{
	"", ".", "..", "...", "....",
	"a", "A", "z", "Z", "0", "9",
	"a.", "A.", "ab", "Ab", "aB", "AB",
	"a.b", "ab.", "a.b.", "Ab.", "abC.",
	"U.S.", "U.S.A.", "p.m.", "a.m.", "e.g.", "i.e.",
	"J.R.R.", "F.B.I.",
	"3", "3.", "3.1", "3.14", "3.14.",
	"1.2.3", "1.2.3.", "1.0.0.", "12.34.56.",
	"1)", "1.)", "12)", "12.)", "abc)", "1a)",
	"a..", "a...", "a..b", ".a", ".a.",
	"##number##", "##number##.", "##number##abc",
	"hello.", "world", "Mr.",
	"_", "_.", "_a.", "a_.",
	`."`, `.'`, `.)`, `.’`, `.”`,
	`?"`, `?'`, `?)`, `?’`, `?”`,
	`!"`, `!'`, `!)`, `!’`, `!”`,
	`?`, `!`,
	`.(`, `.[`, `.[abc`, `?[xyz`,
	"中.文.", "中", "café.",
	// Multi-byte tail cases for isListNumber `.?`. Regex `.`
	// matches a rune; a byte-only scanner would diverge on these.
	"1世", "12世", "1世)", "1世)abc",
}

func TestIsAlphaToken_MatchesRegex(t *testing.T) {
	for _, tok := range scannerCorpus {
		want := refReAlpha.MatchString(tok)
		got := isAlphaToken(tok)
		assert.Equalf(t, want, got,
			"isAlphaToken(%q) = %v, refReAlpha = %v",
			tok, got, want)
	}
}

func TestIsInitial_MatchesRegex(t *testing.T) {
	for _, tok := range scannerCorpus {
		want := refReInitial.MatchString(tok)
		got := isInitial(tok)
		assert.Equalf(t, want, got,
			"isInitial(%q) = %v, refReInitial = %v",
			tok, got, want)
	}
}

func TestIsEllipsis_MatchesRegex(t *testing.T) {
	for _, tok := range scannerCorpus {
		want := refReEllipsis.MatchString(tok)
		got := isEllipsis(tok)
		assert.Equalf(t, want, got,
			"isEllipsis(%q) = %v, refReEllipsis = %v",
			tok, got, want)
	}
}

func TestIsListNumber_MatchesRegex(t *testing.T) {
	for _, tok := range scannerCorpus {
		want := refReListNumber.MatchString(tok)
		got := isListNumber(tok)
		assert.Equalf(t, want, got,
			"isListNumber(%q) = %v, refReListNumber = %v",
			tok, got, want)
	}
}

func TestIsCoordinateSecondPart_MatchesRegex(t *testing.T) {
	for _, tok := range scannerCorpus {
		want := refReCoordinateSecondPart.MatchString(tok)
		got := isCoordinateSecondPart(tok)
		assert.Equalf(t, want, got,
			"isCoordinateSecondPart(%q) = %v, refReCoordinateSecondPart = %v",
			tok, got, want)
	}
}

func TestIsNumberPrefix_MatchesRegex(t *testing.T) {
	for _, tok := range scannerCorpus {
		want := refReNumericPrefix.MatchString(tok)
		got := isNumberPrefix(tok)
		assert.Equalf(t, want, got,
			"isNumberPrefix(%q) = %v, refReNumericPrefix = %v",
			tok, got, want)
	}
}

// TestScanners_FuzzyAgainstRegexes is the property-style oracle: it
// enumerates every length-4 string over a structural alphabet hitting
// the relevant byte classes (letters, digits, period, paren, other
// non-word). Total: 9^4 = 6561 inputs per scanner. Any divergence
// from the reference regex fails fast with a concrete input. Cheap
// enough to run in unit-test time.
func TestScanners_FuzzyAgainstRegexes(t *testing.T) {
	alphabet := []byte{'a', 'Z', '0', '9', '.', ')', '_', '-', ' '}
	const n = 4
	indices := make([]int, n)
	buf := make([]byte, n)
	count := 0
	for {
		for i := 0; i < n; i++ {
			buf[i] = alphabet[indices[i]]
		}
		tok := string(buf)

		assert.Equalf(t, refReAlpha.MatchString(tok), isAlphaToken(tok),
			"isAlphaToken divergence on %q", tok)
		assert.Equalf(t, refReInitial.MatchString(tok), isInitial(tok),
			"isInitial divergence on %q", tok)
		assert.Equalf(t, refReEllipsis.MatchString(tok), isEllipsis(tok),
			"isEllipsis divergence on %q", tok)
		assert.Equalf(t, refReListNumber.MatchString(tok), isListNumber(tok),
			"isListNumber divergence on %q", tok)
		assert.Equalf(t, refReCoordinateSecondPart.MatchString(tok),
			isCoordinateSecondPart(tok),
			"isCoordinateSecondPart divergence on %q", tok)

		count++
		// base-len(alphabet) counter.
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
	require.Equal(t, len(alphabet)*len(alphabet)*len(alphabet)*len(alphabet), count)
}

func TestHasPeriodFinal(t *testing.T) {
	cases := map[string]bool{
		"":     false,
		".":    true,
		"a":    false,
		"a.":   true,
		"...":  true,
		"abc.": true,
		// CJK full-width period (U+3002, 3 bytes in UTF-8) — must
		// match upstream's HasPeriodFinal, which accepts it.
		"中文。": true,
		"。":   true,
		// Other CJK runes (no terminal period) — must NOT match.
		"中文": false,
		"中":  false,
		// `？` and `！` are sentence enders but not HasPeriodFinal
		// (that helper checks specifically for the period variants).
		"abc？": false,
		"abc！": false,
	}
	for tok, want := range cases {
		assert.Equalf(t, want, HasPeriodFinal(tok),
			"HasPeriodFinal(%q)", tok)
	}
}

func TestHasUnreliableEndChars(t *testing.T) {
	// Mirror the upstream English-only set.
	wants := map[string]bool{
		"":        false,
		`hello`:   false,
		`hello.`:  false,
		`hello."`: true,
		`hello.'`: true,
		`hello.)`: true,
		`hello.’`: true,
		`hello.”`: true,
		`hello?"`: true,
		`hello?’`: true,
		`hello!”`: true,
		`hello.[`: false, // .[ is sent-end but not unreliable
		// Lone `?` and `!` are sent-end, but not in the unreliable set.
		`?`: false,
		`!`: false,
	}
	for tok, want := range wants {
		assert.Equalf(t, want, hasUnreliableEndChars(tok),
			"hasUnreliableEndChars(%q)", tok)
	}
}

func TestHasSentEndChars(t *testing.T) {
	wants := map[string]bool{
		"":         false,
		"hello":    false,
		"hello.":   false, // bare period — not in the sent-end set
		`?`:        true,
		`!`:        true,
		`hello?"`:  true,
		`hello!”`:  true,
		`hello.")`: true, // contains `."` so matches via suffix `.")`? actually suffix is `")`
		`x.[y`:     true, // contains `.[` paren marker
		`y.(z`:     true,
	}
	for tok, want := range wants {
		assert.Equalf(t, want, hasSentEndChars(tok),
			"hasSentEndChars(%q)", tok)
	}
}

func TestToken_String(t *testing.T) {
	tok := Token{Tok: "abc.", Position: 5, SentBreak: true, Abbr: true}
	s := tok.String()
	assert.Contains(t, s, `"abc."`)
	assert.Contains(t, s, "SentBreak: true")
	assert.Contains(t, s, "Abbr: true")
	assert.Contains(t, s, "Position: 5")
}
