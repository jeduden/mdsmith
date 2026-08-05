//go:build !race

package checker_test

// raceEnabled is the build-tag sentinel for `-race`. Under the default
// build (no race), it is false. The race_on_test.go variant flips it
// under `-race`. Allocation-budget tests key off this constant to skip
// when the race detector is instrumenting allocations: the detector's
// bookkeeping adds enough extra allocations to make the per-op count
// flaky at the edge of a tight budget, and the budget is for
// production behaviour, not race-instrumented test runs.
const raceEnabled = false
