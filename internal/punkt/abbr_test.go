package punkt

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refReAbbr is the upstream regex from
// `neurosnap/sentences@v1.1.2/english/main.go:15`. The
// MatchAbbrPattern DFA must return the same membership answer as
// `refReAbbr.FindAllString(tok, 1) != nil` for every input. Plan 191
// owns the equivalence contract; this file mirrors the same suite
// inside the fork package so any drift fails here too.
var refReAbbr = regexp.MustCompile(`((?:[\w]\.)+[\w]*\.)`)

func abbrPatternRegex(tok string) bool {
	return len(refReAbbr.FindAllString(tok, 1)) > 0
}

func TestMatchAbbrPattern_Table(t *testing.T) {
	cases := []struct {
		tok  string
		want bool
	}{
		{"U.S.", true},
		{"p.m.", true},
		{"J.R.R.", true},
		{"a.b.c.", true},
		{"e.g.", true},
		{"Mr.", false},
		{"hello.", false},
		{"3.14", false},
		{".", false},
		{"", false},
		// "a..b" – the regex matches the prefix `a..`; record the
		// regex's truth.
		{"a..b", true},
	}
	for _, tc := range cases {
		require.Equalf(t, tc.want, abbrPatternRegex(tc.tok),
			"oracle disagrees with plan expectation for %q", tc.tok)
		assert.Equalf(t, tc.want, MatchAbbrPattern(tc.tok),
			"MatchAbbrPattern(%q) = %v, want %v",
			tc.tok, MatchAbbrPattern(tc.tok), tc.want)
	}
}

func TestMatchAbbrPattern_Fuzzy(t *testing.T) {
	alphabet := []rune{'a', 'b', 'Z', '0', '9', '_', '.', '-', ' ', '\x00'}
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
		got := MatchAbbrPattern(tok)
		require.Equalf(t, want, got,
			"divergence on %q: MatchAbbrPattern=%v, refReAbbr=%v",
			tok, got, want)
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
	require.Equal(t, len(alphabet)*len(alphabet)*len(alphabet)*len(alphabet)*len(alphabet), count)
}

func TestIsWordByte(t *testing.T) {
	for b := 0; b < 256; b++ {
		want := (b >= '0' && b <= '9') ||
			(b >= 'A' && b <= 'Z') ||
			(b >= 'a' && b <= 'z') ||
			b == '_'
		assert.Equalf(t, want, isWordByte(byte(b)),
			"isWordByte(0x%02x)", b)
	}
}
