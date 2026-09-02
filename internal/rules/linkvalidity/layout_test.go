package linkvalidity

import (
	"testing"
	"unsafe"
)

// TestRevMatchFieldOrder asserts that the pointer-containing text and
// url byte slices precede the scalar col0 and matchEnd ints in
// revMatch. Go's GC ptrdata for a struct spans from offset 0 through
// the end of the last pointer-containing field; with the scalars
// first (the prior order), ptrdata had to cover both slices anyway,
// so grouping the slices first instead shrinks ptrdata from 48 bytes
// to 32. See docs/development/high-performance-go.md "Struct
// layout". The test fails (red) until col0 and matchEnd are moved
// after text and url.
func TestRevMatchFieldOrder(t *testing.T) {
	got := unsafe.Offsetof(revMatch{}.col0)
	const want = uintptr(48) // after text and url (24 bytes each)
	if got != want {
		t.Errorf("unsafe.Offsetof(revMatch.col0) = %d; want %d (move col0/matchEnd after text/url to minimise GC scan)",
			got, want)
	}
}
