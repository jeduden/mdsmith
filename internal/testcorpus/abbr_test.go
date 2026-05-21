package testcorpus

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAbbrHeavy_NonEmpty pins the corpus invariant the rest of the
// suite assumes — at least one paragraph and every entry non-empty.
// A truncated or empty corpus would silently weaken benchmarks that
// run "for each paragraph" because the loop body would never fire.
func TestAbbrHeavy_NonEmpty(t *testing.T) {
	got := AbbrHeavy()
	require.NotEmpty(t, got)
	for i, s := range got {
		assert.NotEmptyf(t, strings.TrimSpace(s),
			"AbbrHeavy()[%d] must not be empty", i)
	}
}

// TestAbbrHeavy_ReturnsFreshCopy pins the immutability contract.
// AbbrHeavy must hand out a fresh slice on every call so a caller
// that mutates its result cannot affect any other consumer running
// in parallel.
func TestAbbrHeavy_ReturnsFreshCopy(t *testing.T) {
	a := AbbrHeavy()
	b := AbbrHeavy()
	require.NotEmpty(t, a)
	// Different slice headers — different backing arrays. Mutating
	// `a` must not affect `b`.
	a[0] = "MUTATED"
	assert.NotEqual(t, "MUTATED", b[0],
		"two AbbrHeavy() calls must return independent slices")
	// And the canonical fixture must be unchanged on the next call.
	assert.NotEqual(t, "MUTATED", AbbrHeavy()[0],
		"a caller's mutation must not leak into the package-level corpus")
}

// TestAbbrHeavyParagraph_JoinsCorpus pins the join contract: the
// returned paragraph contains every entry of AbbrHeavy in order, with
// a single space between entries. A regression here would change what
// MDS024's BenchmarkRule_MDS024 actually measures, so this anchors
// the fixture even though the function body is trivial.
func TestAbbrHeavyParagraph_JoinsCorpus(t *testing.T) {
	got := AbbrHeavyParagraph()
	entries := AbbrHeavy()
	for _, s := range entries {
		assert.Containsf(t, got, s,
			"AbbrHeavyParagraph must include corpus entry %q", s)
	}
	// Joined with a single space — not a newline (paragraph stays
	// a single paragraph) and not multiple spaces.
	want := strings.Join(entries, " ")
	assert.Equal(t, want, got,
		"AbbrHeavyParagraph must join entries with a single space")
}

// TestJoinWithSpace_EmptyCorpus is the explicit zero case: an empty
// slice must produce an empty paragraph.
func TestJoinWithSpace_EmptyCorpus(t *testing.T) {
	assert.Equal(t, "", joinWithSpace(nil),
		"nil input must produce an empty paragraph")
	assert.Equal(t, "", joinWithSpace([]string{}),
		"empty slice must produce an empty paragraph")
}

// TestJoinWithSpace_SingleEntry pins the single-entry branch: no
// leading or trailing space.
func TestJoinWithSpace_SingleEntry(t *testing.T) {
	assert.Equal(t, "lonely",
		joinWithSpace([]string{"lonely"}),
		"single-entry slice must not add a separator")
}
