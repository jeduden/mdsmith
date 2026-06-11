package arena

import (
	"testing"
)

// TestResetReusesAllSlabsNotJustLast pins the cursor behaviour the
// arena pool depends on: filling more than one slab, Resetting, and
// filling again must reuse the existing slabs from the first one
// instead of growing the slab list run after run.
func TestResetReusesAllSlabsNotJustLast(t *testing.T) {
	a := New()
	// Fill past one slab so the arena holds at least two text slabs.
	for i := 0; i < textSlabCap+1; i++ {
		a.Text()
	}
	slabsAfterFill := len(a.texts)
	if slabsAfterFill < 2 {
		t.Fatalf("expected at least 2 slabs after overfill, got %d", slabsAfterFill)
	}
	for cycle := 0; cycle < 3; cycle++ {
		a.Reset()
		for i := 0; i < textSlabCap+1; i++ {
			a.Text()
		}
		if len(a.texts) != slabsAfterFill {
			t.Fatalf("cycle %d: slab count grew from %d to %d; Reset must reuse earlier slabs",
				cycle, slabsAfterFill, len(a.texts))
		}
	}
}

// TestResetReusesSegmentSlabs is the segment-backing variant of the
// slab-reuse gate.
func TestResetReusesSegmentSlabs(t *testing.T) {
	a := New()
	for i := 0; i < 3*segmentSlabCap/initialSegmentCap; i++ {
		a.allocSegmentBacking(initialSegmentCap)
	}
	slabsAfterFill := len(a.segments)
	if slabsAfterFill < 2 {
		t.Fatalf("expected at least 2 segment slabs, got %d", slabsAfterFill)
	}
	for cycle := 0; cycle < 3; cycle++ {
		a.Reset()
		for i := 0; i < 3*segmentSlabCap/initialSegmentCap; i++ {
			a.allocSegmentBacking(initialSegmentCap)
		}
		if len(a.segments) != slabsAfterFill {
			t.Fatalf("cycle %d: segment slab count grew from %d to %d",
				cycle, slabsAfterFill, len(a.segments))
		}
	}
}
