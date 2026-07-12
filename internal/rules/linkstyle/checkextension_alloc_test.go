package linkstyle

import "testing"

// TestCheckExtension_UpperCaseZeroAllocs guards against strings.ToLower
// allocating in checkExtension, which runs once per link/image target in
// every checked file for MDS022 — one of the hottest per-node paths in the
// rule set (docs/development/high-performance-go.md#strings-and-bytes).
// strings.ToLower allocates a new string whenever the input contains an
// upper-case byte; strings.EqualFold compares without allocating.
func TestCheckExtension_UpperCaseZeroAllocs(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	allocs := testing.AllocsPerRun(100, func() {
		_ = checkExtension("keep", "docs/README.MD")
	})
	if allocs > 0 {
		t.Fatalf("checkExtension: expected 0 allocs/op for upper-case extension, got %.0f", allocs)
	}
}
