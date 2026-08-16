//go:build race

package occurrence

// raceEnabled is the build-tag sentinel for `-race`. See the
// race_off_test.go variant for the rationale; this file is selected
// when the race detector is active, so
// TestCheck_PatternOnly_AllocBudget_* skips instead of fighting the
// detector's allocation bookkeeping.
const raceEnabled = true
