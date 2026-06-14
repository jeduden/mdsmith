// Package arena is a per-parse slab allocator that absorbs the four
// structural allocators identified in plan 197's allocator matrix:
//
//   - ast.NewTextSegment / ast.NewText
//   - ast.NewParagraph
//   - text.NewSegments (standalone, as in FindClosure)
//   - text.(*Segments).Append backing-array growth (incl. the
//     `lines` Segments embedded in every Paragraph)
//
// Lifetime: one Arena is created at the top of parser.Parse and
// thrown away when the AST consumer drops its references — the
// slabs are garbage-collected along with the tree. The plan-198
// risk section called out a hazard with sharing one arena across
// parses on a pooled parser (mdsmith's contentParserPool reuses
// parsers across documents, so a still-live AST from an earlier
// Parse can be clobbered by a later Parse). The per-parse design
// sidesteps that hazard: per-node allocation savings still land
// because a single slab absorbs many AST nodes, but slab reuse
// across parses is intentionally not attempted.
//
// Concurrency: an Arena is owned by exactly one Parse invocation
// at a time and is not safe for concurrent use. mdsmith's parser
// pool gives each Get caller exclusive access until Put; each
// Parse builds its own Arena.
//
// Nil safety: every public (*Arena) method tolerates a nil receiver
// and falls back to the upstream constructor / append, so call
// sites can replace `ast.NewText()` with `a.Text()` regardless of
// whether they're running on the arena path. The
// `goldmark_upstream` build tag turns the canonical path off by
// returning nil from `parser.newArenaForParse`.
package arena

import (
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
)

// Per-slab capacity. Tuned for the common-case corpus shape: most
// parses produce dozens of paragraphs and hundreds of text tokens,
// so a single slab usually fits one parse. Slabs grow on demand and
// are retained across Reset.
const (
	textSlabCap        = 256
	paragraphSlabCap   = 32
	headingSlabCap     = 32
	listItemSlabCap    = 64
	segmentsObjSlabCap = 64
	segmentSlabCap     = 1024
	codeSpanSlabCap    = 64
	linkSlabCap        = 32
	emphasisSlabCap    = 32

	// Initial Segment-backing capacity handed to a fresh Segments.
	// Four covers most paragraphs (one to four lines); when a
	// paragraph runs longer Grow doubles via the slab.
	initialSegmentCap = 4
)

// Arena owns the slab storage for one Parse call. parser.Parse
// creates a fresh Arena at entry; the slabs are released by GC
// when the caller drops references to the returned AST. The
// per-parser-with-Reset design described above was dropped to
// keep AST lifetime independent of the parser pool.
type Arena struct {
	texts        slabs[ast.Text]
	paragraphs   slabs[ast.Paragraph]
	headings     slabs[ast.Heading]
	listItems    slabs[ast.ListItem]
	segmentsObjs slabs[text.Segments]
	segments     []*segmentSlab
	codeSpans    slabs[ast.CodeSpan]
	links        slabs[ast.Link]
	emphases     slabs[ast.Emphasis]

	// segmentIdx is the segment-backing cursor; the node slabs carry
	// their own cursor inside slabs[T]. Reset rewinds every cursor to
	// zero so a reused arena (the engine's per-file pool) refills its
	// existing slabs from the start instead of only ever reusing the
	// last one and growing the list on every cycle.
	segmentIdx int
}

// slab is one fixed-capacity block of T values.
type slab[T any] struct {
	data []T
}

// slabs is the cursor-managed slab list one node type allocates from.
type slabs[T any] struct {
	list []*slab[T]
	idx  int
}

// alloc returns a pointer to a zero T carved from the cursor-selected
// slab, advancing the cursor past full slabs and appending a new slab
// of capacity capN when none has room. The returned pointer is stable:
// slab backing arrays never grow.
func (ss *slabs[T]) alloc(capN int) *T {
	for ss.idx < len(ss.list) {
		cur := ss.list[ss.idx]
		if len(cur.data) < cap(cur.data) {
			var zero T
			cur.data = append(cur.data, zero)
			return &cur.data[len(cur.data)-1]
		}
		ss.idx++
	}
	s := &slab[T]{data: make([]T, 0, capN)}
	var zero T
	s.data = append(s.data, zero)
	ss.list = append(ss.list, s)
	return &s.data[0]
}

// reset zeroes the used portion of every slab — dropping node
// pointers so a pooled arena does not pin the prior AST — and
// rewinds the cursor.
func (ss *slabs[T]) reset() {
	for _, s := range ss.list {
		clear(s.data)
		s.data = s.data[:0]
	}
	ss.idx = 0
}

// used reports how many T values are currently allocated.
func (ss *slabs[T]) used() int {
	n := 0
	for _, s := range ss.list {
		n += len(s.data)
	}
	return n
}

type segmentSlab struct {
	data []text.Segment
}

// New returns an empty Arena. The first allocation lazily provisions
// a slab; until then the Arena holds no backing memory.
func New() *Arena {
	return &Arena{}
}

// Reset returns the arena to its starting state without releasing
// the slab memory. Cursors drop to zero so the next allocation
// reuses the slab from offset 0. Reset is idempotent and nil-safe.
func (a *Arena) Reset() {
	if a == nil {
		return
	}
	// slabs.reset zeroes the live portion of each pointer-bearing
	// slab before reslicing so a reused Arena does not pin the prior
	// AST through stale parent/sibling pointers (ast.BaseNode et al.)
	// or through Segments.grow back-pointers. text.Segment has no
	// pointers, so the segment-backing slabs only need their length
	// reset.
	a.texts.reset()
	a.paragraphs.reset()
	a.headings.reset()
	a.listItems.reset()
	a.segmentsObjs.reset()
	a.codeSpans.reset()
	a.links.reset()
	a.emphases.reset()
	for _, s := range a.segments {
		s.data = s.data[:0]
	}
	a.segmentIdx = 0
}

// Text returns a zero-initialised *ast.Text from the arena. With a
// nil receiver falls back to ast.NewText.
func (a *Arena) Text() *ast.Text {
	if a == nil {
		return ast.NewText()
	}
	return a.texts.alloc(textSlabCap)
}

// TextSegment returns a *ast.Text initialised with the given source
// segment. With a nil receiver falls back to ast.NewTextSegment.
func (a *Arena) TextSegment(seg text.Segment) *ast.Text {
	if a == nil {
		return ast.NewTextSegment(seg)
	}
	t := a.Text()
	t.Segment = seg
	return t
}

// RawTextSegment returns a raw-flagged *ast.Text initialised with
// the given source segment. With a nil receiver falls back to
// ast.NewRawTextSegment.
func (a *Arena) RawTextSegment(seg text.Segment) *ast.Text {
	if a == nil {
		return ast.NewRawTextSegment(seg)
	}
	t := a.TextSegment(seg)
	t.SetRaw(true)
	return t
}

// Paragraph returns a zero-initialised *ast.Paragraph whose
// embedded Lines() is pre-equipped with an arena-backed grower, so
// every line appended to that paragraph also stays in arena memory.
// With a nil receiver falls back to ast.NewParagraph.
func (a *Arena) Paragraph() *ast.Paragraph {
	if a == nil {
		return ast.NewParagraph()
	}
	p := a.paragraphs.alloc(paragraphSlabCap)
	a.EquipLines(p.Lines())
	return p
}

// Heading returns a zero-initialised *ast.Heading with the given
// level from the arena. The block parsers (atx_heading, setext_headings)
// build every heading through this so the heading-dense neutral corpus
// no longer heap-allocates one ast.Heading per heading. With a nil
// receiver falls back to ast.NewHeading.
func (a *Arena) Heading(level int) *ast.Heading {
	if a == nil {
		return ast.NewHeading(level)
	}
	h := a.headings.alloc(headingSlabCap)
	h.Level = level
	return h
}

// ListItem returns a zero-initialised *ast.ListItem with the given
// offset from the arena. list_item's parser builds every item through
// this so list-dense documents no longer heap-allocate one
// ast.ListItem per item. With a nil receiver falls back to
// ast.NewListItem.
func (a *Arena) ListItem(offset int) *ast.ListItem {
	if a == nil {
		return ast.NewListItem(offset)
	}
	li := a.listItems.alloc(listItemSlabCap)
	li.Offset = offset
	return li
}

// RawHTML returns a *ast.RawHTML whose inline Segments is
// arena-backed. The RawHTML struct itself is heap-allocated (the
// arena does not slab it — it sees too few uses relative to Text
// and Paragraph to justify another slab type). With a nil receiver
// falls back to ast.NewRawHTML.
func (a *Arena) RawHTML() *ast.RawHTML {
	if a == nil {
		return ast.NewRawHTML()
	}
	return &ast.RawHTML{
		Segments: a.Segments(),
	}
}

// Segments returns a *text.Segments pre-equipped with arena-backed
// growth. The returned Segments lives in arena memory and its
// Append/AppendAll/Unshift calls allocate further backing space
// from the arena. With a nil receiver falls back to
// text.NewSegments.
func (a *Arena) Segments() *text.Segments {
	if a == nil {
		return text.NewSegments()
	}
	s := a.segmentsObjs.alloc(segmentsObjSlabCap)
	a.EquipLines(s)
	return s
}

// EquipLines installs an arena-backed grower on the given Segments
// (the lines field embedded in every BaseBlock).
//
// Precondition: s must be empty. EquipLines calls
// text.Segments.SetBacking, which replaces the values slice
// wholesale — any existing entries would be silently dropped. The
// arena uses EquipLines only on freshly-allocated Segments (from
// Paragraph and Segments).
//
// Nil-safe on both arguments.
func (a *Arena) EquipLines(s *text.Segments) {
	if a == nil || s == nil {
		return
	}
	backing := a.allocSegmentBacking(initialSegmentCap)
	s.SetBacking(backing, a)
}

// Grow implements text.SegmentsGrower. Called from
// *text.Segments.Append when the current backing slice has zero
// spare capacity; returns a doubled backing slice carved from the
// arena's segment slab, with the old entries copied in and the new
// segment already appended.
//
// Grow is safe to call with a nil receiver — it falls back to plain
// append, which keeps the upstream semantics for tests that build
// arena-less Segments through SetBacking with this Grower set to
// nil (the normal path).
func (a *Arena) Grow(old []text.Segment, next text.Segment) []text.Segment {
	if a == nil {
		return append(old, next)
	}
	newCap := cap(old) * 2
	if newCap == 0 {
		newCap = initialSegmentCap
	}
	fresh := a.allocSegmentBacking(newCap)
	fresh = fresh[:len(old)]
	copy(fresh, old)
	return append(fresh, next)
}

// CodeSpan returns a zero-initialised *ast.CodeSpan from the arena.
// With a nil receiver falls back to ast.NewCodeSpan.
func (a *Arena) CodeSpan() *ast.CodeSpan {
	if a == nil {
		return ast.NewCodeSpan()
	}
	return a.codeSpans.alloc(codeSpanSlabCap)
}

// Link returns a zero-initialised *ast.Link from the arena. With a
// nil receiver falls back to ast.NewLink.
func (a *Arena) Link() *ast.Link {
	if a == nil {
		return ast.NewLink()
	}
	return a.links.alloc(linkSlabCap)
}

// Emphasis returns a *ast.Emphasis with the given level from the
// arena. With a nil receiver falls back to ast.NewEmphasis.
func (a *Arena) Emphasis(level int) *ast.Emphasis {
	if a == nil {
		return ast.NewEmphasis(level)
	}
	em := a.emphases.alloc(emphasisSlabCap)
	em.Level = level
	return em
}

// allocSegmentBacking carves out the next n Segment slots from the
// segment-backing slab and returns a zero-length slice with cap n.
// If the current slab cannot fit n, a new slab is allocated; a
// request larger than the default slab size triggers a one-off slab
// sized to fit.
func (a *Arena) allocSegmentBacking(n int) []text.Segment {
	slab := a.currentSegmentSlab(n)
	start := len(slab.data)
	end := start + n
	slab.data = slab.data[:end]
	return slab.data[start:start:end]
}

func (a *Arena) currentSegmentSlab(needed int) *segmentSlab {
	for a.segmentIdx < len(a.segments) {
		cur := a.segments[a.segmentIdx]
		if cap(cur.data)-len(cur.data) >= needed {
			return cur
		}
		a.segmentIdx++
	}
	sz := segmentSlabCap
	if needed > sz {
		sz = needed
	}
	s := &segmentSlab{data: make([]text.Segment, 0, sz)}
	a.segments = append(a.segments, s)
	return s
}

// TextsAllocated reports how many Text nodes have been carved from
// the arena since the last Reset. Introspection for tests that need
// to prove a parse actually drew from a caller-supplied arena;
// nil-safe like every other method.
func (a *Arena) TextsAllocated() int {
	if a == nil {
		return 0
	}
	return a.texts.used()
}

// HeadingsAllocated reports how many Heading nodes have been carved
// from the arena since the last Reset. Nil-safe like TextsAllocated.
func (a *Arena) HeadingsAllocated() int {
	if a == nil {
		return 0
	}
	return a.headings.used()
}

// ListItemsAllocated reports how many ListItem nodes have been carved
// from the arena since the last Reset. Nil-safe like TextsAllocated.
func (a *Arena) ListItemsAllocated() int {
	if a == nil {
		return 0
	}
	return a.listItems.used()
}
