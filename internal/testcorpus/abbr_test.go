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
	require.NotEmpty(t, AbbrHeavy)
	for i, s := range AbbrHeavy {
		assert.NotEmptyf(t, strings.TrimSpace(s),
			"AbbrHeavy[%d] must not be empty", i)
	}
}

// TestAbbrHeavyParagraph_JoinsCorpus pins the join contract: the
// returned paragraph contains every entry of AbbrHeavy in order, with
// a single space between entries. A regression here would change what
// MDS024's BenchmarkRule_MDS024 actually measures, so this anchors
// the fixture even though the function body is trivial.
func TestAbbrHeavyParagraph_JoinsCorpus(t *testing.T) {
	got := AbbrHeavyParagraph()
	for _, s := range AbbrHeavy {
		assert.Containsf(t, got, s,
			"AbbrHeavyParagraph must include corpus entry %q", s)
	}
	// Joined with a single space — not a newline (paragraph stays
	// a single paragraph) and not multiple spaces.
	want := strings.Join(AbbrHeavy, " ")
	assert.Equal(t, want, got,
		"AbbrHeavyParagraph must join entries with a single space")
}

// TestAbbrHeavyParagraph_EmptyCorpus is the explicit zero case: if
// AbbrHeavy were ever emptied (someone removes every entry), the
// joined paragraph must be empty too. Drives the corresponding
// branch in AbbrHeavyParagraph red/green.
func TestAbbrHeavyParagraph_EmptyCorpus(t *testing.T) {
	saved := AbbrHeavy
	t.Cleanup(func() { AbbrHeavy = saved })
	AbbrHeavy = nil
	assert.Equal(t, "", AbbrHeavyParagraph(),
		"empty corpus must produce an empty paragraph")
}
