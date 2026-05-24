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
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Per-slab capacity. Tuned for the common-case corpus shape: most
// parses produce dozens of paragraphs and hundreds of text tokens,
// so a single slab usually fits one parse. Slabs grow on demand and
// are retained across Reset.
const (
	textSlabCap        = 256
	paragraphSlabCap   = 32
	segmentsObjSlabCap = 64
	segmentSlabCap     = 1024

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
	texts        []*textSlab
	paragraphs   []*paragraphSlab
	segmentsObjs []*segmentsObjSlab
	segments     []*segmentSlab
}

type textSlab struct {
	data []ast.Text
}

type paragraphSlab struct {
	data []ast.Paragraph
}

type segmentsObjSlab struct {
	data []text.Segments
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
	// Zero the live portion of each pointer-bearing slab before
	// reslicing so a reused Arena does not pin the prior AST
	// through stale parent/sibling pointers (ast.BaseNode et al.)
	// or through Segments.grow back-pointers. Without clear(), the
	// GC sees the old structs as still reachable via the slab
	// array and the previously-parsed tree (plus its arena slabs)
	// stays alive across Reset.
	//
	// text.Segment has no pointers, so the segment-backing slabs
	// only need their length reset.
	for _, s := range a.texts {
		clear(s.data)
		s.data = s.data[:0]
	}
	for _, s := range a.paragraphs {
		clear(s.data)
		s.data = s.data[:0]
	}
	for _, s := range a.segmentsObjs {
		clear(s.data)
		s.data = s.data[:0]
	}
	for _, s := range a.segments {
		s.data = s.data[:0]
	}
}

// Text returns a zero-initialised *ast.Text from the arena. With a
// nil receiver falls back to ast.NewText.
func (a *Arena) Text() *ast.Text {
	if a == nil {
		return ast.NewText()
	}
	slab := a.currentTextSlab()
	slab.data = append(slab.data, ast.Text{})
	return &slab.data[len(slab.data)-1]
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
	slab := a.currentParagraphSlab()
	slab.data = append(slab.data, ast.Paragraph{})
	p := &slab.data[len(slab.data)-1]
	a.EquipLines(p.Lines())
	return p
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
	slab := a.currentSegmentsObjSlab()
	slab.data = append(slab.data, text.Segments{})
	s := &slab.data[len(slab.data)-1]
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

func (a *Arena) currentTextSlab() *textSlab {
	if n := len(a.texts); n > 0 {
		cur := a.texts[n-1]
		if len(cur.data) < cap(cur.data) {
			return cur
		}
	}
	s := &textSlab{data: make([]ast.Text, 0, textSlabCap)}
	a.texts = append(a.texts, s)
	return s
}

func (a *Arena) currentParagraphSlab() *paragraphSlab {
	if n := len(a.paragraphs); n > 0 {
		cur := a.paragraphs[n-1]
		if len(cur.data) < cap(cur.data) {
			return cur
		}
	}
	s := &paragraphSlab{data: make([]ast.Paragraph, 0, paragraphSlabCap)}
	a.paragraphs = append(a.paragraphs, s)
	return s
}

func (a *Arena) currentSegmentsObjSlab() *segmentsObjSlab {
	if n := len(a.segmentsObjs); n > 0 {
		cur := a.segmentsObjs[n-1]
		if len(cur.data) < cap(cur.data) {
			return cur
		}
	}
	s := &segmentsObjSlab{data: make([]text.Segments, 0, segmentsObjSlabCap)}
	a.segmentsObjs = append(a.segmentsObjs, s)
	return s
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
	if n := len(a.segments); n > 0 {
		cur := a.segments[n-1]
		if cap(cur.data)-len(cur.data) >= needed {
			return cur
		}
	}
	sz := segmentSlabCap
	if needed > sz {
		sz = needed
	}
	s := &segmentSlab{data: make([]text.Segment, 0, sz)}
	a.segments = append(a.segments, s)
	return s
}
