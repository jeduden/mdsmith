package markdown

import "fmt"

// Edit is a half-open byte range [Start, End) to remove from a body,
// optionally with replacement bytes spliced in at the same position.
// A nil or zero-length Repl is a pure deletion (the original
// behavior); a non-empty Repl gives byte-exact replacement, which
// rewriters like MDS034's bare-URL wrap rely on.
type Edit struct {
	Start int
	End   int
	Repl  []byte
}

// Splice returns a new slice equal to body with every edit range
// removed and its Repl bytes spliced in at the original position, in
// a single left-to-right pass. Edits must be ascending and
// non-overlapping — the order an AST walk over a parsed Document
// naturally yields heading and processing-instruction spans, so a
// caller collecting spans in document order can pass them straight
// through.
//
// Panics with a descriptive message when the precondition is
// violated (edits out of order or overlapping). The raw Go panic
// from a downstream slice-bounds-out-of-range is unhelpful when
// debugging a rewriter that produced bad edits; surfacing the
// invariant at the entry point makes the failure self-diagnostic.
//
// This is mdsmith's producer: it is byte-exact span surgery on the
// original source, not an AST-to-Markdown re-render, so its output
// never fights `mdsmith fix` (which is itself edit-based). body is
// not mutated; a fresh slice is returned.
func Splice(body []byte, edits []Edit) []byte {
	if len(edits) == 0 {
		out := make([]byte, len(body))
		copy(out, body)
		return out
	}
	// Enforce the documented contract once at the entry point so
	// callers learn about edit-list defects loudly rather than via
	// an opaque slice-bounds panic during the build loop below.
	prevEnd := 0
	for i, e := range edits {
		if e.Start < 0 {
			panic(fmt.Sprintf(
				"markdown.Splice: edit %d has negative Start "+
					"({Start:%d, End:%d})", i, e.Start, e.End))
		}
		if e.Start < prevEnd {
			panic(fmt.Sprintf(
				"markdown.Splice: edits must be ascending and "+
					"non-overlapping; edit %d {Start:%d, End:%d} "+
					"overlaps previous edit ending at %d",
				i, e.Start, e.End, prevEnd))
		}
		if e.End < e.Start {
			panic(fmt.Sprintf(
				"markdown.Splice: edit %d has End<Start "+
					"({Start:%d, End:%d})", i, e.Start, e.End))
		}
		if e.End > len(body) {
			panic(fmt.Sprintf(
				"markdown.Splice: edit %d {Start:%d, End:%d} "+
					"exceeds body length %d",
				i, e.Start, e.End, len(body)))
		}
		prevEnd = e.End
	}
	total := len(body)
	for _, e := range edits {
		total -= e.End - e.Start
		total += len(e.Repl)
	}
	out := make([]byte, 0, total)
	prev := 0
	for _, e := range edits {
		out = append(out, body[prev:e.Start]...)
		out = append(out, e.Repl...)
		prev = e.End
	}
	out = append(out, body[prev:]...)
	return out
}
