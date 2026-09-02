package samefileanchor

import (
	"testing"
	"unsafe"
)

// TestAnchorCheckerFieldOrder asserts that built (a bool, no pointers)
// is the last field in anchorChecker, after the pointer-containing r,
// f, slugs, and diags fields. Go's GC ptrdata for a struct spans from
// offset 0 through the end of the last pointer-containing field, so a
// scalar declared before diags (the last pointer field) costs GC scan
// bytes for nothing. See docs/development/high-performance-go.md
// "Struct layout". The test fails (red) until built is moved after
// diags.
func TestAnchorCheckerFieldOrder(t *testing.T) {
	got := unsafe.Offsetof(anchorChecker{}.built)
	const want = uintptr(48) // after r, f, slugs (8 bytes each) and diags (24 bytes)
	if got != want {
		t.Errorf("unsafe.Offsetof(anchorChecker.built) = %d; want %d (move built after diags to minimise GC scan)",
			got, want)
	}
}
