//go:build !race

package linelength

// raceEnabled is the build-tag sentinel for `-race`. See the
// integration package's race_off_test.go for the rationale; the
// alloc-budget gate skips when the race detector is on because
// its bookkeeping adds non-deterministic allocations.
const raceEnabled = false
