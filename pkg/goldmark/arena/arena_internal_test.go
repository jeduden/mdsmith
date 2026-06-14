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
	slabsAfterFill := len(a.texts.list)
	if slabsAfterFill < 2 {
		t.Fatalf("expected at least 2 slabs after overfill, got %d", slabsAfterFill)
	}
	for cycle := 0; cycle < 3; cycle++ {
		a.Reset()
		for i := 0; i < textSlabCap+1; i++ {
			a.Text()
		}
		if len(a.texts.list) != slabsAfterFill {
			t.Fatalf("cycle %d: slab count grew from %d to %d; Reset must reuse earlier slabs",
				cycle, slabsAfterFill, len(a.texts.list))
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
	cs, ln, em := len(a.codeSpans.list), len(a.links.list), len(a.emphases.list)
	if cs < 2 || ln < 2 || em < 2 {
		t.Fatalf("expected at least 2 slabs each, got %d %d %d", cs, ln, em)
	}
	for cycle := 0; cycle < 3; cycle++ {
		a.Reset()
		fill()
		if len(a.codeSpans.list) != cs || len(a.links.list) != ln || len(a.emphases.list) != em {
			t.Fatalf("cycle %d: slab counts grew: %d %d %d",
				cycle, len(a.codeSpans.list), len(a.links.list), len(a.emphases.list))
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

// TestStructuralSlabsAdvanceCursors overfills the paragraph and
// segments-object slabs so their cursor-advance paths run, then
// verifies Reset reuses them like the other slab types.
func TestStructuralSlabsAdvanceCursors(t *testing.T) {
	a := New()
	for i := 0; i < paragraphSlabCap+1; i++ {
		a.Paragraph()
	}
	for i := 0; i < segmentsObjSlabCap+1; i++ {
		a.Segments()
	}
	ps, ss := len(a.paragraphs.list), len(a.segmentsObjs.list)
	if ps < 2 || ss < 2 {
		t.Fatalf("expected at least 2 slabs each, got %d %d", ps, ss)
	}
	a.Reset()
	for i := 0; i < paragraphSlabCap+1; i++ {
		a.Paragraph()
	}
	for i := 0; i < segmentsObjSlabCap+1; i++ {
		a.Segments()
	}
	if len(a.paragraphs.list) != ps || len(a.segmentsObjs.list) != ss {
		t.Fatalf("slab counts grew across Reset: %d %d", len(a.paragraphs.list), len(a.segmentsObjs.list))
	}
}

// TestStructuralHeadingListItemSlabs pins the Heading/ListItem arena
// constructors. The heading- and list-heavy neutral corpus (Rust Book
// + Reference) allocated every ast.Heading and ast.ListItem on the
// heap before this; routing them through the arena drops those from
// the per-file allocation count like the Text/Paragraph slabs already
// do. The constructors must match the upstream ones field-for-field,
// fall back to the heap on a nil receiver, count their allocations,
// and reuse their slabs across Reset.
func TestStructuralHeadingListItemSlabs(t *testing.T) {
	var nilA *Arena
	if nilA.Heading(2) == nil || nilA.ListItem(3) == nil {
		t.Fatal("nil arena constructors must fall back to heap nodes")
	}
	if nilA.HeadingsAllocated() != 0 || nilA.ListItemsAllocated() != 0 {
		t.Fatal("nil arena must report zero")
	}

	a := New()
	if got, want := a.Heading(3).Level, ast.NewHeading(3).Level; got != want {
		t.Errorf("Heading level = %d, want %d", got, want)
	}
	if got, want := a.ListItem(5).Offset, ast.NewListItem(5).Offset; got != want {
		t.Errorf("ListItem offset = %d, want %d", got, want)
	}
	if got, want := a.Heading(1).Kind(), ast.NewHeading(1).Kind(); got != want {
		t.Errorf("Heading kind = %v, want %v", got, want)
	}
	if got, want := a.ListItem(1).Kind(), ast.NewListItem(1).Kind(); got != want {
		t.Errorf("ListItem kind = %v, want %v", got, want)
	}

	// Overfill both slabs so the cursor-advance path runs, then verify
	// Reset rewinds the counts and reuses the slabs run after run.
	fill := func() {
		for i := 0; i < headingSlabCap+1; i++ {
			a.Heading(1)
		}
		for i := 0; i < listItemSlabCap+1; i++ {
			a.ListItem(0)
		}
	}
	a.Reset()
	fill()
	hs, ls := len(a.headings.list), len(a.listItems.list)
	if hs < 2 || ls < 2 {
		t.Fatalf("expected at least 2 slabs each, got %d %d", hs, ls)
	}
	for cycle := 0; cycle < 3; cycle++ {
		a.Reset()
		if a.HeadingsAllocated() != 0 || a.ListItemsAllocated() != 0 {
			t.Fatalf("cycle %d: Reset must rewind allocation counts to zero", cycle)
		}
		fill()
		if len(a.headings.list) != hs || len(a.listItems.list) != ls {
			t.Fatalf("cycle %d: slab counts grew: %d %d",
				cycle, len(a.headings.list), len(a.listItems.list))
		}
	}
}

// TestTextsAllocatedCounts pins the introspection helper both ways:
// nil arena reports zero, and counts track allocations across Reset.
func TestTextsAllocatedCounts(t *testing.T) {
	var nilA *Arena
	if nilA.TextsAllocated() != 0 {
		t.Fatal("nil arena must report zero")
	}
	a := New()
	a.Text()
	a.Text()
	if got := a.TextsAllocated(); got != 2 {
		t.Fatalf("TextsAllocated = %d, want 2", got)
	}
	a.Reset()
	if got := a.TextsAllocated(); got != 0 {
		t.Fatalf("after Reset = %d, want 0", got)
	}
}
