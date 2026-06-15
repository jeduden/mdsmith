package lint

import (
	"unsafe"

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
		// On the parse-skipped path (f.AST nil) the goldmark walk would
		// surface nothing, so serve from the Layer 1 inline index instead.
		// A struct-literal File with neither an AST nor a source has no code
		// spans either way. The corpus equivalence harness pins the two
		// projection sources byte-identical.
		if f.AST == nil {
			if len(f.Source) == 0 {
				return
			}
			idx := InlineIndexProjection(f)
			f.codeSpanContent = idx.CodeSpanContent
			f.codeSpanLiteral = idx.CodeSpanLiteral
			return
		}
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

// LineStrings returns f.Lines as zero-copy strings (one per line,
// same indexes). Computed once per File and cached; the returned
// slice is shared read-only. Consumers that hand out per-diagnostic
// context windows slice it instead of allocating a fresh []string
// with copied lines per diagnostic.
//
// The strings alias the source buffer via unsafe.String. Invariant:
// the source is never mutated after the File is built — check never
// writes it and fix builds replacement content in fresh buffers — so
// the views stay valid for as long as any consumer holds them.
func (f *File) LineStrings() []string {
	if f.lineStringsDone.Load() {
		return f.lineStrings
	}
	f.lineStringsMu.Lock()
	defer f.lineStringsMu.Unlock()
	if !f.lineStringsDone.Load() {
		defer f.lineStringsDone.Store(true)
		if len(f.Lines) > 0 {
			ls := make([]string, len(f.Lines))
			for i, b := range f.Lines {
				ls[i] = BytesView(b)
			}
			f.lineStrings = ls
		}
	}
	return f.lineStrings
}

// BytesView returns b's bytes as a string without copying. The caller
// must guarantee b is never mutated while the string is reachable.
func BytesView(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// MaskRanges returns line with any bytes that fall inside one of the
// given source byte ranges replaced by spaces. lineStart is the
// line's byte offset in Source (see LineStartOffset). The original
// slice is returned unchanged when no range overlaps, so the common
// path allocates nothing. Rules use it to blank code-span content
// before pattern-matching a line.
func MaskRanges(line []byte, lineStart int, ranges []Range) []byte {
	lineEnd := lineStart + len(line)
	var out []byte
	for _, rg := range ranges {
		if rg.End <= lineStart || rg.Start >= lineEnd {
			continue
		}
		if out == nil {
			out = make([]byte, len(line))
			copy(out, line)
		}
		from := max(rg.Start-lineStart, 0)
		to := min(rg.End-lineStart, len(out))
		for k := from; k < to; k++ {
			out[k] = ' '
		}
	}
	if out == nil {
		return line
	}
	return out
}
