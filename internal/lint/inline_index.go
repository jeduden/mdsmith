package lint

// Layer 1 inline index: a byte-level scan over File.Source that locates
// inline code spans without a goldmark parse. It is the parse-skip source
// for CodeSpanContentRanges / CodeSpanLiteralRanges when f.AST is nil,
// mirroring the way Layer 0 (layer0.go) backs the block-level code/PI line
// sets. The scan finds only what the re-backed rules need today — code
// spans — and is gated byte-identical to goldmark's CodeSpan node bounds by
// the corpus equivalence harness (internal/integration).
//
// A backtick code span is a run of N backticks (the opener) followed by a
// content run that contains no run of exactly N backticks, closed by the
// next run of exactly N backticks. CommonMark converts interior line
// endings to spaces and, when the content both begins and ends with a
// space and is not all spaces, strips one space from each side; goldmark
// records the post-trim bounds as the span's Text-child segments, so the
// scan reproduces that trim to keep CodeSpanContentRanges identical.
//
// Backticks inside fenced or indented code blocks are not code-span
// delimiters. The scan skips any byte on a line the Layer 0 scan marks as a
// code-block line, exactly as the AST walk never descends into a code block.

// InlineIndex is the product of one byte-level inline scan: the code-span
// content and literal byte ranges in document order. It carries no node
// tree and is the inline-projection source whenever f.AST is nil.
type InlineIndex struct {
	// CodeSpanContent holds each span's text-content range (backticks and
	// the CommonMark single-space trim excluded), in document order. Empty
	// (zero-width) content ranges are omitted, matching the AST projection.
	CodeSpanContent []Range
	// CodeSpanLiteral holds each span's range including its surrounding
	// backtick runs, in document order. Indexes correspond 1:1 with
	// CodeSpanContent's omission rule: a span contributes a literal range
	// only when it contributes a content range, matching the AST walk which
	// records a literal range only for spans with Text children.
	CodeSpanLiteral []Range
}

// InlineIndexProjection returns the cached inline scan for f, computing it
// once on first use. The atomic.Bool + mutex memo matches the other File
// projections (see the File.codeBlockLines field comment) so the build
// sheds the closure box sync.Once would force. The returned pointer is
// shared read-only; callers must not mutate it.
func InlineIndexProjection(f *File) *InlineIndex {
	if f.inlineIndexDone.Load() {
		return f.inlineIndex
	}
	f.inlineIndexMu.Lock()
	defer f.inlineIndexMu.Unlock()
	if !f.inlineIndexDone.Load() {
		defer f.inlineIndexDone.Store(true)
		f.inlineIndex = scanInlineIndex(f)
	}
	return f.inlineIndex
}

// scanInlineIndex runs the byte-level inline pass over f.Source, skipping
// bytes on lines that hold no inline content — fenced/indented code blocks,
// HTML blocks, and processing-instruction blocks (from the Layer 0 scan).
// goldmark never parses inline markup inside those blocks, so a backtick
// there is not a code-span delimiter. It returns an index whose code-span
// ranges match goldmark's CodeSpan node bounds.
func scanInlineIndex(f *File) *InlineIndex {
	idx := &InlineIndex{}
	codeLines := nonInlineLines(f)
	src := f.Source
	i := 0
	for i < len(src) {
		c := src[i]
		if c == '\\' {
			// A backslash escapes the next byte; a backslash-escaped
			// backtick cannot open a code span. Skip both bytes. Inside a
			// code block this is irrelevant (the line is skipped below), but
			// the cheap escape skip keeps the common prose path correct.
			i += 2
			continue
		}
		if c != '`' {
			i++
			continue
		}
		if lineInSet(f, codeLines, i) {
			i++
			continue
		}
		next := scanCodeSpanAt(f, codeLines, i)
		if next < 0 {
			// No closing run: the backtick run is literal text. Advance past
			// the whole run so its later backticks are not re-scanned as a
			// fresh opener (goldmark treats an unclosed run as text).
			i = endOfBacktickRun(src, i)
			continue
		}
		appendCodeSpan(idx, src, i, next)
		i = next
	}
	return idx
}

// nonInlineLines returns the set of 1-based source lines whose bytes carry
// no inline markup: fenced/indented code-block lines, PI-block lines, and
// every line inside an HTML block. goldmark parses no inline content on
// these lines, so a backtick there opens no code span. The set is built
// from the Layer 0 scan: its CodeBlockLines and PIBlockLines sets plus the
// line span of every BlockHTML block. Returns the CodeBlockLines map
// directly (no copy) when there are no PI or HTML lines to add, so the
// common code-only document allocates nothing extra.
func nonInlineLines(f *File) map[int]struct{} {
	l0 := Layer0(f)
	hasHTML := false
	for _, span := range l0.BlockSpans {
		if span.Kind == BlockHTML {
			hasHTML = true
			break
		}
	}
	if len(l0.PIBlockLines) == 0 && !hasHTML {
		return l0.CodeBlockLines
	}
	set := make(map[int]struct{}, len(l0.CodeBlockLines)+len(l0.PIBlockLines))
	for ln := range l0.CodeBlockLines {
		set[ln] = struct{}{}
	}
	for ln := range l0.PIBlockLines {
		set[ln] = struct{}{}
	}
	for _, span := range l0.BlockSpans {
		if span.Kind != BlockHTML {
			continue
		}
		for ln := span.Start; ln <= span.End; ln++ {
			set[ln] = struct{}{}
		}
	}
	return set
}

// scanCodeSpanAt tests whether a code span opens at the backtick at src[i]
// and, if so, returns the byte offset just past its closing backtick run.
// It returns -1 when the run has no matching closing run (the opener is
// literal text). The opening run length is the count of consecutive
// backticks; the span closes at the next run of exactly that length whose
// bytes are not on a code-block line.
func scanCodeSpanAt(f *File, codeLines map[int]struct{}, i int) int {
	src := f.Source
	openEnd := endOfBacktickRun(src, i)
	runLen := openEnd - i
	j := openEnd
	for j < len(src) {
		if src[j] != '`' {
			j++
			continue
		}
		if lineInSet(f, codeLines, j) {
			j++
			continue
		}
		closeEnd := endOfBacktickRun(src, j)
		if closeEnd-j == runLen {
			return closeEnd
		}
		// A run of a different length is interior content; skip it whole so
		// its backticks are not re-tested one at a time.
		j = closeEnd
	}
	return -1
}

// appendCodeSpan records one span's content and literal ranges, matching
// goldmark exactly. goldmark records the span's Text-child segment as the
// post-trim content (the CommonMark single-space trim applied), then the
// projection derives the literal range by extending that content over any
// backtick bytes immediately adjacent to it (collectCodeSpanRangesInto in
// codespans.go). The literal therefore differs from the raw [openStart,
// closeEnd) span when a trimmed-off space separates the content from a
// delimiter backtick — so the literal is computed the same two-step way
// here. A zero-width content range contributes neither entry, matching the
// AST projection which records nothing for a span with no Text child.
func appendCodeSpan(idx *InlineIndex, src []byte, openStart, closeEnd int) {
	openEnd := endOfBacktickRun(src, openStart)
	runLen := openEnd - openStart
	contentStart := openEnd
	contentEnd := closeEnd - runLen
	cs, ce := trimCodeSpanContent(src, contentStart, contentEnd)
	if ce <= cs {
		return
	}
	idx.CodeSpanContent = append(idx.CodeSpanContent, Range{Start: cs, End: ce})
	ls := cs
	for ls > 0 && src[ls-1] == '`' {
		ls--
	}
	le := ce
	for le < len(src) && src[le] == '`' {
		le++
	}
	idx.CodeSpanLiteral = append(idx.CodeSpanLiteral, Range{Start: ls, End: le})
}

// trimCodeSpanContent reproduces CommonMark's code-span content trim that
// goldmark records: when the content both begins and ends with a space (or
// line-ending byte goldmark treats as a space) and is not made up entirely
// of such bytes, one byte is stripped from each side. The returned range is
// the post-trim content [start, end).
func trimCodeSpanContent(src []byte, start, end int) (int, int) {
	if end <= start {
		return start, end
	}
	if !isCodeSpanSpace(src[start]) || !isCodeSpanSpace(src[end-1]) {
		return start, end
	}
	if allCodeSpanSpaces(src, start, end) {
		return start, end
	}
	return start + 1, end - 1
}

// isCodeSpanSpace reports whether b is a byte CommonMark folds to a space
// inside a code span for the strip-one-space rule: a space or a line ending.
func isCodeSpanSpace(b byte) bool {
	return b == ' ' || b == '\n' || b == '\r'
}

// allCodeSpanSpaces reports whether every byte in [start, end) is a
// code-span space byte; such a span is never trimmed (CommonMark keeps an
// all-space span verbatim).
func allCodeSpanSpaces(src []byte, start, end int) bool {
	for k := start; k < end; k++ {
		if !isCodeSpanSpace(src[k]) {
			return false
		}
	}
	return true
}

// endOfBacktickRun returns the offset just past the run of backticks that
// starts at src[i]. src[i] must be a backtick.
func endOfBacktickRun(src []byte, i int) int {
	j := i
	for j < len(src) && src[j] == '`' {
		j++
	}
	return j
}

// lineInSet reports whether the source byte at offset belongs to a line in
// set (a 1-based line-number set, e.g. the Layer 0 code-block lines).
func lineInSet(f *File, set map[int]struct{}, offset int) bool {
	if len(set) == 0 {
		return false
	}
	_, ok := set[f.LineOfOffset(offset)]
	return ok
}
