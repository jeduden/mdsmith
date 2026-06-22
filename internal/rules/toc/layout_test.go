package toc

import (
	"testing"
	"unsafe"
)

// TestRuleFieldOrder asserts that the engine pointer field comes first in the
// MDS038 Rule struct. The current layout puts engine after engineOnce (a
// sync.Once = 16 bytes with no pointers), forcing GC to scan 24 bytes.
// Moving engine first reduces the GC pointer-scan span to 8 bytes.
// The test fails (red) until fields are reordered.
func TestRuleFieldOrder(t *testing.T) {
	got := unsafe.Offsetof(Rule{}.engine)
	const want = uintptr(0)
	if got != want {
		t.Errorf("unsafe.Offsetof(Rule.engine) = %d; want %d (move engine first to minimise GC scan)",
			got, want)
	}
}
