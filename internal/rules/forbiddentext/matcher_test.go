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

// TestMatcherCacheKey_DistinguishesLists is the regression test for a
// cache-key collision: the key was a NUL-joined concatenation, so a
// single needle containing the separator byte produced the same key as
// the two-needle list it splits into. The second list then reused the
// first's automaton and MDS056 silently stopped reporting.
func TestMatcherCacheKey_DistinguishesLists(t *testing.T) {
	collisionPairs := [][2][]string{
		{{"a\x00b"}, {"a", "b"}},
		{{"ab"}, {"a", "b"}},
		{{""}, {}},
		{{"a", ""}, {"a"}},
		{{"1:a"}, {"a"}},
		{{"x", "yz"}, {"xy", "z"}},
	}
	for _, pair := range collisionPairs {
		assert.NotEqualf(t, matcherCacheKey(pair[0]), matcherCacheKey(pair[1]),
			"distinct needle lists %q and %q share a cache key",
			pair[0], pair[1])
	}

	// Equal lists must still share a key, or the cache never hits.
	assert.Equal(t,
		matcherCacheKey([]string{"delve", "realm"}),
		matcherCacheKey([]string{"delve", "realm"}))
}

// TestCachedMatcher_ReusesAndSeparates covers both cache paths: a
// repeat call for the same list returns the identical instance, and a
// different list does not.
func TestCachedMatcher_ReusesAndSeparates(t *testing.T) {
	first := cachedMatcher([]string{"alpha", "beta"})
	again := cachedMatcher([]string{"alpha", "beta"})
	require.NotNil(t, first)
	assert.Same(t, first, again, "the same list must reuse one automaton")

	other := cachedMatcher([]string{"alpha\x00beta"})
	require.NotNil(t, other)
	assert.NotSame(t, first, other,
		"a needle containing the old separator must not reuse the joined list's automaton")

	// The separated automaton must behave per its own list.
	assert.True(t, first.matches("say alpha here"))
	assert.False(t, other.matches("say alpha here"))
	assert.True(t, other.matches("say alpha\x00beta here"))
}

// TestCachedMatcher_NilForEmptyList pins that an unusable list caches
// and returns nil rather than an automaton that matches nothing.
func TestCachedMatcher_NilForEmptyList(t *testing.T) {
	assert.Nil(t, cachedMatcher(nil))
	assert.Nil(t, cachedMatcher([]string{""}))
}

// TestMatcher_BuildAlphabet checks the compact-alphabet mapping: every
// byte used by a needle gets its own symbol, everything else shares 0.
func TestMatcher_BuildAlphabet(t *testing.T) {
	var m matcher
	require.True(t, m.buildAlphabet([]string{"ab", "bc"}))

	assert.Equal(t, 4, m.nsym, "three distinct bytes plus the shared symbol")
	assert.NotZero(t, m.symbol['a'])
	assert.NotZero(t, m.symbol['b'])
	assert.NotZero(t, m.symbol['c'])
	assert.Zero(t, m.symbol['z'], "an unused byte stays on the shared symbol")

	var empty matcher
	assert.False(t, empty.buildAlphabet([]string{"", ""}),
		"a list with no usable needle reports no alphabet")
}

// TestMatcher_BuildTrie checks the goto table: shared prefixes share
// nodes, and only needle ends are marked terminal.
func TestMatcher_BuildTrie(t *testing.T) {
	var m matcher
	require.True(t, m.buildAlphabet([]string{"ab", "ac"}))
	trie, isEnd := m.buildTrie([]string{"ab", "ac"})

	require.Len(t, isEnd, 4, "root, shared 'a', then 'b' and 'c'")
	assert.False(t, isEnd[0], "the root is never a needle end")

	a := trie[int(m.symbol['a'])]
	require.Positive(t, a, "the root has an edge for 'a'")
	assert.False(t, isEnd[a], "'a' alone is not a needle")

	ends := 0
	for _, e := range isEnd {
		if e {
			ends++
		}
	}
	assert.Equal(t, 2, ends, "exactly the two needles end somewhere")
}

// TestMatcher_FoldFailLinks checks the property the fold exists to
// give: every (state, symbol) pair has a direct successor, so matching
// never walks a fail chain.
func TestMatcher_FoldFailLinks(t *testing.T) {
	m := newMatcher([]string{"ab", "bc"})
	require.NotNil(t, m)

	require.Len(t, m.next, len(m.terminal)*m.nsym)
	for state := range m.terminal {
		for sym := 0; sym < m.nsym; sym++ {
			next := m.next[state*m.nsym+sym]
			assert.GreaterOrEqualf(t, next, int32(0),
				"state %d symbol %d has no successor", state, sym)
			assert.Lessf(t, int(next), len(m.terminal),
				"state %d symbol %d leaves the table", state, sym)
		}
	}

	// A suffix match must be reachable without an output chain: "bc"
	// is found even though the walk entered through "ab".
	assert.True(t, m.matches("abc"))
}

// TestRule_CheckNodeConsultsMatcher pins that the single-pass gate is
// actually wired into CheckNode, deterministically and without timing.
//
// The benchmark's budget only runs under -bench, which CI does not do
// for this package, so it cannot catch the gate being removed. This
// test can: it installs an automaton compiled from a list that does
// NOT include the configured needle, so the gate reports "no match"
// while the per-needle loop would report one. Diagnostics appear only
// if the gate was skipped.
func TestRule_CheckNodeConsultsMatcher(t *testing.T) {
	const src = "# Doc\n\nThis paragraph mentions delve once.\n"

	t.Run("gate short-circuits the per-needle loop", func(t *testing.T) {
		r := &Rule{Contains: []string{"delve"}, ac: newMatcher([]string{"zzz"})}
		assert.Nil(t, r.Check(mustFile(t, src)),
			"CheckNode ignored the matcher and ran the per-needle loop")
	})

	t.Run("gate passes the paragraph through on a match", func(t *testing.T) {
		r := &Rule{}
		require.NoError(t, r.ApplySettings(map[string]any{
			"contains": []string{"delve"},
		}))
		require.NotNil(t, r.ac, "ApplySettings must compile the matcher")

		diags := r.Check(mustFile(t, src))
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "delve")
	})

	t.Run("an uncompiled rule still checks via the loop", func(t *testing.T) {
		// Contains assigned directly, so ac is nil: the rule must fall
		// back to the per-needle loop rather than silently pass.
		r := &Rule{Contains: []string{"delve"}}
		require.Nil(t, r.ac)

		diags := r.Check(mustFile(t, src))
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "delve")
	})
}
