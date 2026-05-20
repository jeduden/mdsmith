//go:build !mdtext_punkt_upstream && !race

package paragraphstructure

// raceEnabled is the build-tag sentinel for `-race`. Under the
// default build (no race), it is false; the race_on_test.go variant
// flips it. The alloc-budget gate keys off this constant to skip
// when the race detector is instrumenting allocations — the
// detector's bookkeeping adds enough extra allocations to make the
// per-op count flaky on the edge of the ≤ 10 budget, and the budget
// is for production behaviour, not race-instrumented test runs.
const raceEnabled = false
