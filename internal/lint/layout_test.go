package lint

import (
	"testing"
	"unsafe"
)

// TestStructLayout asserts the optimal size for BlockSpan. BlockSpan is
// built once per block by Layer0 and read by rule.WalkBlocks across every
// default rule that walks the parse-skip path (MDS002, MDS010, MDS011,
// MDS013, MDS015, MDS031, MDS065, MDS066), so its per-instance size
// multiplies by block count and rule count. Ordering fields large-to-small
// (the three ints, then the two byte-sized fields together) packs it into
// 32 bytes; interleaving a byte field between the ints pads it to 40
// (docs/development/high-performance-go.md#struct-layout).
func TestStructLayout(t *testing.T) {
	got := unsafe.Sizeof(BlockSpan{})
	const want = uintptr(32)
	if got != want {
		t.Errorf("unsafe.Sizeof(BlockSpan{}) = %d; want %d (reorder fields to eliminate padding)",
			got, want)
	}
}
