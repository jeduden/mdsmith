package uniquefrontmatter

import (
	"testing"
	"unsafe"
)

// TestPathEntryFieldOrder asserts that line (an int, no pointers) is
// the last field in pathEntry, after the pointer-containing value and
// firstPath strings. Go's GC ptrdata for a struct spans from offset 0
// through the end of the last pointer-containing field, so a scalar
// declared between two string fields costs GC scan bytes for nothing.
// pathEntry is stored once per violation in scopeIndex.byPath, an
// index built across the whole workspace, so the saving scales with
// corpus size. See docs/development/high-performance-go.md "Struct
// layout". The test fails (red) until line is moved after firstPath.
func TestPathEntryFieldOrder(t *testing.T) {
	got := unsafe.Offsetof(pathEntry{}.line)
	const want = uintptr(32) // after value and firstPath (16 bytes each)
	if got != want {
		t.Errorf("unsafe.Offsetof(pathEntry.line) = %d; want %d (move line after firstPath to minimise GC scan)",
			got, want)
	}
}
