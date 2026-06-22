package tableformat

import (
	"testing"
	"unsafe"
)

// TestRuleFieldOrder asserts that the Style string field comes first in the
// MDS025 Rule struct so that the GC pointer-scan span is minimised to 8 bytes
// (just the string data pointer at offset 0) rather than 24 bytes (when Style
// sat after two int fields). The test fails (red) until fields are reordered.
func TestRuleFieldOrder(t *testing.T) {
	got := unsafe.Offsetof(Rule{}.Style)
	const want = uintptr(0)
	if got != want {
		t.Errorf("unsafe.Offsetof(Rule.Style) = %d; want %d (move Style first to minimise GC scan)",
			got, want)
	}
}
