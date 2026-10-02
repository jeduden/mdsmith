package metrics

import "testing"

// skipAllocGate skips an AllocsPerRun gate under -short and -race.
func skipAllocGate(t *testing.T) {
	t.Helper()
	if testing.Short() || raceEnabled {
		t.Skip("alloc gate skipped under -short and -race")
	}
}
