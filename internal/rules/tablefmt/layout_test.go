package tablefmt

import (
	"testing"
)

// TestStripPrefixNoAlloc asserts that stripPrefix performs zero heap
// allocations when given a non-empty prefix. The previous implementation
// allocated string(line) and []byte(…) — two allocs per table row.
func TestStripPrefixNoAlloc(t *testing.T) {
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	line := []byte("> | col | other |")
	prefix := "> "
	var sink []byte
	allocs := testing.AllocsPerRun(100, func() {
		sink = stripPrefix(line, prefix)
	})
	_ = sink
	if allocs != 0 {
		t.Errorf("stripPrefix allocated %.0f times; want 0", allocs)
	}
}
