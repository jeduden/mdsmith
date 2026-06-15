package lint

import (
	"bytes"
	"regexp"
)

// regexpMustCompile is a thin alias for regexp.MustCompile, kept so the
// package-scope HTML-block pattern table reads as a list of literals.
func regexpMustCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}

// allowedBlockTags is the CommonMark type-6 HTML block tag set, mirroring
// parser.allowedBlockTags (unexported there). A type-6 HTML block opens
// only on one of these tag names; the list must stay in sync with the
// goldmark fork so the Layer 0 scan classifies HTML blocks identically.
var allowedBlockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "base": true,
	"basefont": true, "blockquote": true, "body": true, "caption": true,
	"center": true, "col": true, "colgroup": true, "dd": true,
	"details": true, "dialog": true, "dir": true, "div": true,
	"dl": true, "dt": true, "fieldset": true, "figcaption": true,
	"figure": true, "footer": true, "form": true, "frame": true,
	"frameset": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "head": true, "header": true, "hr": true,
	"html": true, "iframe": true, "legend": true, "li": true,
	"link": true, "main": true, "menu": true, "menuitem": true,
	"meta": true, "nav": true, "noframes": true, "ol": true,
	"optgroup": true, "option": true, "p": true, "param": true,
	"search": true, "section": true, "summary": true, "table": true,
	"tbody": true, "td": true, "tfoot": true, "th": true, "thead": true,
	"title": true, "tr": true, "track": true, "ul": true,
}

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
	// classFrontMatter marks a line inside the leading YAML front matter.
	classFrontMatter
)

// BlockSpan is one Layer 0 block: its kind, its 1-based inclusive start
// and end source lines, and its block-quote/list nesting depth.
type BlockSpan struct {
	Kind  BlockKind
	Start int
	End   int
	Depth int
}

// Layer0Scan is the product of one forward pass over File.Lines: a compact
// per-line classification, the code-block and processing-instruction line
// sets, the block spans, and the front-matter bounds. It carries no node
// tree and is the only block-level projection source when a File was built
// with a nil AST (the parse-skipped path).
type Layer0Scan struct {
	// Classes holds one lineClass per source line, indexed by (line-1).
	Classes []lineClass
	// CodeBlockLines is the set of 1-based line numbers inside fenced or
	// indented code blocks, including fence lines.
	CodeBlockLines map[int]struct{}
	// PIBlockLines is the set of 1-based line numbers inside
	// processing-instruction blocks, including the opening and closing
	// marker lines.
	PIBlockLines map[int]struct{}
	// BlockSpans lists every block in document order.
	BlockSpans []BlockSpan
	// FrontMatterEnd is the 1-based last line of the leading YAML front
	// matter, or 0 when the document has none.
	FrontMatterEnd int
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

// run drives the forward pass: front matter first, then a block loop that
// dispatches on the line's leading construct.
func (s *scanner) run() {
	s.scanFrontMatter()
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

// scanFrontMatter consumes a leading YAML front-matter block delimited by
// a `---` line at the very top and the next `---` line. It mirrors
// markdown.StripFrontMatter: only a document that opens with exactly
// `---` carries front matter, and the block runs through the closing
// `---`. When no front matter is present, the cursor stays at line 0.
func (s *scanner) scanFrontMatter() {
	if len(s.lines) == 0 || !isFrontMatterDelim(s.lines[0]) {
		return
	}
	for j := 1; j < len(s.lines); j++ {
		if isFrontMatterDelim(s.lines[j]) {
			for k := 0; k <= j; k++ {
				s.l0.Classes[k] |= classFrontMatter
			}
			s.l0.FrontMatterEnd = j + 1
			s.i = j + 1
			return
		}
	}
}

// isFrontMatterDelim reports whether line is a front-matter fence: exactly
// "---" after trimming a trailing carriage return.
func isFrontMatterDelim(line []byte) bool {
	return bytes.Equal(bytes.TrimRight(line, "\r"), frontMatterDelim)
}

var frontMatterDelim = []byte("---")

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
	default:
		s.scanParagraph()
	}
}

// fenceInfo describes an opening fenced-code fence line.
type fenceInfo struct {
	char   byte
	indent int
	length int
	// hasInfo records whether the opening fence carries a non-empty info
	// string after the fence run. goldmark exposes no source position for
	// an info-less, content-less fence, so the projection emits no lines
	// for it — hasInfo drives that quirk.
	hasInfo bool
}

// openingFence parses line as a fenced-code opening fence, returning its
// data and ok=true when it qualifies: indent < 4, a run of >= 3 identical
// fence characters, and (for backtick fences) no backtick in the info
// string. Mirrors fencedCodeBlockParser.Open.
func openingFence(line []byte) (fenceInfo, bool) {
	indent := leadingSpaces(line)
	if indent >= 4 {
		return fenceInfo{}, false
	}
	if indent >= len(line) {
		return fenceInfo{}, false
	}
	ch := line[indent]
	if ch != '`' && ch != '~' {
		return fenceInfo{}, false
	}
	j := indent
	for j < len(line) && line[j] == ch {
		j++
	}
	length := j - indent
	if length < 3 {
		return fenceInfo{}, false
	}
	rest := line[j:]
	if ch == '`' && bytes.IndexByte(rest, '`') >= 0 {
		return fenceInfo{}, false
	}
	return fenceInfo{
		char:    ch,
		indent:  indent,
		length:  length,
		hasInfo: len(bytes.TrimSpace(rest)) > 0,
	}, true
}

// closingFence reports whether line closes a fence opened with fi: indent
// < 4, a run of >= fi.length identical fence characters, and only
// whitespace after the run. Mirrors fencedCodeBlockParser.Continue.
func closingFence(line []byte, fi fenceInfo) bool {
	indent := leadingSpaces(line)
	if indent >= 4 {
		return false
	}
	j := indent
	for j < len(line) && line[j] == fi.char {
		j++
	}
	if j-indent < fi.length {
		return false
	}
	return isBlankLine(line[j:])
}

// tryFence recognises a fenced code block at the cursor. It marks every
// line from the opening fence through the closing fence (or end of
// document for an unclosed fence) as code, records the span, and advances
// the cursor past it. Returns false when the cursor line is not a fence.
func (s *scanner) tryFence() bool {
	fi, ok := openingFence(s.lines[s.i])
	if !ok {
		return false
	}
	openLine := s.i // 0-based opening fence index
	// Scan content lines until a closing fence or EOF. The closing fence
	// is never a content line (goldmark closes before appending it).
	lastContent := 0 // 1-based; 0 means "no content lines"
	closed := false
	s.i++
	for s.i < len(s.lines) {
		if s.trailingEmptyLine(s.i) {
			break
		}
		if closingFence(s.lines[s.i], fi) {
			closed = true
			break
		}
		lastContent = s.i + 1
		s.i++
	}
	// goldmark exposes no source position for an info-less, content-less
	// fence, so addFencedCodeBlockLines emits nothing for it. Mirror that:
	// skip marking entirely when the fence has neither info nor content.
	if fi.hasInfo || lastContent > 0 {
		s.markCode(openLine)
		for ln := openLine + 2; ln <= lastContent; ln++ {
			s.markCode(ln - 1)
		}
		// Mirror addFencedCodeBlockLines: the closing fence is the line
		// after the last content line (or after the opening fence when
		// there were no content lines). For a closed fence that is the
		// matched line; for an unclosed fence it is a phantom line, marked
		// only when within bounds.
		closeLine := lastContent + 1
		if lastContent == 0 {
			closeLine = openLine + 2 // 0-based open +1 to 1-based, +1 next
		}
		if closeLine <= len(s.lines) {
			s.markCode(closeLine - 1)
		}
	}
	if closed {
		s.i++ // advance past the matched closing fence line
	}
	s.addSpan(BlockFencedCode, openLine, s.i-1, 0)
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

// htmlBlockType identifies which of CommonMark's seven HTML block kinds a
// line opens (0 = none). Each kind has a distinct closing condition, which
// htmlClose encodes.
type htmlBlockType int

const (
	htmlNone htmlBlockType = iota
	htmlType1
	htmlType2
	htmlType3
	htmlType4
	htmlType5
	htmlType6
	htmlType7
)

var (
	htmlType1Open  = regexpMustCompile(`(?i)^[ ]{0,3}<(script|pre|style|textarea)(\s|>|/>|$)`)
	htmlType1Close = regexpMustCompile(`(?i)</(script|pre|style|textarea)>`)
	htmlType2Open  = regexpMustCompile(`^[ ]{0,3}<!--`)
	htmlType3Open  = regexpMustCompile(`^[ ]{0,3}<\?`)
	htmlType4Open  = regexpMustCompile(`^[ ]{0,3}<![A-Za-z]`)
	htmlType5Open  = regexpMustCompile(`^[ ]{0,3}<!\[CDATA\[`)
	htmlType6Open  = regexpMustCompile(`^[ ]{0,3}</?([a-zA-Z][a-zA-Z0-9-]*)(\s|>|/>|$)`)
	htmlType7Open  = regexpMustCompile(`^[ ]{0,3}<(/[ ]*)?[a-zA-Z][a-zA-Z0-9-]*(\s[^>]*)?[ ]*/?>[ \t\r]*$`)
)

// openHTMLBlock classifies line as an HTML block opener, returning the
// type (htmlNone when none). It mirrors the precedence in
// htmlBlockParser.Open: types 1–5 first, then type 7 (gated on an allowed
// or generic tag and unable to interrupt a paragraph), then type 6. The
// inParagraph flag suppresses type 7, which cannot interrupt a paragraph.
func openHTMLBlock(line []byte, inParagraph bool) htmlBlockType {
	switch {
	case htmlType1Open.Match(line):
		return htmlType1
	case htmlType2Open.Match(line):
		return htmlType2
	case htmlType3Open.Match(line):
		return htmlType3
	case htmlType4Open.Match(line):
		return htmlType4
	case htmlType5Open.Match(line):
		return htmlType5
	}
	if m := htmlType6Open.FindSubmatch(line); m != nil && allowedBlockTags[lowerTag(m[1])] {
		return htmlType6
	}
	if !inParagraph && htmlType7Open.Match(line) {
		tag := type7Tag(line)
		if tag != "script" && tag != "style" && tag != "pre" && tag != "textarea" {
			return htmlType7
		}
	}
	return htmlNone
}

// type7Tag extracts the lowercased tag name from a type-7 HTML opener so
// the script/style/pre/textarea exclusion (those are type 1) can be
// applied.
func type7Tag(line []byte) string {
	i := leadingSpaces(line)
	if i < len(line) && line[i] == '<' {
		i++
	}
	for i < len(line) && (line[i] == '/' || line[i] == ' ') {
		i++
	}
	start := i
	for i < len(line) && (isTagByte(line[i])) {
		i++
	}
	return lowerTag(line[start:i])
}

// isTagByte reports whether b can appear in an HTML tag name after the
// first letter.
func isTagByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '-'
}

// lowerTag lowercases an ASCII tag name without allocating beyond the
// result string.
func lowerTag(b []byte) string {
	out := make([]byte, len(b))
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// htmlBlockCloses reports whether line closes an HTML block of the given
// type. Types 1–5 close on a line containing their terminator; types 6 and
// 7 close on the first blank line (handled by the caller, which stops the
// block before a blank line). The single-line-open close (a terminator on
// the opening line) is handled by the caller checking the opening line too.
func htmlBlockCloses(line []byte, t htmlBlockType) bool {
	switch t {
	case htmlType1:
		return htmlType1Close.Match(line)
	case htmlType2:
		return bytes.Contains(line, htmlClose2)
	case htmlType3:
		return bytes.Contains(line, htmlClose3)
	case htmlType4:
		return bytes.Contains(line, htmlClose4)
	case htmlType5:
		return bytes.Contains(line, htmlClose5)
	}
	return false
}

var (
	htmlClose2 = []byte("-->")
	htmlClose3 = []byte("?>")
	htmlClose4 = []byte(">")
	htmlClose5 = []byte("]]>")
)

// tryHTMLBlock recognises an HTML block at the cursor and consumes it,
// recording the span and advancing past it. Its interior is opaque to the
// code/PI/fence scanners, so an indented line inside an HTML comment is not
// mistaken for indented code. inParagraph suppresses type 7 (which cannot
// interrupt a paragraph). Returns false when the cursor line opens no HTML
// block.
func (s *scanner) tryHTMLBlock(inParagraph bool) bool {
	t := openHTMLBlock(s.lines[s.i], inParagraph)
	if t == htmlNone {
		return false
	}
	start := s.i
	closeOnTerminator := t >= htmlType1 && t <= htmlType5
	// Types 1–5 may close on their opening line.
	if closeOnTerminator && htmlBlockCloses(s.lines[s.i], t) {
		s.i++
		s.addSpan(BlockHTML, start, start, 0)
		s.prevNonBlankParagraph = false
		return true
	}
	s.i++
	for s.i < len(s.lines) {
		if s.trailingEmptyLine(s.i) {
			break
		}
		cur := s.lines[s.i]
		if closeOnTerminator {
			if htmlBlockCloses(cur, t) {
				s.i++
				break
			}
		} else if isBlankLine(cur) {
			// Types 6 and 7 close before the first blank line.
			break
		}
		s.i++
	}
	s.addSpan(BlockHTML, start, s.i-1, 0)
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
	line := s.lines[s.i]
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
	if j < len(line) && line[j] != ' ' && line[j] != '\t' && line[j] != '\r' {
		return false
	}
	start := s.i
	s.addSpan(BlockATXHeading, start, start, 0)
	s.i++
	s.prevNonBlankParagraph = false
	return true
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
	// goldmark trims trailing blank lines from the block, so only mark
	// through the last non-blank indented line as code.
	for k := start; k <= lastNonBlank; k++ {
		if isBlankLine(s.lines[k]) {
			s.l0.Classes[k] |= classBlank
			continue
		}
		s.markCode(k)
	}
	// Reset the cursor to just past the last code line; blank lines after
	// it are reclassified by the main loop.
	s.i = lastNonBlank + 1
	s.addSpan(BlockIndentedCode, start, lastNonBlank, 0)
	s.prevNonBlankParagraph = false
	return true
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
		c := line[j]
		switch {
		case c == ch:
			count++
		case c == ' ' || c == '\t' || c == '\r':
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
