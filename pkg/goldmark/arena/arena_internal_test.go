package arena

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
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

// TestInlineNodeSlabsReuseAcrossReset extends the cursor gate to the
// inline-node slabs (CodeSpan, Link, Emphasis).
func TestInlineNodeSlabsReuseAcrossReset(t *testing.T) {
	a := New()
	fill := func() {
		for i := 0; i < codeSpanSlabCap+1; i++ {
			a.CodeSpan()
		}
		for i := 0; i < linkSlabCap+1; i++ {
			a.Link()
		}
		for i := 0; i < emphasisSlabCap+1; i++ {
			a.Emphasis(1)
		}
	}
	fill()
	cs, ln, em := len(a.codeSpans), len(a.links), len(a.emphases)
	if cs < 2 || ln < 2 || em < 2 {
		t.Fatalf("expected at least 2 slabs each, got %d %d %d", cs, ln, em)
	}
	for cycle := 0; cycle < 3; cycle++ {
		a.Reset()
		fill()
		if len(a.codeSpans) != cs || len(a.links) != ln || len(a.emphases) != em {
			t.Fatalf("cycle %d: slab counts grew: %d %d %d",
				cycle, len(a.codeSpans), len(a.links), len(a.emphases))
		}
	}
}

// TestInlineNodeConstructorsMatchUpstream pins that the arena
// constructors produce nodes indistinguishable from the upstream
// ones, and that the nil receiver falls back to the heap.
func TestInlineNodeConstructorsMatchUpstream(t *testing.T) {
	a := New()
	if got, want := a.CodeSpan().Kind(), ast.NewCodeSpan().Kind(); got != want {
		t.Errorf("CodeSpan kind = %v, want %v", got, want)
	}
	if got, want := a.Link().Kind(), ast.NewLink().Kind(); got != want {
		t.Errorf("Link kind = %v, want %v", got, want)
	}
	if got, want := a.Emphasis(2).Level, ast.NewEmphasis(2).Level; got != want {
		t.Errorf("Emphasis level = %v, want %v", got, want)
	}
	var nilA *Arena
	if nilA.CodeSpan() == nil || nilA.Link() == nil || nilA.Emphasis(1) == nil {
		t.Errorf("nil arena constructors must fall back to heap nodes")
	}
}
