package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHeading_PlanEditsApplyThroughApplyEdits runs a real Heading Plan
// through ApplyEdits instead of only asserting on the Plan's shape.
// Only TestMove_* did this before the same-range conflict check
// landed in ApplyEdits — a Heading or LinkRef plan with two
// non-overlapping edits on one line never ran that gate, so a
// regression there could make `mdsmith rename` newly exit 2 on
// perfectly valid input.
func TestHeading_PlanEditsApplyThroughApplyEdits(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# Setup\n\nBody.\n",
		// Two anchor links to the renamed heading on the same line, at
		// different byte offsets — non-overlapping edits that a false
		// positive in ApplyEdits's conflict/overlap check would reject.
		"b.md": "See [a](a.md#setup) and [b](a.md#setup) too.\n",
	})
	src := ws.files["a.md"]
	changes, err := callHeading(ws, "a.md", "a.md", src, 1, "Setup", "Install")
	require.NoError(t, err)
	require.Len(t, changes["b.md"], 2, "sanity: two same-line anchor edits")

	out, err := ApplyEdits(ws.files["b.md"], changes["b.md"])
	require.NoError(t, err)
	assert.Equal(t, "See [a](a.md#install) and [b](a.md#install) too.\n", string(out))
}

// TestLinkRef_PlanEditsApplyThroughApplyEdits is LinkRef's counterpart
// to TestHeading_PlanEditsApplyThroughApplyEdits: two shortcut uses of
// the same label on one line, plus the ref-def on another, all run
// through the production ApplyEdits.
func TestLinkRef_PlanEditsApplyThroughApplyEdits(t *testing.T) {
	src := []byte("Use [spec] and [spec] again.\n\n[spec]: u\n")
	edits, err := callLinkRef(src, "spec", "rfc")
	require.NoError(t, err)
	require.Len(t, edits, 3, "sanity: two same-line uses plus the def")

	out, err := ApplyEdits(src, edits)
	require.NoError(t, err)
	assert.Equal(t, "Use [rfc] and [rfc] again.\n\n[rfc]: u\n", string(out))
}
