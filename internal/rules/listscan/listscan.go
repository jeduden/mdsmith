// Package listscan parses Markdown list structure directly from raw
// source lines, with no goldmark AST. It exists for the Layer-0
// ("nil-AST") lint path: when the engine skips the goldmark parse,
// rules MDS014, MDS016, MDS045, MDS046, and MDS061 can no longer walk
// *ast.List / *ast.ListItem nodes, so this package re-derives the same
// facts they need (marker line, nesting level, list ordered-ness, list
// Start value, per-item literal number, and whether an item is
// multi-block) from f.Lines alone.
//
// The block-level Layer-0 scanner (internal/lint/layer0.go) is not
// sufficient: it collapses a whole list into one single-line BlockList
// span per marker line, carries no nesting depth, and misclassifies a
// >=4-space-indented nested item as a paragraph. listscan is a proper
// line-based list parser whose output is validated byte-for-byte against
// goldmark's AST in listscan_ast_test.go and over the whole repository
// corpus in listscan_corpus_test.go.
//
// Known limitation: HTML blocks are not modeled. goldmark treats an HTML
// block (a comment, a `<div>` … `</div>` run, etc.) as a leaf block whose
// interior is raw and opaque, so a bullet- or number-shaped line inside one
// is not a list item, and an HTML block nested in a list item counts as a
// separate block (making the item multi-block). listscan does not recognize
// HTML-block boundaries, so it would diverge on such input. The parse-skip
// gate excludes files containing an HTML block, keeping them off this path;
// the corpus equivalence test skips them for the same reason. Lifting the
// limitation means porting goldmark's seven HTML-block open/close rules
// (already in internal/lint/layer0.go) into this parser.
package listscan

// Item is one parsed list item.
type Item struct {
	// Line is the 1-based source line of the item's marker.
	Line int
	// Level is the nesting level: 0 for a top-level item, 1 for an item
	// inside a once-nested list, and so on. It matches the count of
	// *ast.ListItem ancestors in goldmark's tree.
	Level int
	// Ordered reports whether the item belongs to an ordered list.
	Ordered bool
	// Number is the literal ordered number written on the marker line
	// (ordered items only; 0 for bullets).
	Number int
	// Marker is the marker byte: '-', '*', or '+' for bullets; '.' or
	// ')' for ordered items.
	Marker byte
	// MultiBlock reports whether the item contains more than one block
	// child, matching goldmark's isMultiItem (ListItem.ChildCount > 1).
	MultiBlock bool
}

// List is one parsed list: a maximal run of sibling items at the same
// nesting level with the same ordered-ness.
type List struct {
	// Ordered reports whether this is an ordered list.
	Ordered bool
	// Start is the list's start value: for an ordered list, the literal
	// number of its first item (matching goldmark list.Start); 0 for an
	// unordered list.
	Start int
	// Depth is the number of *ast.List ancestors, which equals the Level
	// of the list's items.
	Depth int
	// FirstLine is the 1-based source line of the first item's marker.
	FirstLine int
	// LastLine is the 1-based source line of the list's last content
	// line (including continuation and nested-list lines).
	LastLine int
	// TopLevel reports whether the list has no *ast.ListItem ancestor.
	TopLevel bool
	// Items holds the list's direct child items in document order.
	Items []Item
}

// Parse scans lines and returns every list in document order plus a flat
// slice of every item in document order. The flat item slice is built
// from the lists' final Items, so the MultiBlock and Number values it
// carries are the finalized ones.
func Parse(lines [][]byte) (lists []List, items []Item) {
	p := &parser{lines: lines}
	p.run()
	flat := make([]Item, 0, p.itemCount)
	for _, l := range p.lists {
		flat = append(flat, l.Items...)
	}
	return p.lists, sortByLine(flat)
}

// sortByLine orders items by their marker line. Lists are recorded in
// document order, but a nested list's items are appended when the nested
// list closes, which can place them after a later sibling's items; a
// stable sort by line restores document order for the flat slice.
func sortByLine(items []Item) []Item {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j-1].Line > items[j].Line; j-- {
			items[j-1], items[j] = items[j], items[j-1]
		}
	}
	return items
}

// frame is one open list item on the parse stack.
type frame struct {
	// listIndex is the index in p.lists of the list this item belongs to.
	listIndex int
	// itemIndex is the index in that list's Items slice.
	itemIndex int
	// contentCol is the column where the item's content begins: indent +
	// marker width + spaces after the marker. A following line nests
	// inside this item when its indent is >= contentCol.
	contentCol int
	// blockCount counts the item's block children, mirroring goldmark's
	// ChildCount used by isMultiItem.
	blockCount int
	// pendingBlank records a blank line seen while this item is open but
	// not yet followed by a continuation; it makes the next content line
	// start a new block child.
	pendingBlank bool
	// inParagraph records whether the previous line attributed to this
	// item was paragraph text, so a following text line at lower indent
	// can lazily continue it.
	inParagraph bool
	// childListIndex is the index of the open child list nested directly
	// under this item, or -1 when none is open. It lets a following
	// nested marker rejoin the same child list instead of starting a new
	// one.
	childListIndex int
	// emptyLine is true while the item has no recorded source line yet
	// (an empty marker line whose content has not arrived). The first
	// continuation line that attaches content sets the item's Line.
	emptyLine bool
}

type parser struct {
	lines [][]byte
	lists []List
	// itemCount counts appended items for sizing the flat slice in Parse.
	itemCount int
	// stack holds the currently open list items, outermost first.
	stack []frame
	// blankRun counts consecutive blank lines pending before the current
	// line.
	blankRun int
	// topListIndex is the index of the open top-level list, or -1 when
	// none is open (a non-list block closed the run).
	topListIndex int
	// topInParagraph records whether the previous top-level (stack-empty)
	// line opened or continued a paragraph with no intervening blank. It
	// lets markerIsLazyText apply CommonMark's "an ordered list whose first
	// number is not 1 cannot interrupt a paragraph" rule at the document
	// root, not only inside a list item.
	topInParagraph bool
}

func (p *parser) run() {
	p.topListIndex = -1
	for i := 0; i < len(p.lines); i++ {
		if i == len(p.lines)-1 && len(p.lines[i]) == 0 {
			// Trailing empty element from a source ending in newline.
			break
		}
		line := p.lines[i]
		if isBlankLine(line) {
			p.blankRun++
			// A blank line closes any open top-level paragraph, so a marker
			// after it interrupts nothing and starts a list.
			p.topInParagraph = false
			continue
		}
		i = p.scanLine(i, line)
		p.blankRun = 0
	}
}

// scanLine processes the line at 0-based index i and returns the index of
// the last line it consumed (i for a single line; a higher index when it
// consumed a fenced code block's interior). It closes any open item the
// line's indent does not reach, then dispatches to a fenced-code block, a
// new list item, or a continuation line.
func (p *parser) scanLine(i int, line []byte) int {
	lineNo := i + 1
	indent := leadingSpaces(line)
	markerToken := hasMarkerToken(line, indent)
	interrupts := interruptsParagraph(line, indent)

	// Close any open item whose content column the line's indent does not
	// reach, so the surviving stack top is the item this line belongs to.
	// A lazy paragraph continuation at lower indent keeps the innermost
	// paragraph item open; a marker or paragraph-interrupting line is
	// never lazy and closes down to its true level.
	lazy := !markerToken && !interrupts
	for len(p.stack) > 0 && indent < p.stack[len(p.stack)-1].contentCol {
		if lazy && p.blankRun == 0 && p.stack[len(p.stack)-1].inParagraph {
			break
		}
		p.popFrame()
	}

	baseCol := 0
	if n := len(p.stack); n > 0 {
		baseCol = p.stack[n-1].contentCol
	}

	// A fenced code block opens when the fence run sits no more than 3
	// columns past the parent content column (same indent budget a marker
	// gets). Detect it relative to baseCol so a fence nested inside a list
	// item — whose absolute indent is the item's content column — is still
	// recognized.
	if fence, ok := openingFenceRel(line, indent, baseCol); ok {
		// A fenced code block is not a paragraph, so a marker after it (once
		// the block closes) interrupts nothing.
		p.topInParagraph = false
		return p.consumeFence(i, fence)
	}

	if mi, ok := parseMarker(line, indent, baseCol); ok && !p.markerIsLazyText(indent, mi) {
		// Opening a list ends any top-level paragraph; the document root is
		// now in list context.
		p.topInParagraph = false
		p.handleMarkerLine(lineNo, mi)
		return i
	}
	p.handleContinuation(lineNo, indent)
	// Track top-level paragraph state for the next line's interruption test:
	// when this line lands at the document root, a plain-text line opens or
	// continues a paragraph while a heading or thematic break does not.
	if len(p.stack) == 0 {
		p.topInParagraph = !interrupts
	}
	return i
}

// consumeFence handles a fenced code block opening at 0-based index open.
// The closing/attribution decision has already been made by scanLine, so
// the surviving stack top (if any) owns the block. The interior and
// closing fence are skipped so their bytes are never read as list
// markers, while the containing list's LastLine extends to the last
// content line (never the closing fence, matching goldmark's
// FencedCodeBlock.Lines). It returns the 0-based index of the block's
// last line.
func (p *parser) consumeFence(open int, fence fenceInfo) int {
	openLine := open + 1
	if len(p.stack) > 0 {
		top := &p.stack[len(p.stack)-1]
		top.blockCount++
		top.pendingBlank = false
		if top.blockCount > 1 {
			p.lists[top.listIndex].Items[top.itemIndex].MultiBlock = true
		}
		top.inParagraph = false
		p.bumpAncestors(openLine)
	} else {
		p.topListIndex = -1
	}

	i := open + 1
	for i < len(p.lines) {
		if i == len(p.lines)-1 && len(p.lines[i]) == 0 {
			break
		}
		if closingFence(p.lines[i], fence) {
			return i
		}
		if len(p.stack) > 0 {
			p.bumpAncestors(i + 1)
		}
		i++
	}
	return i - 1
}

// markerIsLazyText reports whether a recognized marker must be absorbed
// as paragraph text rather than open a list item. Per CommonMark an
// ordered list whose first number is not 1 cannot interrupt a paragraph:
// when the marker would nest inside (or continue) an item whose current
// block is an open paragraph with no intervening blank line, an ordered
// marker numbered other than 1 is lazy text, not a new sublist. Bullets
// and ordered markers numbered 1 always interrupt.
func (p *parser) markerIsLazyText(indent int, mi markerInfo) bool {
	if !mi.ordered || mi.number == 1 {
		return false
	}
	if p.blankRun > 0 {
		return false
	}
	n := len(p.stack)
	if n == 0 {
		// At the document root the marker is lazy paragraph text when an open
		// top-level paragraph precedes it with no blank between.
		return p.topInParagraph
	}
	top := p.stack[n-1]
	if !top.inParagraph {
		return false
	}
	// The marker continues/nests under this item only when its indent
	// reaches the item's content column; a shallower marker is a sibling
	// or closes the item and does interrupt.
	return indent >= top.contentCol
}

// interruptsParagraph reports whether line begins a block that interrupts
// an open paragraph, so it cannot be a lazy continuation. It covers the
// constructs CommonMark lets interrupt a paragraph and that the list
// rules' corpus exercises: ATX headings, fenced-code openers, and
// thematic breaks. (Blank lines are handled by the caller; HTML blocks
// and block quotes are out of scope for the list corpus.)
func interruptsParagraph(line []byte, indent int) bool {
	if indent >= 4 || indent >= len(line) {
		return false
	}
	if isThematicBreak(line) {
		return true
	}
	if line[indent] == '#' {
		j := indent
		for j < len(line) && line[j] == '#' {
			j++
		}
		level := j - indent
		if level >= 1 && level <= 6 &&
			(j >= len(line) || line[j] == ' ' || line[j] == '\t' || line[j] == '\r') {
			return true
		}
	}
	if c := line[indent]; c == '`' || c == '~' {
		j := indent
		for j < len(line) && line[j] == c {
			j++
		}
		if j-indent >= 3 {
			return true
		}
	}
	return false
}

// hasMarkerToken reports whether the line carries a bullet or ordered
// list-item token at indent, ignoring the indent-relative-to-parent
// constraint that parseMarker enforces. It lets scanLine treat a marker
// line as never-lazy when deciding which open items to close.
func hasMarkerToken(line []byte, indent int) bool {
	if indent >= len(line) {
		return false
	}
	c := line[indent]
	switch c {
	case '-', '*', '+':
		if isThematicBreak(line) {
			return false
		}
		j := indent + 1
		return j >= len(line) || line[j] == ' ' || line[j] == '\t' || line[j] == '\r'
	}
	if c >= '0' && c <= '9' {
		_, ok := orderedInfo(line, indent)
		return ok
	}
	return false
}

// markerInfo describes a recognized list-item marker on a line.
type markerInfo struct {
	ordered    bool
	number     int
	marker     byte
	contentCol int
	// empty reports that the marker line carries no content after the
	// marker. goldmark gives such an item no source line of its own, so
	// its recorded Line is 0 until a continuation line attaches content;
	// the five rules skip line-0 items, matching firstLineOfListItem
	// returning 0.
	empty bool
}

// handleMarkerLine opens a new list item. scanLine has already closed any
// inner open items the marker's indent did not reach, so the surviving
// stack top (if any) is this item's parent.
func (p *parser) handleMarkerLine(lineNo int, mi markerInfo) {
	level := len(p.stack)
	startsNewChildList := false
	if level > 0 {
		// A nested marker that starts a NEW child list adds a block child
		// to its parent item (the parent now holds a paragraph/text block
		// plus this sub-list, or two sub-lists). Joining an existing
		// sibling child list does not.
		if _, joins := p.siblingList(level, mi); !joins {
			startsNewChildList = true
		}
	}

	listIndex, itemIndex := p.appendItem(lineNo, level, mi)

	if level > 0 {
		parent := &p.stack[level-1]
		// A nested item closes the parent's open paragraph: the parent now
		// holds a child list, so a later ordered marker reaching the parent
		// interrupts that list context rather than continuing a paragraph.
		parent.inParagraph = false
		if startsNewChildList {
			parent.blockCount++
			if parent.blockCount > 1 {
				p.lists[parent.listIndex].Items[parent.itemIndex].MultiBlock = true
			}
		}
	}

	blockCount := 1
	if mi.empty {
		blockCount = 0
	}
	p.stack = append(p.stack, frame{
		listIndex:      listIndex,
		itemIndex:      itemIndex,
		contentCol:     mi.contentCol,
		blockCount:     blockCount,
		inParagraph:    !mi.empty,
		childListIndex: -1,
		emptyLine:      mi.empty,
	})
}

// appendItem records a new item at the given nesting level, joining the
// current sibling list when one is open or starting a new list. It
// returns the list and item indices.
func (p *parser) appendItem(lineNo, level int, mi markerInfo) (int, int) {
	// An empty marker line has no source line of its own in goldmark, so
	// the item's Line stays 0 until a continuation attaches content.
	itemLine := lineNo
	if mi.empty {
		itemLine = 0
	}
	// An empty marker line yields no source line, so the rules never read
	// its literal number; mirror that by reporting Number 0 for it.
	number := mi.number
	if mi.empty {
		number = 0
	}
	item := Item{
		Line:    itemLine,
		Level:   level,
		Ordered: mi.ordered,
		Number:  number,
		Marker:  mi.marker,
	}
	p.itemCount++

	if li, ok := p.siblingList(level, mi); ok {
		p.lists[li].Items = append(p.lists[li].Items, item)
		p.refreshFirstLine(li)
		if !mi.empty {
			p.lists[li].LastLine = lineNo
			p.bumpAncestors(lineNo)
		}
		return li, len(p.lists[li].Items) - 1
	}

	l := List{
		Ordered:   mi.ordered,
		Depth:     level,
		FirstLine: itemLine,
		LastLine:  itemLine,
		TopLevel:  level == 0,
		Items:     []Item{item},
	}
	if mi.ordered {
		l.Start = mi.number
	}
	li := len(p.lists)
	p.lists = append(p.lists, l)
	if level == 0 {
		p.topListIndex = li
	} else {
		p.stack[level-1].childListIndex = li
	}
	p.refreshFirstLine(li)
	if !mi.empty {
		p.bumpAncestors(lineNo)
	}
	return li, 0
}

// refreshFirstLine sets a list's FirstLine to its first item that carries
// a positive source line, matching goldmark's lineOfNode, which descends
// to the first child block with a source position. A list whose items are
// all empty keeps FirstLine at 0.
func (p *parser) refreshFirstLine(li int) {
	for _, it := range p.lists[li].Items {
		if it.Line > 0 {
			p.lists[li].FirstLine = it.Line
			return
		}
	}
	p.lists[li].FirstLine = 0
}

// siblingList returns the open list a new marker at this level should
// join, and false when a new list must start. goldmark starts a new list
// when the marker delimiter changes: a different bullet character
// (`-`/`*`/`+`) or a different ordered delimiter (`.` vs `)`) splits the
// run, as does an ordered/unordered switch.
func (p *parser) siblingList(level int, mi markerInfo) (int, bool) {
	var li int
	if level == 0 {
		li = p.topListIndex
	} else {
		li = p.stack[level-1].childListIndex
	}
	if li < 0 {
		return 0, false
	}
	l := p.lists[li]
	if l.Ordered != mi.ordered {
		return 0, false
	}
	if l.Items[0].Marker != mi.marker {
		return 0, false
	}
	return li, true
}

// handleContinuation attributes a non-marker, non-blank line to the
// innermost open item. scanLine has already closed every item the line's
// indent does not reach (respecting lazy paragraph continuation), so an
// empty stack here means the line interrupts the top-level list run.
func (p *parser) handleContinuation(lineNo, indent int) {
	if len(p.stack) == 0 {
		p.topListIndex = -1
		return
	}

	top := &p.stack[len(p.stack)-1]
	if top.emptyLine {
		// First content for a previously-empty item: record its source
		// line and start its block count at one.
		top.emptyLine = false
		p.lists[top.listIndex].Items[top.itemIndex].Line = lineNo
		p.refreshFirstLine(top.listIndex)
		top.blockCount = 1
		top.pendingBlank = false
		top.inParagraph = true
		p.bumpAncestors(lineNo)
		return
	}
	lazyCont := indent < top.contentCol && p.blankRun == 0 && top.inParagraph
	if p.blankRun > 0 {
		top.pendingBlank = true
	}
	// A new block child opens when a blank separated this line from the
	// item's previous content (paragraph break), or when the previous
	// content was not a lazily-continuable paragraph. A lazy continuation
	// of the same paragraph never opens a new block.
	if !lazyCont && (top.pendingBlank || !top.inParagraph) {
		top.blockCount++
		top.pendingBlank = false
		if top.blockCount > 1 {
			p.lists[top.listIndex].Items[top.itemIndex].MultiBlock = true
		}
	}
	top.inParagraph = true
	p.bumpAncestors(lineNo)
}

// bumpAncestors extends LastLine on every open ancestor list (and the
// top-level list) to lineNo.
func (p *parser) bumpAncestors(lineNo int) {
	for i := range p.stack {
		li := p.stack[i].listIndex
		if lineNo > p.lists[li].LastLine {
			p.lists[li].LastLine = lineNo
		}
	}
}

// popFrame removes the innermost open item and clears its parent's
// open-child-list pointer, since a closed item's child list cannot be
// rejoined.
func (p *parser) popFrame() {
	p.stack = p.stack[:len(p.stack)-1]
}

// parseMarker recognizes a list-item marker at the given indent. baseCol
// is the content column of the enclosing open item (0 at the top level):
// goldmark allows a marker to sit up to 3 columns past its parent's
// content column, so the indent-relative-to-baseCol must be < 4 for the
// line to open an item rather than continue indented code.
func parseMarker(line []byte, indent, baseCol int) (markerInfo, bool) {
	if indent-baseCol >= 4 || indent >= len(line) {
		return markerInfo{}, false
	}
	c := line[indent]
	switch c {
	case '-', '*', '+':
		if isThematicBreak(line) {
			return markerInfo{}, false
		}
		j := indent + 1
		if j < len(line) && line[j] != ' ' && line[j] != '\t' && line[j] != '\r' {
			return markerInfo{}, false
		}
		return markerInfo{
			marker:     c,
			contentCol: contentColumn(line, indent+1),
			empty:      markerLineEmpty(line, indent+1),
		}, true
	}
	if c >= '0' && c <= '9' {
		return orderedInfo(line, indent)
	}
	return markerInfo{}, false
}

// markerLineEmpty reports whether the marker line carries no content
// after the marker (only whitespace through end of line).
func markerLineEmpty(line []byte, afterMarker int) bool {
	for i := afterMarker; i < len(line); i++ {
		if line[i] != ' ' && line[i] != '\t' && line[i] != '\r' {
			return false
		}
	}
	return true
}

func orderedInfo(line []byte, indent int) (markerInfo, bool) {
	j := indent
	digits := 0
	for j < len(line) && line[j] >= '0' && line[j] <= '9' {
		j++
		digits++
	}
	if digits == 0 || digits > 9 {
		return markerInfo{}, false
	}
	if j >= len(line) || (line[j] != '.' && line[j] != ')') {
		return markerInfo{}, false
	}
	marker := line[j]
	end := j + 1
	if end < len(line) && line[end] != ' ' && line[end] != '\t' && line[end] != '\r' {
		return markerInfo{}, false
	}
	return markerInfo{
		ordered:    true,
		number:     atoiBytes(line[indent : indent+digits]),
		marker:     marker,
		contentCol: contentColumn(line, end),
		empty:      markerLineEmpty(line, end),
	}, true
}

// contentColumn returns the column where an item's content begins given
// the byte index just past the marker. It counts the spaces (tabs as
// 4-column stops) after the marker, but caps padding at one column when 5
// or more spaces follow — 5+ spaces makes the remainder indented code, so
// goldmark uses a single space of marker padding. A marker with no
// following content also uses one column of padding.
func contentColumn(line []byte, afterMarker int) int {
	col := afterMarker
	spaces := 0
	for col < len(line) && (line[col] == ' ' || line[col] == '\t') {
		spaces++
		col++
	}
	if col >= len(line) || spaces == 0 || spaces >= 5 {
		return afterMarker + 1
	}
	return afterMarker + spaces
}

func leadingSpaces(line []byte) int {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	return i
}

func isBlankLine(line []byte) bool {
	for _, c := range line {
		if c != ' ' && c != '\t' && c != '\r' {
			return false
		}
	}
	return true
}

func atoiBytes(b []byte) int {
	n := 0
	for _, c := range b {
		n = n*10 + int(c-'0')
	}
	return n
}

// fenceInfo describes a fenced-code opening fence.
type fenceInfo struct {
	char   byte
	length int
	// baseCol is the content column of the item the fence opened inside
	// (0 at the top level). The closing fence is recognized relative to
	// this column, mirroring goldmark's container-relative fence parsing.
	baseCol int
}

// openingFenceRel parses line as a fenced-code opener relative to baseCol:
// the fence run must sit no more than 3 columns past baseCol, be a run of
// 3 or more identical “ ` “ or `~` characters, and (for backtick
// fences) carry no backtick in the info string. It mirrors goldmark's
// container-relative fence parsing so a fence nested inside a list item is
// recognized at its indented position.
func openingFenceRel(line []byte, indent, baseCol int) (fenceInfo, bool) {
	if indent-baseCol >= 4 || indent >= len(line) {
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
	if ch == '`' {
		for _, b := range line[j:] {
			if b == '`' {
				return fenceInfo{}, false
			}
		}
	}
	return fenceInfo{char: ch, length: length, baseCol: baseCol}, true
}

// closingFence reports whether line closes a fence opened with fi: its
// fence run sits no more than 3 columns past fi.baseCol, runs >= fi.length
// identical fence characters, and is followed only by whitespace.
func closingFence(line []byte, fi fenceInfo) bool {
	indent := leadingSpaces(line)
	if indent-fi.baseCol >= 4 {
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

// isThematicBreak reports whether line is a thematic break (3+ of a
// single -, *, or _ with only spaces between), which is not a list item.
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
