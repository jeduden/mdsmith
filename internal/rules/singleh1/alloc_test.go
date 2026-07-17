package singleh1

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// TestFix_RepsPreSizedForManyH1s pins Fix's allocation cost on a document
// with many extra H1s. len(toDemote) is known before the reps loop starts
// (each iteration appends at most one entry), so sizing reps from it up
// front avoids append's grow+copy (nil→1→2→4→8→16→32...) that a document
// with many extra H1s used to pay — one regrowth allocation per doubling.
func TestFix_RepsPreSizedForManyH1s(t *testing.T) {
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}

	var sb strings.Builder
	sb.WriteString("# Title\n\n")
	for i := 0; i < 19; i++ {
		sb.WriteString("# Extra\n\n")
	}
	src := []byte(sb.String())
	r := &Rule{}

	warm, err := lint.NewFile("warm.md", src)
	require.NoError(t, err)
	_ = r.Fix(warm)

	const runs = 50
	parse := testing.AllocsPerRun(runs, func() {
		_, err := lint.NewFile("parse.md", src)
		require.NoError(t, err)
	})
	full := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("fix.md", src)
		require.NoError(t, err)
		_ = r.Fix(f)
	})
	fixAllocs := full - parse
	if fixAllocs < 0 {
		fixAllocs = 0
	}
	t.Logf("Fix (19 extra H1s) allocs/op = %.0f", fixAllocs)
	// Measured floor after pre-sizing is 44 for this 19-extra-H1 fixture;
	// the old nil-slice append regrowth (nil→1→2→4→8→16→32) cost 48.
	require.LessOrEqualf(t, fixAllocs, 46.0,
		"Fix allocs/op = %.0f for 19 extra H1s; want reps pre-sized to len(toDemote) instead of growing from nil",
		fixAllocs)
}
