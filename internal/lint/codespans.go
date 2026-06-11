package lint

import (
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// CodeSpanContentRanges returns the source byte range of every inline
// code span's text content (backticks excluded), in document order.
// Ranges with no content (no Text children, or zero-width bounds) are
// omitted. Computed once per File and cached; the returned slice is
// shared read-only and must not be mutated.
//
// Several rules (reversed-link detection, ambiguous emphasis,
// reference-label scanning) each used to re-walk the AST to rediscover
// the same spans; the memo amortizes that to one walk per file.
func (f *File) CodeSpanContentRanges() []Range {
	f.ensureCodeSpanRanges()
	return f.codeSpanContent
}

// CodeSpanLiteralRanges returns the source byte range of every inline
// code span including its surrounding backtick runs, in document
// order. Spans with no Text children are omitted. Computed once per
// File and cached; the returned slice is shared read-only and must not
// be mutated.
func (f *File) CodeSpanLiteralRanges() []Range {
	f.ensureCodeSpanRanges()
	return f.codeSpanLiteral
}

// ensureCodeSpanRanges builds both cached projections in one AST walk.
// The atomic.Bool + mutex memo matches the other File caches (see the
// newlineOffsets field comment for why not sync.Once).
func (f *File) ensureCodeSpanRanges() {
	if f.codeSpansDone.Load() {
		return
	}
	f.codeSpansMu.Lock()
	defer f.codeSpansMu.Unlock()
	if !f.codeSpansDone.Load() {
		defer f.codeSpansDone.Store(true)
		collectCodeSpanRangesInto(f.AST, f.Source, &f.codeSpanContent, &f.codeSpanLiteral)
	}
}

// collectCodeSpanRangesInto descends node n via recursion (no closure
// box) and appends each code span's content range and backtick-extended
// literal range. nil n (struct-literal Files with no AST) appends
// nothing.
func collectCodeSpanRangesInto(n ast.Node, source []byte, content, literal *[]Range) {
	if n == nil {
		return
	}
	if _, ok := n.(*ast.CodeSpan); ok {
		first, last := codeSpanTextBounds(n)
		if first >= 0 {
			if last > first {
				*content = append(*content, Range{Start: first, End: last})
			}
			start := first
			for start > 0 && source[start-1] == '`' {
				start--
			}
			end := last
			for end < len(source) && source[end] == '`' {
				end++
			}
			*literal = append(*literal, Range{Start: start, End: end})
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectCodeSpanRangesInto(c, source, content, literal)
	}
}

// codeSpanTextBounds returns the minimal and maximal source offsets of
// n's Text children, or (-1, -1) when it has none.
func codeSpanTextBounds(n ast.Node) (first, last int) {
	first, last = -1, -1
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok {
			continue
		}
		if first == -1 || t.Segment.Start < first {
			first = t.Segment.Start
		}
		if t.Segment.Stop > last {
			last = t.Segment.Stop
		}
	}
	return first, last
}

// LineStartOffset returns the byte offset in Source where 0-based line
// i begins. Indexes past the last line clamp to len(Source); negative
// indexes clamp to 0. Backed by the same cached newline index
// LineOfOffset uses, so a per-rule line-starts rebuild is unnecessary.
func (f *File) LineStartOffset(i int) int {
	if i <= 0 {
		return 0
	}
	nl := f.lineIndex()
	if i-1 >= len(nl) {
		return len(f.Source)
	}
	return nl[i-1] + 1
}
