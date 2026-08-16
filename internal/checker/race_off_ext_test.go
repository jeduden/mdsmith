//go:build !race

package checker_test

// raceEnabled is the build-tag sentinel for `-race` in this package's
// external test binary. The in-package sentinel of the same name lives
// in race_off_test.go (package checker) and is not visible from
// checker_test, so the external allocation-budget tests need their own
// copy. Under the default build (no race) it is false; the
// race_on_ext_test.go variant flips it under `-race`.
// Allocation-budget tests key off this constant to skip when the race
// detector is instrumenting allocations: the detector's bookkeeping
// adds enough extra allocations to make the per-op count flaky at the
// edge of a tight budget, and the budget is for production behaviour,
// not race-instrumented test runs.
const raceEnabled = false
