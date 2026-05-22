package linelength

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// TestApplyStrict_NonBool covers the type-mismatch error path in
// applyStrict. The setting accepts only a bool; the negative path
// returned an unexercised error before this test.
func TestApplyStrict_NonBool(t *testing.T) {
	r := &Rule{}
	err := r.applyStrict("not a bool")
	require.Error(t, err)
	require.Contains(t, err.Error(), "strict must be a bool")
}

// TestCheck_BaseMaxFallback covers the defensive `if baseMax <= 0`
// branch in Check that resets to the documented default (80).
// A non-positive Max is invalid config but the runtime falls back
// silently rather than panicking; pinning the behaviour anchors
// the contract.
func TestCheck_BaseMaxFallback(t *testing.T) {
	// Max = 0 ⇒ baseMax = 80. A line ≤ 80 chars passes; a 81-char
	// line trips.
	src := []byte(nChars(81, 'a') + "\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Max: 0, Exclude: defaultExclude()}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic on the long line")
}

// TestCheck_HeadingMaxForATXAndSetext covers the headingLineNum
// branches plus collectHeadingLines via a HeadingMax setting. The
// ATX-no-Lines() fallback path is exercised by the inline child
// text node walk inside headingLineNum.
func TestCheck_HeadingMaxForATXAndSetext(t *testing.T) {
	hm := 10
	src := []byte(
		"# Long ATX heading title beyond the limit\n" +
			"\n" +
			"Setext heading title beyond the limit\n" +
			"=====================================\n",
	)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Max: 80, HeadingMax: &hm, Exclude: defaultExclude()}
	diags := r.Check(f)
	// Both heading lines exceed the heading-max of 10.
	require.GreaterOrEqual(t, len(diags), 1)
}
