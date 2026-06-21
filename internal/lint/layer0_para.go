package lint

import "bytes"

var (
	fenceBacktickRun = []byte("```")
	fenceTildeRun    = []byte("~~~")
	fourSpaceRun     = []byte("    ")
)

// SourceMayHaveCodeBlock reports whether source could contain a fenced or
// indented code block: it holds a fenced-code marker run (``` or ~~~), a tab,
// or a run of four spaces. Every code block forces one of these bytes —
// fences need three backticks or tildes; an indented code block needs a
// four-column indent, which is four spaces or a tab — regardless of how
// deeply the block nests inside lists or block quotes.
//
// The Layer 0 parse-skip gate skips the goldmark parse only when this returns
// false. A source with none of these markers has no code block, so its
// CollectCodeBlockLines is empty under both the Layer 0 scan and the AST and
// the line-based rules behave identically. Any source that might hold code is
// parsed normally, which sidesteps every Layer 0/AST CodeBlockLines
// divergence — all of which require a code block to be present (the scanner
// does not descend into a list item's content, so a fence or indent inside a
// list item is the known divergence class; this guard makes the gate
// indifferent to it). The check is deliberately coarse — an inline `code`
// span or a column of alignment spaces also trips it — but provably sound,
// allocation-free, and far more robust than re-deriving goldmark's
// container-aware code-block detection in the gate.
func SourceMayHaveCodeBlock(source []byte) bool {
	return bytes.IndexByte(source, '\t') >= 0 ||
		bytes.Contains(source, fenceBacktickRun) ||
		bytes.Contains(source, fenceTildeRun) ||
		bytes.Contains(source, fourSpaceRun)
}

// SourceMayHaveBlockQuote reports whether source could contain a block
// quote: it holds at least one `>` byte. A block quote requires a `>`
// marker, so a source with no `>` has no quote.
//
// The Layer 0 parse-skip gate skips the goldmark parse only when this
// returns false. The scanner collapses a block quote into a single
// BlockQuote span and does not descend into its body to emit the
// heading and fenced-code spans block-kind rules (MDS002, MDS015) react
// to, so a quote-nested heading or fence is invisible to the block scan
// while the AST path still flags it. Disqualifying any source that might
// hold a quote sidesteps that divergence the same way the code-block
// guard handles a list-nested code block. The check is deliberately
// coarse — a `>` in an autolink, raw HTML, or prose also trips it — but
// provably sound and allocation-free.
func SourceMayHaveBlockQuote(source []byte) bool {
	return bytes.IndexByte(source, '>') >= 0
}

// scanParagraph consumes a paragraph: the run of non-blank lines that no
// other block kind claimed, stopping at a blank line, a fence, a PI, or an
// ATX heading. A `---` / `===` underline directly under a paragraph line
// promotes the run to a setext heading. Block quotes and list markers are
// recorded by kind but otherwise scanned as a single line so their inner
// constructs (which the projections do not depend on) stay simple.
func (s *scanner) scanParagraph() {
	start := s.i
	line := s.lines[s.i]
	kind := paragraphLeadKind(line)
	if kind != BlockParagraph {
		s.addSpan(kind, start, start, blockDepth(line))
		s.i++
		s.prevNonBlankParagraph = kind == BlockQuote || kind == BlockList
		return
	}
	s.i++
	for s.i < len(s.lines) {
		if s.trailingEmptyLine(s.i) {
			break
		}
		cur := s.lines[s.i]
		if isBlankLine(cur) {
			break
		}
		if isSetextUnderline(cur) {
			s.markSetextRun(start, s.i)
			s.i++
			s.prevNonBlankParagraph = false
			return
		}
		if _, ok := openingFence(cur); ok {
			break
		}
		if opensPI(cur) {
			break
		}
		// An ATX heading interrupts a paragraph (atxHeadingParser:
		// CanInterruptParagraph is true), so the paragraph ends before it.
		if isATXHeadingLine(cur) {
			break
		}
		// HTML blocks of types 1–6 can interrupt a paragraph (type 7
		// cannot, so inParagraph is true here).
		if openHTMLBlock(cur, true) != htmlNone {
			break
		}
		if paragraphLeadKind(cur) != BlockParagraph {
			break
		}
		s.i++
	}
	s.addSpan(BlockParagraph, start, s.i-1, 0)
	s.prevNonBlankParagraph = true
}

// markSetextRun records the paragraph run [start, underline] as a setext
// heading span.
func (s *scanner) markSetextRun(start, underline int) {
	s.addSpan(BlockSetextHeading, start, underline, 0)
}

// paragraphLeadKind classifies a non-blank, non-code, non-PI, non-ATX line
// by its leading marker so the paragraph scan can break on block-quote and
// list boundaries and tag thematic breaks. Returns BlockParagraph for an
// ordinary text line.
func paragraphLeadKind(line []byte) BlockKind {
	indent := leadingSpaces(line)
	if indent >= 4 || indent >= len(line) {
		return BlockParagraph
	}
	switch line[indent] {
	case '>':
		return BlockQuote
	case '*', '-', '+':
		if isThematicBreak(line) {
			return BlockThematicBreak
		}
		if isBulletMarker(line, indent) {
			return BlockList
		}
		return BlockParagraph
	case '_':
		if isThematicBreak(line) {
			return BlockThematicBreak
		}
		return BlockParagraph
	}
	if isOrderedMarker(line, indent) {
		return BlockList
	}
	return BlockParagraph
}

// blockDepth returns the block-quote nesting depth of line: the number of
// leading `>` markers (each optionally followed by a space), after up to 3
// spaces of indent. Non-quote lines are depth 0.
func blockDepth(line []byte) int {
	depth := 0
	i := 0
	for {
		j := i
		for j < len(line) && j-i < 4 && line[j] == ' ' {
			j++
		}
		if j < len(line) && line[j] == '>' {
			depth++
			j++
			if j < len(line) && line[j] == ' ' {
				j++
			}
			i = j
			continue
		}
		break
	}
	return depth
}

// isLazyContinuation reports whether line can lazily continue an open
// block quote paragraph or code block: a non-blank line that does not begin
// a new top-level block. Per CommonMark, a line starting a fence, PI, HTML
// block, ATX heading, list, thematic break, or nested quote interrupts the
// quote instead of continuing it; everything else (plain text, including a
// 4-space-indented line, which cannot start indented code mid-paragraph)
// is a lazy continuation. The caller has already handled the quote-marker
// and blank-line cases, so this only classifies non-marker, non-blank
// lines.
func isLazyContinuation(line []byte) bool {
	if _, ok := openingFence(line); ok {
		return false
	}
	if opensPI(line) || openHTMLBlock(line, true) != htmlNone {
		return false
	}
	return paragraphLeadKind(line) == BlockParagraph
}

// lineHasNonFenceCode reports whether line could contribute a code block to
// a recursively-scanned block-quote body for a reason OTHER than opening a
// fence — the caller already tested the fence case via openingFence and
// folds it in separately. It is true when the line carries a >=4-column
// indent (potential indented code) or is itself a nested block quote
// (whose deeper levels may hold code only the recursive scan can reach).
// May over-report (an indented or quoted line that yields no code), which
// only costs a recursion that finds nothing; it must never under-report.
func lineHasNonFenceCode(line []byte) bool {
	if indentWidth(line) >= 4 && !isBlankLine(line) {
		return true
	}
	return paragraphLeadKind(line) == BlockQuote
}

// stripQuoteMarker removes one block-quote level from line: up to 3 spaces
// of indent, a `>`, and one optional following space. A line with no
// marker (a lazy continuation) is returned unchanged.
func stripQuoteMarker(line []byte) []byte {
	i := leadingSpaces(line)
	if i >= len(line) || line[i] != '>' {
		return line
	}
	i++
	if i < len(line) && line[i] == ' ' {
		i++
	}
	return line[i:]
}

// isBulletMarker reports whether the marker at indent is a list bullet
// (`-`, `*`, `+` followed by a space, tab, or end of line).
func isBulletMarker(line []byte, indent int) bool {
	j := indent + 1
	return j >= len(line) || line[j] == ' ' || line[j] == '\t' || line[j] == '\r'
}

// isOrderedMarker reports whether line opens with an ordered-list marker:
// 1–9 digits, a `.` or `)`, then a space, tab, or end of line.
func isOrderedMarker(line []byte, indent int) bool {
	j := indent
	digits := 0
	for j < len(line) && line[j] >= '0' && line[j] <= '9' {
		j++
		digits++
	}
	if digits == 0 || digits > 9 {
		return false
	}
	if j >= len(line) || (line[j] != '.' && line[j] != ')') {
		return false
	}
	j++
	return j >= len(line) || line[j] == ' ' || line[j] == '\t' || line[j] == '\r'
}

// isThematicBreak reports whether line is a thematic break: at most 3
// spaces of indent, then 3 or more of a single `-`, `*`, or `_` character
// with only spaces interspersed.
func isThematicBreak(line []byte) bool {
	indent := leadingSpaces(line)
	if indent >= 4 || indent >= len(line) {
		return false
	}
	ch := line[indent]
	if ch != '-' && ch != '*' && ch != '_' {
		return false
	}
	count := 0
	for j := indent; j < len(line); j++ {
		switch c := line[j]; c {
		case ch:
			count++
		case ' ', '\t', '\r':
		default:
			return false
		}
	}
	return count >= 3
}

// isSetextUnderline reports whether line is a setext heading underline: at
// most 3 spaces of indent, then a run of only `=` or only `-` characters
// (with optional trailing spaces).
func isSetextUnderline(line []byte) bool {
	indent := leadingSpaces(line)
	if indent >= 4 || indent >= len(line) {
		return false
	}
	ch := line[indent]
	if ch != '=' && ch != '-' {
		return false
	}
	j := indent
	for j < len(line) && line[j] == ch {
		j++
	}
	return isBlankLine(line[j:])
}
