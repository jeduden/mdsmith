package lint

import "bytes"

// BlockKind classifies a Layer 0 block span by its leading construct.
type BlockKind uint8

const (
	// BlockParagraph is a run of non-blank lines that is not any other
	// block kind — the default classification.
	BlockParagraph BlockKind = iota
	// BlockFencedCode is a ``` / ~~~ fenced code block, including its
	// fence lines.
	BlockFencedCode
	// BlockIndentedCode is a 4-space / tab indented code block.
	BlockIndentedCode
	// BlockATXHeading is a `#`-prefixed heading line.
	BlockATXHeading
	// BlockSetextHeading is a heading whose underline is `=` or `-`.
	BlockSetextHeading
	// BlockQuote is a `>`-prefixed block quote.
	BlockQuote
	// BlockList is a list item run (bullet or ordered).
	BlockList
	// BlockHTML is an HTML block.
	BlockHTML
	// BlockPI is a `<?…?>` processing-instruction block.
	BlockPI
	// BlockThematicBreak is a `---` / `***` / `___` thematic break.
	BlockThematicBreak
)

// lineClass is a compact per-line classification bitfield produced by the
// Layer 0 scan. Bits are independent so a line can carry several roles
// (e.g. a fence line is both inside a code block and a fence boundary).
type lineClass uint8

const (
	// classCode marks a line that lies inside a fenced or indented code
	// block (including the fence lines themselves).
	classCode lineClass = 1 << iota
	// classPI marks a line that lies inside a processing-instruction
	// block (including its opening and closing markers).
	classPI
	// classBlank marks a whitespace-only line.
	classBlank
)

// BlockSpan is one Layer 0 block: its kind, its 1-based inclusive start
// and end source lines, and its block-quote/list nesting depth.
type BlockSpan struct {
	Kind  BlockKind
	Start int
	End   int
	Depth int
	// Closed is meaningful only for BlockFencedCode: true when the fence
	// has a matching closing delimiter, false when it runs to end of file
	// (or a trailing blank line) unclosed. tryFence already computes this;
	// MDS031 unclosed-code-block reads it on the parse-skip path. Always
	// false for every other block kind.
	Closed bool
}

// Layer0Scan is the product of one forward pass over File.Lines: a compact
// per-line classification, the code-block and processing-instruction line
// sets, and the block spans. It carries no node tree and is the only
// block-level projection source when a File was built with a nil AST (the
// parse-skipped path).
type Layer0Scan struct {
	// CodeBlockLines is the set of 1-based line numbers inside fenced or
	// indented code blocks, including fence lines. Read by
	// collectCodeBlockLines on the parse-skipped path.
	CodeBlockLines map[int]struct{}
	// PIBlockLines is the set of 1-based line numbers inside
	// processing-instruction blocks, including the opening and closing
	// marker lines. Read by collectPIBlockLines on the parse-skipped path.
	PIBlockLines map[int]struct{}

	// Classes and BlockSpans are the per-line classification and the
	// ordered block list. BlockSpans is now load-bearing in production:
	// rule.WalkBlocks drives every rule.BlockChecker (e.g. MDS002
	// heading-style, MDS010 fenced-code-style, MDS011 fenced-code-language,
	// MDS013 blank-line-around-headings, MDS015 blank-line-around-fenced-code,
	// MDS031 unclosed-code-block, MDS065 code-block-style, MDS066
	// commands-show-output) over these spans on the parse-skipped path, so
	// the block-kind dispatch that fills them must stay byte-faithful to
	// goldmark — a change here can alter shipped diagnostics. Classes remains
	// internal scaffolding for the scan.
	// Note: fenced code blocks nested inside block quotes are not emitted
	// as BlockFencedCode spans — tryBlockquote maps only CodeBlockLines.
	// Rules that need nested blocks must force the AST parse.

	// Classes holds one lineClass per source line, indexed by (line-1).
	Classes []lineClass
	// BlockSpans lists every block in document order.
	BlockSpans []BlockSpan
}

var piOpenMarker = []byte("<?")

var piCloseMarker = []byte("?>")

// Layer0 returns the cached single-pass block scan for f, computing it
// once on first use. The atomic.Bool + mutex memo matches the other
// File projections (see the File.codeBlockLines field comment) so the
// build sheds the closure box sync.Once would force. The returned
// pointer is shared read-only; callers must not mutate it.
func Layer0(f *File) *Layer0Scan {
	if f.layer0Done.Load() {
		return f.layer0
	}
	f.layer0Mu.Lock()
	defer f.layer0Mu.Unlock()
	if !f.layer0Done.Load() {
		defer f.layer0Done.Store(true)
		f.layer0 = scanLayer0(f.Lines)
	}
	return f.layer0
}

// scanLayer0 runs the single forward pass over lines. It pre-sizes both
// line-set maps to the line count so the common case (most lines in code
// or PI blocks for a code-heavy file) does not re-grow the map, keeping
// the scan inside the rule allocation budget.
func scanLayer0(lines [][]byte) *Layer0Scan {
	n := len(lines)
	l0 := &Layer0Scan{
		Classes:        make([]lineClass, n),
		CodeBlockLines: make(map[int]struct{}, n),
		PIBlockLines:   make(map[int]struct{}, n),
		// Pre-size the span slice so the common document (one block every
		// few lines) fills it without the geometric re-grows that
		// otherwise dominate the scan's allocation count. n/2+1 covers a
		// dense alternating block/blank layout in one allocation.
		BlockSpans: make([]BlockSpan, 0, n/2+1),
	}
	sc := scanner{lines: lines, l0: l0}
	sc.run()
	return l0
}

// scanner threads the per-pass cursor state through the block-recognition
// helpers so each is a small method rather than a closure capturing the
// loop variables.
type scanner struct {
	lines [][]byte
	l0    *Layer0Scan
	// i is the 0-based index of the line under inspection.
	i int
	// prevNonBlankParagraph records whether the immediately preceding
	// line opened or continued a paragraph, which governs whether an
	// indented line starts a code block (goldmark: indented code cannot
	// interrupt a paragraph).
	prevNonBlankParagraph bool
}

// run drives the forward pass: a block loop that dispatches on each line's
// leading construct. Front matter is intentionally NOT handled here: the
// goldmark parse this scan must match byte-for-byte never strips front
// matter (the engine strips it before constructing the File, so the scan
// receives an already-stripped body), and re-detecting a leading `---`
// pair here would mis-consume a body that legitimately opens with a
// thematic break followed by a later `---`. A leading `---` is therefore
// classified as a thematic break or setext underline, exactly as goldmark
// classifies it.
func (s *scanner) run() {
	for s.i < len(s.lines) {
		s.scanBlock()
	}
}

// trailingEmptyLine reports whether index i is the trailing empty element
// bytes.Split appends for a source ending in a newline. That element has
// no corresponding source line, so it is never classified or recorded.
func (s *scanner) trailingEmptyLine(i int) bool {
	return i == len(s.lines)-1 && len(s.lines[i]) == 0
}

// scanBlock recognises the block starting at the cursor and advances past
// it, recording its span and per-line classes. The dispatch order follows
// goldmark's block-parser precedence for the constructs the projections
// depend on.
func (s *scanner) scanBlock() {
	line := s.lines[s.i]
	if s.trailingEmptyLine(s.i) {
		s.i++
		return
	}
	if isBlankLine(line) {
		s.l0.Classes[s.i] |= classBlank
		s.prevNonBlankParagraph = false
		s.i++
		return
	}
	switch {
	case s.tryFence():
	case s.tryPI():
	case s.tryHTMLBlock(false):
	case s.tryATXHeading():
	case s.tryIndentedCode():
	case s.tryBlockquote():
	default:
		s.scanParagraph()
	}
}

// tryBlockquote recognises a block quote at the cursor and descends into
// its body. goldmark parses block constructs (fenced/indented code, PIs,
// nested quotes) inside a quote, so a `> ```\n> code\n> ```\n` quote
// contains a real code block whose lines must land in CodeBlockLines. The
// scan collects the run of `>`-prefixed lines (plus lazy paragraph
// continuations), strips one level of `>` (and an optional following
// space) to form the quote body, recursively scans that body, and maps the
// child's code line numbers back onto the parent lines.
//
// Known limitation: an unclosed fenced code block nested two or more quote
// levels deep (`> > ```\n> > x\n`) drops its phantom closing-fence line —
// the deeper level's phantom falls past this level's body, and the bounds
// guard skips it rather than panicking. Single-level unclosed fences,
// closed fences, and lazy continuations are all handled. The repository
// corpus contains no such shape (the equivalence harness is green) and the
// parse-skip gate is default-off, so the divergence is latent. Returns
// false when the cursor line is not a block quote.
func (s *scanner) tryBlockquote() bool {
	line := s.lines[s.i]
	if paragraphLeadKind(line) != BlockQuote {
		return false
	}
	start := s.i
	depth := blockDepth(line)
	// Collect the consecutive marker-led lines, stripping one quote level.
	// codeCapable records whether any stripped line could open a code block
	// (a fence or a >=4-column indent); the overwhelmingly common
	// prose-only block quote sets it false and skips the recursive scan and
	// its allocations entirely.
	remaining := len(s.lines) - s.i
	body := make([][]byte, 0, remaining)
	parentLine := make([]int, 0, remaining)
	codeCapable := false
	// openFence tracks whether a fenced code block opened by a marker line
	// is still open. A fenced code block inside a quote must keep its `>`
	// marker on every line — it does not accept lazy continuation — so a
	// non-marker line while a fence is open ends the quote rather than
	// extending the code.
	var openFence *fenceInfo
	for s.i < len(s.lines) {
		if s.trailingEmptyLine(s.i) {
			break
		}
		cur := s.lines[s.i]
		if isBlankLine(cur) {
			break
		}
		var stripped []byte
		if paragraphLeadKind(cur) == BlockQuote {
			stripped = stripQuoteMarker(cur)
		} else if openFence == nil && isLazyContinuation(cur) {
			// A non-marker plain-text line lazily continues the quote's open
			// paragraph; it carries no `>` to strip and maps through
			// verbatim. Suppressed while a fence is open (see openFence).
			stripped = cur
		} else {
			// A line that starts a new top-level block (heading, fence,
			// list, thematic break, HTML, PI) — or any non-marker line while
			// a fence is open — interrupts the quote.
			break
		}
		body = append(body, stripped)
		parentLine = append(parentLine, s.i)
		// Compute the fence-open once and feed both the codeCapable guard
		// (does the body need a recursive scan?) and the open-fence tracking
		// (can the next non-marker line lazily continue, or does the fence
		// forbid it?), so openingFence runs once per body line, not twice.
		fi, opensFence := openingFence(stripped)
		if !codeCapable && (opensFence || lineHasNonFenceCode(stripped)) {
			codeCapable = true
		}
		openFence = advanceFenceState(openFence, stripped, fi, opensFence)
		s.i++
	}
	// A fence still open when the quote ends has its phantom closing-fence
	// line one past the last body line — exactly the trailing element
	// bytes.Split appends at document level. Append that slot to the body
	// (mapped to the parent line after the last quote line) so the inner
	// scan records the phantom close at the same parent line the AST does.
	if openFence != nil && len(parentLine) > 0 {
		body = append(body, nil)
		parentLine = append(parentLine, parentLine[len(parentLine)-1]+1)
	}
	// Recursively scan the quote body only when it could contain code, and
	// translate the child's code line numbers (1-based within body) back to
	// parent line numbers. PI blocks open only at the document root
	// (piBlockParser.Open rejects a non-Document parent), so a `<?…?>`
	// inside a quote is not a PI — inner.PIBlockLines is never translated.
	if codeCapable {
		inner := scanLayer0(body)
		for ln := range inner.CodeBlockLines {
			// A phantom closing-fence line from a deeper recursion level can
			// fall one past this level's body (ln-1 == len(parentLine)); the
			// bounds check keeps that rare nested-unclosed-fence case from
			// panicking, at the cost of not marking the phantom line — a
			// benign under-mark covered by the known-limitation note.
			if ln-1 < len(parentLine) {
				if p := parentLine[ln-1]; p < len(s.lines) {
					s.markCode(p)
				}
			}
		}
	}
	s.addSpan(BlockQuote, start, s.i-1, depth)
	s.prevNonBlankParagraph = false
	return true
}

// tryPI recognises a processing-instruction block at the cursor. It
// mirrors the piBlockParser: an opening line with up to 3 spaces of
// indent, a `<?` prefix, and a non-empty name. A single-line PI closes on
// its opening line when `?>` appears (right-trimmed); otherwise the block
// runs until a line that is exactly `?>` after trimming. Returns false
// when the cursor line does not open a PI.
func (s *scanner) tryPI() bool {
	line := s.lines[s.i]
	if !opensPI(line) {
		return false
	}
	indent := leadingSpaces(line)
	trimmed := line[indent:]
	start := s.i
	s.markPI(s.i)
	if bytes.Contains(bytes.TrimRight(trimmed, " \t\r\n"), piCloseMarker) {
		s.i++
		s.addSpan(BlockPI, start, start, 0)
		s.prevNonBlankParagraph = false
		return true
	}
	s.i++
	for s.i < len(s.lines) {
		if s.trailingEmptyLine(s.i) {
			break
		}
		cur := s.lines[s.i]
		if bytes.Equal(bytes.TrimSpace(cur), piCloseMarker) {
			s.markPI(s.i)
			s.i++
			break
		}
		s.markPI(s.i)
		s.i++
	}
	s.addSpan(BlockPI, start, s.i-1, 0)
	s.prevNonBlankParagraph = false
	return true
}

// opensPI reports whether line opens a processing-instruction block: up
// to 3 spaces of indent, a `<?` prefix, and a non-empty name. Mirrors
// piBlockParser.Open's accept conditions.
func opensPI(line []byte) bool {
	indent := leadingSpaces(line)
	if indent > 3 {
		return false
	}
	trimmed := line[indent:]
	if !bytes.HasPrefix(trimmed, piOpenMarker) {
		return false
	}
	return len(piName(trimmed[2:])) > 0
}

// piName returns the PI name from the bytes after `<?`: the substring up
// to the first whitespace or `?>`. Mirrors extractPINameBytes.
func piName(b []byte) []byte {
	b = bytes.TrimRight(b, "\r\n")
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			return b[:i]
		}
		if c == '?' && i+1 < len(b) && b[i+1] == '>' {
			return b[:i]
		}
	}
	return b
}

// tryATXHeading recognises an ATX heading line (1–6 `#` after up to 3
// spaces of indent, followed by a space, tab, or end of line). Headings
// are a single line. Returns false when the cursor line is not a heading.
func (s *scanner) tryATXHeading() bool {
	if !isATXHeadingLine(s.lines[s.i]) {
		return false
	}
	start := s.i
	s.addSpan(BlockATXHeading, start, start, 0)
	s.i++
	s.prevNonBlankParagraph = false
	return true
}

// isATXHeadingLine reports whether line is an ATX heading: 1–6 `#` after
// up to 3 spaces of indent, followed by a space, tab, carriage return, or
// end of line. Mirrors atxHeadingParser.Open and is the same predicate
// tryATXHeading and scanParagraph (paragraph interruption) both use. It
// takes a full unstripped line (and so enforces the indent < 4 guard
// itself), distinct from lineclass_scan.go's indent-stripped isATXHeading.
func isATXHeadingLine(line []byte) bool {
	indent := leadingSpaces(line)
	if indent >= 4 {
		return false
	}
	j := indent
	for j < len(line) && line[j] == '#' {
		j++
	}
	level := j - indent
	if level < 1 || level > 6 {
		return false
	}
	return j >= len(line) || line[j] == ' ' || line[j] == '\t' || line[j] == '\r'
}

// tryIndentedCode recognises an indented code block at the cursor. An
// indented code block cannot interrupt a paragraph (goldmark:
// CanInterruptParagraph is false), so it opens only when the preceding
// line did not continue a paragraph. The block runs while lines stay
// 4-space/tab indented or blank, then trailing blank lines are excluded
// from the span and code classification (goldmark trims them on close).
// Returns false when the cursor line does not open indented code.
func (s *scanner) tryIndentedCode() bool {
	if s.prevNonBlankParagraph {
		return false
	}
	line := s.lines[s.i]
	if indentWidth(line) < 4 || isBlankLine(line) {
		return false
	}
	start := s.i
	lastNonBlank := s.i
	s.i++
	for s.i < len(s.lines) {
		if s.trailingEmptyLine(s.i) {
			break
		}
		cur := s.lines[s.i]
		if isBlankLine(cur) {
			s.i++
			continue
		}
		if indentWidth(cur) < 4 {
			break
		}
		lastNonBlank = s.i
		s.i++
	}
	// goldmark trims trailing blank lines from the block but keeps blank
	// lines interior to it (its Continue appends them as content lines).
	// Mark every line up to the last non-blank indented line as code,
	// including interior blanks, so the projection matches addBlockLines.
	for k := start; k <= lastNonBlank; k++ {
		s.markCode(k)
	}
	// Reset the cursor to just past the last code line; blank lines after
	// it are reclassified by the main loop.
	s.i = lastNonBlank + 1
	s.addSpan(BlockIndentedCode, start, lastNonBlank, 0)
	s.prevNonBlankParagraph = false
	return true
}

// addSpan appends a block span [start, end] (1-based, inclusive) of the
// given kind and nesting depth.
func (s *scanner) addSpan(kind BlockKind, start, end, depth int) {
	s.l0.BlockSpans = append(s.l0.BlockSpans, BlockSpan{
		Kind:  kind,
		Start: start + 1,
		End:   end + 1,
		Depth: depth,
	})
}

// markCode records the 0-based line i as a code line: it sets the code
// class bit and inserts the 1-based line number into CodeBlockLines.
func (s *scanner) markCode(i int) {
	s.l0.Classes[i] |= classCode
	s.l0.CodeBlockLines[i+1] = struct{}{}
}

// markPI records the 0-based line i as a PI line: it sets the PI class bit
// and inserts the 1-based line number into PIBlockLines.
func (s *scanner) markPI(i int) {
	s.l0.Classes[i] |= classPI
	s.l0.PIBlockLines[i+1] = struct{}{}
}

// leadingSpaces returns the count of leading ASCII space characters in
// line (tabs are not expanded — callers that need tab-aware indent width
// use indentWidth).
func leadingSpaces(line []byte) int {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	return i
}

// indentWidth returns the leading indentation width of line, counting a
// tab as advancing to the next 4-column stop (goldmark's tab handling for
// indented code) and a space as one column. It stops at the first
// non-whitespace byte.
func indentWidth(line []byte) int {
	w := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			w++
		case '\t':
			w += 4 - (w % 4)
		default:
			return w
		}
	}
	return w
}

// isBlankLine reports whether line contains only whitespace (spaces, tabs,
// carriage returns), or is empty.
func isBlankLine(line []byte) bool {
	for _, c := range line {
		if c != ' ' && c != '\t' && c != '\r' {
			return false
		}
	}
	return true
}
