package lint

import (
	"testing"
	"unsafe"
)

// blockSpanSizeBudget pins BlockSpan's struct size. BlockSpan is built once
// per block by Layer0 and read by rule.WalkBlocks across 8+ default rules
// on the parse-skip path — fields ordered large-to-small (three ints, then
// the two byte-sized fields together) pack into 32 bytes; interleaving a
// byte field between the ints pads the struct to 40
// (docs/development/high-performance-go.md#struct-layout).
const blockSpanSizeBudget = 32

func TestBlockSpan_SizeBudget(t *testing.T) {
	got := unsafe.Sizeof(BlockSpan{})
	if got > blockSpanSizeBudget {
		t.Fatalf("unsafe.Sizeof(BlockSpan{}) = %d, budget = %d (field order wastes padding)",
			got, blockSpanSizeBudget)
	}
}
