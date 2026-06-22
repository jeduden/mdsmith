package crossfilereferenceintegrity

import (
	"testing"
	"unsafe"
)

// TestStructLayout asserts the optimal size for the Rule struct.
// The Rule struct currently packs bool fields between larger fields, wasting
// padding bytes. Reordering to place the bools together reduces size from
// 128 to 120 bytes and improves cache utilisation across per-Check calls.
func TestStructLayout(t *testing.T) {
	got := unsafe.Sizeof(Rule{})
	const want = uintptr(120)
	if got != want {
		t.Errorf("unsafe.Sizeof(Rule{}) = %d; want %d (reorder fields to eliminate padding)",
			got, want)
	}
}
