package tablefmt

import (
	"testing"
)

var sinkBytes []byte

// TestStripPrefixNoAlloc asserts that stripPrefix performs zero heap
// allocations when given a non-empty prefix. The previous implementation
// allocated string(line) and []byte(…) — two allocs per table row.
func TestStripPrefixNoAlloc(t *testing.T) {
	line := []byte("> | col | other |")
	prefix := "> "
	allocs := testing.AllocsPerRun(100, func() {
		sinkBytes = stripPrefix(line, prefix)
	})
	if allocs != 0 {
		t.Errorf("stripPrefix allocated %.0f times; want 0", allocs)
	}
}
