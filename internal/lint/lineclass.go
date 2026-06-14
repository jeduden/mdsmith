package lint

import "bytes"

// LineClass is the flat Layer-0 classification of one source line. It is
// the per-line product of ClassifyLines — a node-tree-free alternative to
// navigating the goldmark block tree for the handful of facts line rules
// read (which lines are code, which are headings). The classes mirror the
// vocabulary in the lazy-parse research note's Layer-0 table.
type LineClass uint8

const (
	// LineParagraph is the default: ordinary text, a lazy paragraph
	// continuation, or any line no other class claims.
	LineParagraph LineClass = iota
	// LineBlank is an empty or whitespace-only line (after container
	// prefixes are stripped).
	LineBlank
	// LineATXHeading is an ATX heading line (`#`..`######`).
	LineATXHeading
	// LineSetextUnderline is a setext underline (`=`/`-` run) under a
	// paragraph.
	LineSetextUnderline
	// LineFenceOpen opens a fenced code block.
	LineFenceOpen
	// LineFenceClose closes a fenced code block.
	LineFenceClose
	// LineInCode is a content line inside a fenced or indented code
	// block.
	LineInCode
	// LineHTML is an HTML block line.
	LineHTML
	// LineFrontMatter is a line inside the leading YAML front-matter
	// fence (including its `---` delimiters).
	LineFrontMatter
)

// LineClassifier holds the result of one forward pass over a document's
// lines: a per-line class, the code-block line set, the heading line set,
// and the front-matter bounds. It builds no ast.Node and no heap node
// tree — only a flat class slice and two lazily-allocated line sets — so
// the common line rules (line-length and the other CollectCodeBlockLines
// consumers) can run without the goldmark parse. See plan
// 2606142147 and docs/research/benchmarks/lazy-parse-architecture.md.
type LineClassifier struct {
	classes      []LineClass
	codeBlock    map[int]struct{}
	heading      map[int]struct{}
	fmFrom, fmTo int // 1-based inclusive front-matter bounds; 0 when absent
}

// CodeBlockLines returns the 1-based line numbers inside fenced or
// indented code blocks (fence lines included), byte-compatible with the
// AST-derived lint.CollectCodeBlockLines. The returned map is shared
// read-only and must not be mutated. nil when the document has no code.
func (lc *LineClassifier) CodeBlockLines() map[int]struct{} { return lc.codeBlock }

// HeadingLines returns the 1-based line numbers of ATX headings and
// setext underlines. The returned map is shared read-only. nil when the
// document has no headings.
func (lc *LineClassifier) HeadingLines() map[int]struct{} { return lc.heading }

// Class returns the LineClass of the 1-based line, or LineParagraph when
// line is out of range.
func (lc *LineClassifier) Class(line int) LineClass {
	if line < 1 || line > len(lc.classes) {
		return LineParagraph
	}
	return lc.classes[line-1]
}

// FrontMatter reports the 1-based inclusive line bounds of a leading YAML
// front-matter block, and whether one was found. The engine's flat path
// feeds ClassifyLines already-stripped content, so this only fires when a
// caller classifies a whole file; it lets the classifier stand alone.
func (lc *LineClassifier) FrontMatter() (from, to int, ok bool) {
	return lc.fmFrom, lc.fmTo, lc.fmFrom > 0
}

// FlatHeadingLines returns the heading-line set from f's flat Layer-0
// classifier and true when the File was built on the parse-skip path. It
// returns (nil, false) for every AST-backed File, so a caller takes its
// existing AST walk as the fallback. It lets the line-length rule serve
// its per-heading-limit line set from the classifier without navigating a
// (nil) AST on the flat path.
func FlatHeadingLines(f *File) (map[int]struct{}, bool) {
	if f.lineClass == nil {
		return nil, false
	}
	return f.lineClass.HeadingLines(), true
}

// ClassifyLines runs the single forward pass over lines (each a raw line
// with no trailing newline, as produced by bytes.Split) and returns the
// classification. It tracks fenced-code state, indented-code runs, and a
// small blockquote/list container stack so fences nested one or two
// containers deep are detected exactly as goldmark's block parser places
// them — the equivalence gate (plan 2606142147) pins this against the
// AST set across the neutral corpus and the line-length fixtures.
func ClassifyLines(lines [][]byte) *LineClassifier {
	p := &lc0Pass{lines: lines, out: &LineClassifier{classes: make([]LineClass, len(lines))}}
	p.run()
	return p.out
}

// lc0Container is one open block container on the pass's stack: a
// blockquote (its continuation prefix is `[ ]{0,3}>[ ]?`) or a list item
// (its continuation prefix is up to width spaces, or a blank line).
type lc0Container struct {
	blockquote bool
	width      int // list-item content width to consume; 0 for blockquote
}

// lc0Pass carries the mutable state of one ClassifyLines walk.
type lc0Pass struct {
	lines [][]byte
	out   *LineClassifier
	stack []lc0Container

	inFence       bool
	fenceChar     byte
	fenceLen      int
	fenceHadInfo  bool
	fenceOpenLine int // 1-based

	inHTML  bool   // inside a marker-terminated HTML block
	htmlEnd []byte // the closing string that ends the current HTML block

	prevParagraph bool // previous emitted line was paragraph text (setext gate)
	indentCode    bool // currently inside an indented code run
}

// run walks every line, handling the optional leading front matter first,
// then classifying each remaining line.
func (p *lc0Pass) run() {
	start := p.scanFrontMatter()
	for i := start; i < len(p.lines); i++ {
		p.classifyLine(i)
	}
	if p.inFence {
		p.finishFence(0) // unclosed fence: runs to EOF
	}
}

// scanFrontMatter consumes a leading `---` fenced YAML block when the very
// first line is exactly `---`, recording its bounds and marking the lines
// LineFrontMatter. Returns the index of the first line past it (0 when
// absent). Mirrors lint.StripFrontMatter's leading-delimiter rule so a
// whole-file classification agrees with the stripped engine path.
func (p *lc0Pass) scanFrontMatter() int {
	if len(p.lines) == 0 || !bytes.Equal(bytes.TrimRight(p.lines[0], " \t"), fmDelim) {
		return 0
	}
	for i := 1; i < len(p.lines); i++ {
		if bytes.Equal(bytes.TrimRight(p.lines[i], " \t"), fmDelim) {
			p.out.fmFrom, p.out.fmTo = 1, i+1
			for j := 0; j <= i; j++ {
				p.out.classes[j] = LineFrontMatter
			}
			return i + 1
		}
	}
	return 0 // no closing delimiter: not front matter
}

var fmDelim = []byte("---")

// classifyLine classifies the 1-based line i+1, threading container and
// fence state. It is the per-line core of the pass.
func (p *lc0Pass) classifyLine(i int) {
	line := p.lines[i]
	ln := i + 1
	off, _ := p.consumeContainers(line)
	rest := line[off:]

	if p.inFence {
		p.handleFenceBody(ln, rest)
		return
	}
	if p.inHTML {
		p.handleHTMLBody(ln, rest)
		return
	}
	if isBlankBytes(rest) {
		p.out.classes[i] = LineBlank
		p.prevParagraph = false
		p.indentCode = false
		// A blank line ends a paragraph but does not close list items
		// (CommonMark allows interior blanks); blockquotes that fail to
		// match were already popped by consumeContainers above.
		return
	}
	p.handleContent(i, ln, line, off, rest)
}

// handleFenceBody classifies a line while a fence is open: a matching
// close fence ends the block, anything else is an in-code content line.
func (p *lc0Pass) handleFenceBody(ln int, rest []byte) {
	p.out.classes[ln-1] = LineInCode
	if isFenceClose(rest, p.fenceChar, p.fenceLen) {
		p.out.classes[ln-1] = LineFenceClose
		p.finishFence(ln)
		p.inFence = false
	}
}

// handleHTMLBody classifies a line inside a marker-terminated HTML block:
// every line is LineHTML (never code), and the block ends on the line that
// contains its closing string.
func (p *lc0Pass) handleHTMLBody(ln int, rest []byte) {
	p.out.classes[ln-1] = LineHTML
	if bytes.Contains(rest, p.htmlEnd) {
		p.inHTML = false
		p.htmlEnd = nil
	}
}

// tryStartHTML opens a marker-terminated HTML block (CommonMark types
// 1–5) when rest begins one. It marks the start line LineHTML and, unless
// the same line already carries the closing string, enters HTML-block mode
// so the interior — which may hold blank then indented lines a fence-only
// scanner would misread as indented code — is classified LineHTML, not
// code. Returns false when rest opens no such block.
func (p *lc0Pass) tryStartHTML(i int, rest []byte) bool {
	end, ok := htmlBlockEnd(rest)
	if !ok {
		return false
	}
	p.out.classes[i] = LineHTML
	if !bytes.Contains(rest, end) {
		p.inHTML = true
		p.htmlEnd = end
	}
	return true
}

// handleContent classifies a non-blank, non-fence-body line: it opens new
// containers, then dispatches to the first matching block class.
func (p *lc0Pass) handleContent(i, ln int, line []byte, off int, rest []byte) {
	// New containers (blockquote / list item) may open before the block
	// content; push them and re-resolve the inner content slice.
	rest = p.openContainers(line, off, rest)
	indent := indentColumns(rest)

	switch {
	case indent <= 3 && isATXHeading(rest):
		p.out.classes[i] = LineATXHeading
		p.addHeading(ln)
		p.prevParagraph = false
		p.indentCode = false
	case indent <= 3 && p.tryOpenFence(ln, rest):
		// tryOpenFence set inFence and recorded the open line.
		p.prevParagraph = false
		p.indentCode = false
	case indent <= 3 && p.tryStartHTML(i, rest):
		// tryStartHTML marked the line LineHTML and set inHTML when the
		// block spans more lines.
		p.prevParagraph = false
		p.indentCode = false
	case p.canStartIndentedCode(indent):
		p.out.classes[i] = LineInCode
		p.markCode(ln)
		p.indentCode = true
	case indent <= 3 && p.prevParagraph && isSetextUnderline(rest):
		p.out.classes[i] = LineSetextUnderline
		p.addHeading(ln)
		p.addHeading(ln - 1) // the paragraph line it underlines is the heading
		p.prevParagraph = false
	default:
		p.out.classes[i] = LineParagraph
		p.prevParagraph = true
		p.indentCode = false
	}
}

// openContainers pushes any blockquote / list-item markers that begin at
// off and returns the inner content slice. It loops so a line like `> - x`
// opens both the quote and the list before the caller classifies `x`.
func (p *lc0Pass) openContainers(line []byte, off int, rest []byte) []byte {
	for {
		if w := blockquoteMarker(rest); w > 0 {
			p.stack = append(p.stack, lc0Container{blockquote: true})
			off += w
			rest = line[off:]
			p.prevParagraph = false
			continue
		}
		if w := listMarkerWidth(rest); w > 0 {
			p.stack = append(p.stack, lc0Container{width: w})
			off += w
			rest = line[off:]
			p.prevParagraph = false
			continue
		}
		return rest
	}
}

// consumeContainers advances past the continuation prefixes of every open
// container on the stack, popping the ones that no longer match (so a
// dedented line closes the lists/quotes it left). It returns the offset of
// the first content byte and the number of containers that continued.
func (p *lc0Pass) consumeContainers(line []byte) (offset, matched int) {
	pos := 0
	for _, c := range p.stack {
		next, ok := c.consume(line, pos)
		if !ok {
			p.stack = p.stack[:matched]
			return pos, matched
		}
		pos = next
		matched++
	}
	return pos, matched
}

// consume advances past one container's continuation prefix from pos,
// reporting whether it matched. A blank line continues a list item (CommonMark
// allows blank lines inside list items) but not a blockquote.
func (c lc0Container) consume(line []byte, pos int) (int, bool) {
	if c.blockquote {
		j, sp := pos, 0
		for j < len(line) && line[j] == ' ' && sp < 3 {
			j++
			sp++
		}
		if j < len(line) && line[j] == '>' {
			j++
			if j < len(line) && line[j] == ' ' {
				j++
			}
			return j, true
		}
		return pos, false
	}
	if isBlankFrom(line, pos) {
		return pos, true
	}
	j, sp := pos, 0
	for j < len(line) && line[j] == ' ' && sp < c.width {
		j++
		sp++
	}
	if sp == c.width {
		return j, true
	}
	return pos, false
}

// tryOpenFence opens a fenced code block when rest is an opening fence.
// It records the open line so finishFence can mark the block as a unit.
func (p *lc0Pass) tryOpenFence(ln int, rest []byte) bool {
	ch, n, hadInfo, ok := detectFenceOpen(rest)
	if !ok {
		return false
	}
	p.inFence = true
	p.fenceChar = ch
	p.fenceLen = n
	p.fenceHadInfo = hadInfo
	p.fenceOpenLine = ln
	p.out.classes[ln-1] = LineFenceOpen
	return true
}

// finishFence marks the just-closed (or EOF-terminated) fenced block's
// line set, mirroring lint.addFencedCodeBlockLines byte-for-byte —
// including goldmark's quirk that an empty fence with no info string
// contributes no lines. closeLine is the 1-based close-fence line, or 0
// when the fence runs to EOF unclosed.
func (p *lc0Pass) finishFence(closeLine int) {
	o := p.fenceOpenLine
	contentTo := len(p.lines)
	switch {
	case closeLine > 0:
		contentTo = closeLine - 1
	case len(p.lines) > 0 && len(p.lines[len(p.lines)-1]) == 0:
		// Unclosed fence: the trailing empty element bytes.Split yields
		// for the file's final newline is not a line in goldmark's model,
		// so it is not a content line. Excluding it reproduces goldmark's
		// empty-unclosed-fence quirk (no info, no content -> no lines).
		contentTo = len(p.lines) - 1
	}
	hasContent := contentTo >= o+1
	if !hasContent && !p.fenceHadInfo {
		return // goldmark exposes no source position for this empty fence
	}
	p.markCode(o)
	for k := o + 1; k <= contentTo; k++ {
		p.markCode(k)
	}
	cl := 0
	if hasContent {
		cl = contentTo + 1
	} else if o > 0 {
		cl = o + 1
	}
	if cl >= 1 && cl <= len(p.lines) {
		p.markCode(cl)
	}
}

// canStartIndentedCode reports whether the current line begins or
// continues an indented code block: four or more indent columns, not
// continuing or interrupting a paragraph. An indented code run already in
// progress continues on any ≥4-column line.
func (p *lc0Pass) canStartIndentedCode(indent int) bool {
	if indent < 4 {
		return false
	}
	if p.indentCode {
		return true
	}
	return !p.prevParagraph
}

// markCode records ln in the code-block set and sets its class to in-code
// when no fence-delimiter class was already assigned for that line.
func (p *lc0Pass) markCode(ln int) {
	if p.out.codeBlock == nil {
		p.out.codeBlock = make(map[int]struct{}, 16)
	}
	p.out.codeBlock[ln] = struct{}{}
}

// addHeading records ln in the heading-line set. Callers only pass a
// 1-based line that exists (an ATX line, or a setext underline and the
// title line above it, both ≥ 1), so no lower-bound guard is needed.
func (p *lc0Pass) addHeading(ln int) {
	if p.out.heading == nil {
		p.out.heading = make(map[int]struct{}, 8)
	}
	p.out.heading[ln] = struct{}{}
}
