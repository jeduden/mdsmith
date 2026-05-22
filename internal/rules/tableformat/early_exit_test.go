package tableformat

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// TestCheck_EarlyExitOnNoPipe_AllocsZero pins the plan-195
// optimisation: on a file that contains no `|` byte, Check
// returns before paying `lint.CollectCodeBlockLines` (an AST
// walk + map alloc) and tablefmt.Violations' inner setup. A
// plain "diags == nil" assertion didn't pin this — both the
// early-exit and the fall-through path return nil when no
// tables are found — so a rollback that removes the guard
// would have passed silently. Measuring on a cold File
// (fresh per iteration, parse baseline subtracted) reveals
// the AST walk's allocation footprint: with the guard the
// delta is 0; without it the delta is several allocs from
// the code-block walk and the Violations setup.
func TestCheck_EarlyExitOnNoPipe_AllocsZero(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	if raceEnabled {
		// The race detector's allocation bookkeeping perturbs
		// AllocsPerRun counts; a strict delta == 0 assertion
		// flakes under `go test -race ./...`. The optimisation
		// is for production builds, so skipping race builds
		// keeps the gate stable. Same pattern as the
		// alloc-budget tests in this package.
		t.Skip("alloc gate skipped under -race")
	}
	src := []byte("# Title\n\nProse without tables.\n")
	r := &Rule{}
	const runs = 100
	parse := testing.AllocsPerRun(runs, func() {
		_, err := lint.NewFile("p.md", src)
		require.NoError(t, err)
	})
	full := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("c.md", src)
		require.NoError(t, err)
		_ = r.Check(f)
	})
	delta := full - parse
	if delta < 0 {
		// Clamp small negative deltas to 0 — AllocsPerRun
		// averages across `runs` iterations and can report a
		// slightly-negative delta when the parse and check
		// passes are measured against different GC states.
		delta = 0
	}
	require.Zero(t, delta,
		"Check on a no-pipe file should hit the bytes.IndexByte "+
			"early-exit and add 0 allocs over the parse baseline; "+
			"got %.1f/op (full=%.1f, parse=%.1f)", delta, full, parse)
}
