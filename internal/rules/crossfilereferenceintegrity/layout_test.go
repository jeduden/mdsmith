package crossfilereferenceintegrity

import (
	"testing"
	"unsafe"
)

// TestStructLayout asserts the optimal size for the Rule struct.
// Moving bool fields to the end (previously between larger fields, wasting
// padding bytes) reduces size from 128 to 120 bytes and improves cache
// utilisation across per-Check calls.
func TestStructLayout(t *testing.T) {
	got := unsafe.Sizeof(Rule{})
	const want = uintptr(120)
	if got != want {
		t.Errorf("unsafe.Sizeof(Rule{}) = %d; want %d (reorder fields to eliminate padding)",
			got, want)
	}
}
