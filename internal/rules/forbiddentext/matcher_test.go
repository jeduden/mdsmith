package forbiddentext

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// anyContains is the semantics matcher must reproduce exactly: the
// disjunction the rule's per-needle loop computes.
func anyContains(needles []string, text string) bool {
	for _, s := range needles {
		if s == "" {
			continue
		}
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

func TestMatcher_Cases(t *testing.T) {
	cases := []struct {
		name    string
		needles []string
		text    string
		want    bool
	}{
		{"no needles", nil, "anything at all", false},
		{"only empty needles", []string{"", ""}, "anything", false},
		{"empty text", []string{"delve"}, "", false},
		{"exact whole text", []string{"delve"}, "delve", true},
		{"absent", []string{"delve"}, "nothing here", false},
		{"at start", []string{"delve"}, "delve into it", true},
		{"at end", []string{"realm"}, "into the realm", true},
		{"in middle", []string{"realm"}, "the realm of x", true},
		// Substring semantics, not word semantics: strings.Contains
		// matches inside a larger word and matcher must agree.
		{"inside a longer word", []string{"robust"}, "robustness", true},
		{"multi-word phrase", []string{"deep dive"}, "a deep dive here", true},
		{"phrase split by newline", []string{"deep dive"}, "a deep\ndive", false},
		// One needle is a suffix of another: the longer needle's
		// terminal state must still report the shorter one.
		{"suffix needle", []string{"in the realm", "realm"}, "x in the realm", true},
		{"shorter suffix only", []string{"in the realm", "realm"}, "a realm", true},
		// A needle that is a prefix of another.
		{"prefix needle", []string{"dive", "dive into"}, "dive deep", true},
		// Bytes outside the needle alphabet must not corrupt state.
		{"unicode between", []string{"ab"}, "aéb", false},
		{"unicode needle", []string{"café"}, "a café here", true},
		{"repeated near-miss", []string{"aab"}, "aaaab", true},
		{"overlapping restart", []string{"aab"}, "aaa", false},
		{"case sensitive", []string{"Delve"}, "delve", false},
		{"tab and newline in text", []string{"x y"}, "a\tx y\nb", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newMatcher(tc.needles)
			assert.Equal(t, tc.want, m.matches(tc.text))
			// The oracle must agree, or the case itself is wrong.
			require.Equal(t, tc.want, anyContains(tc.needles, tc.text),
				"test case disagrees with strings.Contains oracle")
		})
	}
}

func TestNewMatcher_NilWhenNoUsableNeedle(t *testing.T) {
	assert.Nil(t, newMatcher(nil))
	assert.Nil(t, newMatcher([]string{}))
	assert.Nil(t, newMatcher([]string{"", ""}))
}

func TestMatcher_NilReceiverNeverMatches(t *testing.T) {
	var m *matcher
	assert.False(t, m.matches("anything"))
}

// TestMatcher_AgreesWithContains is the differential test: over many
// randomly generated needle sets and texts drawn from a deliberately
// tiny alphabet (so overlaps, prefixes, suffixes and restarts occur
// constantly), the automaton must return exactly what the per-needle
// strings.Contains loop returns.
func TestMatcher_AgreesWithContains(t *testing.T) {
	const alphabet = "aab c"
	rnd := rand.New(rand.NewSource(20260820))

	randString := func(maxLen int) string {
		n := rnd.Intn(maxLen + 1)
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteByte(alphabet[rnd.Intn(len(alphabet))])
		}
		return b.String()
	}

	for i := 0; i < 4000; i++ {
		needles := make([]string, rnd.Intn(5)+1)
		for j := range needles {
			needles[j] = randString(4)
		}
		text := randString(24)

		m := newMatcher(needles)
		assert.Equalf(t, anyContains(needles, text), m.matches(text),
			"needles=%q text=%q", needles, text)
	}
}

// TestMatcher_AgreesOnRealisticNeedles runs the same differential
// check against the real no-llm-tells-shaped list and prose-like text,
// including texts built to contain a needle.
func TestMatcher_AgreesOnRealisticNeedles(t *testing.T) {
	needles := benchNeedles()
	m := newMatcher(needles)
	require.NotNil(t, m)

	texts := []string{
		benchParagraph,
		"",
		"We should delve into this.",
		"It is a testament to the design.",
		"in today's fast-paced world things change",
		"navigating the complexities of the parser",
		"robustness is not the same as robust",
		"unlock the potential of nothing",
		strings.Repeat("plain prose without any tells. ", 40),
		strings.Repeat("a", 5000) + "delve",
	}
	for _, text := range texts {
		assert.Equalf(t, anyContains(needles, text), m.matches(text),
			"text=%.40q", text)
	}
}

// TestMatcher_ZeroAllocations pins the gate's reason for existing: the
// pass must not allocate, or it would trade CPU for GC pressure.
func TestMatcher_ZeroAllocations(t *testing.T) {
	m := newMatcher(benchNeedles())
	require.NotNil(t, m)

	avg := testing.AllocsPerRun(100, func() {
		_ = m.matches(benchParagraph)
	})
	assert.Zero(t, avg, "matcher.matches must not allocate")
}
