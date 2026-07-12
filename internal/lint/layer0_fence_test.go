package lint

import (
	"testing"
	"unsafe"
)

// TestFenceInfo_FieldOrderMinimizesPadding pins fenceInfo's memory
// layout at 24 bytes. openingFence returns fenceInfo by value on the
// hottest per-line scan path (tryFence, tried first for every
// non-blank line of every file), so struct padding is copied
// repeatedly. Ordering fields large-to-small (both ints, then the two
// single-byte fields) avoids the 8 bytes of padding a byte-then-int
// layout costs. See docs/development/high-performance-go.md
// "Struct layout" / "Order fields large-to-small".
func TestFenceInfo_FieldOrderMinimizesPadding(t *testing.T) {
	got := unsafe.Sizeof(fenceInfo{})
	if got > 24 {
		t.Fatalf("unsafe.Sizeof(fenceInfo{}) = %d, want <= 24 (large-to-small field order); "+
			"see docs/development/high-performance-go.md", got)
	}
}
