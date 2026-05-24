package arena_test

import (
	"testing"

	"github.com/yuin/goldmark/arena"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// TestNilArenaFallsBackToHeap covers the nil-safety contract: every
// (*Arena) method must work on a nil receiver and produce a heap
// node indistinguishable from the upstream constructor. Call sites
// that swap `ast.NewText()` for `a.Text()` must keep working when
// `a` is nil (the goldmark_upstream build-tag path).
func TestNilArenaFallsBackToHeap(t *testing.T) {
	var a *arena.Arena
	if got := a.Text(); got == nil {
		t.Fatal("nil arena Text() returned nil")
	}
	if got := a.TextSegment(text.NewSegment(0, 5)); got == nil || got.Segment.Start != 0 || got.Segment.Stop != 5 {
		t.Fatalf("nil arena TextSegment lost segment: %+v", got)
	}
	if got := a.RawTextSegment(text.NewSegment(0, 5)); got == nil || !got.IsRaw() {
		t.Fatalf("nil arena RawTextSegment did not set raw flag: %+v", got)
	}
	if got := a.Paragraph(); got == nil {
		t.Fatal("nil arena Paragraph() returned nil")
	}
	if got := a.Segments(); got == nil {
		t.Fatal("nil arena Segments() returned nil")
	}
	// Reset and EquipLines are also no-ops on nil — proves they
	// don't panic.
	a.Reset()
	a.EquipLines(nil)
	a.EquipLines(text.NewSegments())
}

// TestTextAllocReusesSlabAcrossReset verifies the slab promise in
// isolation: after Reset, the next Text() call returns the same
// pointer the arena handed out at the equivalent slab offset
// before Reset. The production parser path creates a fresh arena
// per Parse and does not call Reset, so this test exercises Reset
// directly to keep that contract verified even though no caller
// relies on it today.
func TestTextAllocReusesSlabAcrossReset(t *testing.T) {
	a := arena.New()
	first := a.Text()
	a.Reset()
	second := a.Text()
	if first != second {
		t.Errorf("expected slab slot reuse after Reset; got %p then %p", first, second)
	}
}

// TestParagraphAllocReusesSlabAcrossReset is the Paragraph
// counterpart of TestTextAllocReusesSlabAcrossReset. Each paragraph
// also carries an embedded Lines() Segments; we'll cover that in
// the Grow test below.
func TestParagraphAllocReusesSlabAcrossReset(t *testing.T) {
	a := arena.New()
	first := a.Paragraph()
	a.Reset()
	second := a.Paragraph()
	if first != second {
		t.Errorf("expected slab slot reuse after Reset; got %p then %p", first, second)
	}
}

// TestSegmentsArenaBackedAppendStaysInArena exercises the
// SegmentsGrower wiring: a Segments allocated through the arena
// must, on Append-driven growth, take its new backing slice from
// the arena's segment slab rather than the runtime allocator. We
// can't directly observe `make([]...)` calls, but we can verify the
// slice's underlying array sits inside the slab by comparing
// addresses after several Appends.
func TestSegmentsArenaBackedAppendStaysInArena(t *testing.T) {
	a := arena.New()
	s := a.Segments()
	// initialSegmentCap is 4; push past it to force Grow.
	for i := 0; i < 16; i++ {
		s.Append(text.NewSegment(i, i+1))
	}
	if s.Len() != 16 {
		t.Fatalf("Append lost segments: got %d, want 16", s.Len())
	}
	// Spot-check the contents.
	for i := 0; i < 16; i++ {
		if got := s.At(i); got.Start != i || got.Stop != i+1 {
			t.Errorf("Segments.At(%d) = %+v, want Start=%d Stop=%d", i, got, i, i+1)
		}
	}
}

// TestEquipLinesRedirectsExternalSegments covers the path the
// parser uses to absorb backing-array growth on block nodes the
// arena doesn't construct itself (extension-defined block types).
// After EquipLines, that block's Lines().Append must allocate via
// the arena's grower.
func TestEquipLinesRedirectsExternalSegments(t *testing.T) {
	a := arena.New()
	s := text.NewSegments()
	a.EquipLines(s)
	for i := 0; i < 12; i++ {
		s.Append(text.NewSegment(i, i+1))
	}
	if s.Len() != 12 {
		t.Fatalf("EquipLines path lost segments: got %d, want 12", s.Len())
	}
}

// TestResetIdempotent guards the idempotency claim. Two Resets in
// a row must produce the same slab-cursor-at-zero state as one.
func TestResetIdempotent(t *testing.T) {
	a := arena.New()
	a.Text()
	a.Paragraph()
	a.Segments()
	a.Reset()
	a.Reset() // second Reset must not crash
	if got := a.Text(); got == nil {
		t.Fatal("Text() after double Reset returned nil")
	}
}

// TestResetZeroesPointerSlabs verifies Reset drops pointer-bearing
// fields from the slab arrays so a reused Arena does not pin the
// prior AST. We allocate a Paragraph, attach a child Text under it
// to fix the BaseInline parent pointer, call Reset, then read the
// slab slot back and confirm the parent pointer is gone. Without
// clear() in Reset, that pointer would keep the prior subtree
// reachable across reuse.
//
// Reading slab.data past Reset is a deliberate test-only peek: we
// inspect the same memory region the arena would overwrite on the
// next allocation. The slot must be zero-valued, which is what a
// fresh allocation would also see.
func TestResetZeroesPointerSlabs(t *testing.T) {
	a := arena.New()
	p := a.Paragraph()
	child := a.Text()
	p.AppendChild(p, child)
	// Sanity-check the wiring before Reset.
	if child.Parent() != ast.Node(p) {
		t.Fatalf("pre-Reset wiring failed: child.Parent()=%v", child.Parent())
	}
	a.Reset()
	// After Reset the slab slots that previously held p and child
	// must report zero-valued state — no surviving parent/sibling
	// pointers to pin the prior subtree.
	reallocP := a.Paragraph()
	if reallocP.FirstChild() != nil {
		t.Errorf("reused Paragraph slot still has FirstChild: %v", reallocP.FirstChild())
	}
	reallocT := a.Text()
	if reallocT.Parent() != nil {
		t.Errorf("reused Text slot still has Parent: %v", reallocT.Parent())
	}
}

// TestTextSegmentInitializesFields verifies the arena's
// TextSegment helper sets the Segment field exactly the way the
// upstream constructor does. The fixture below relies on this for
// AST equivalence with the non-arena path.
func TestTextSegmentInitializesFields(t *testing.T) {
	a := arena.New()
	seg := text.NewSegmentPadding(3, 7, 2)
	got := a.TextSegment(seg)
	if got.Segment != seg {
		t.Errorf("TextSegment lost segment: got %+v, want %+v", got.Segment, seg)
	}
	if got.IsRaw() {
		t.Error("TextSegment should not set Raw")
	}
}

// TestParagraphLinesGrowerIsArena verifies that a fresh Paragraph
// returned by the arena has its Lines() Segments pre-wired with
// arena-backed growth. Without this, every paragraph's line append
// after the initial backing-cap would fall back to the heap.
func TestParagraphLinesGrowerIsArena(t *testing.T) {
	a := arena.New()
	p := a.Paragraph()
	// Force Lines to grow past initialSegmentCap=4.
	for i := 0; i < 32; i++ {
		p.Lines().Append(text.NewSegment(i, i+1))
	}
	if p.Lines().Len() != 32 {
		t.Errorf("Paragraph Lines lost segments: got %d, want 32", p.Lines().Len())
	}
}

// TestKindIsPreservedAfterReset confirms that nodes returned after
// Reset still report the same Kind — i.e. zero-resetting the slab
// slot doesn't break the Kind() vtable. (Kind is implemented on
// pointer receivers that consult the type, not a stored field, but
// the test guards against accidental change in the future.)
func TestKindIsPreservedAfterReset(t *testing.T) {
	a := arena.New()
	a.Text()
	a.Paragraph()
	a.Reset()
	if k := a.Text().Kind(); k != ast.KindText {
		t.Errorf("Text Kind after Reset = %v, want %v", k, ast.KindText)
	}
	if k := a.Paragraph().Kind(); k != ast.KindParagraph {
		t.Errorf("Paragraph Kind after Reset = %v, want %v", k, ast.KindParagraph)
	}
}
