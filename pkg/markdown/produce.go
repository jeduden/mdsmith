package markdown

// Edit is a half-open byte range [Start, End) to remove from a body.
type Edit struct {
	Start int
	End   int
}

// Splice returns a new slice equal to body with every edit range
// removed, in a single left-to-right pass. Edits must be ascending
// and non-overlapping — the order an AST walk over a parsed Document
// naturally yields heading and processing-instruction spans, so a
// caller collecting spans in document order can pass them straight
// through.
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
	total := len(body)
	for _, e := range edits {
		total -= e.End - e.Start
	}
	out := make([]byte, 0, total)
	prev := 0
	for _, e := range edits {
		out = append(out, body[prev:e.Start]...)
		prev = e.End
	}
	out = append(out, body[prev:]...)
	return out
}
