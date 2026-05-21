//go:build race

package integration

// raceEnabled is the build-tag sentinel for `-race`. See the
// race_off_test.go variant for the rationale; this file is selected
// when the race detector is active, so the per-rule alloc gate
// skips instead of fighting the detector's allocation bookkeeping.
const raceEnabled = true
