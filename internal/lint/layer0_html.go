package lint

import (
	"bytes"
	"regexp"
)

// allowedBlockTags is the CommonMark type-6 HTML block tag set, mirroring
// parser.allowedBlockTags (unexported there). A type-6 HTML block opens
// only on one of these tag names; the list must stay in sync with the
// goldmark fork so the Layer 0 scan classifies HTML blocks identically.
var allowedBlockTags = map[string]struct{}{
	"address": {}, "article": {}, "aside": {}, "base": {},
	"basefont": {}, "blockquote": {}, "body": {}, "caption": {},
	"center": {}, "col": {}, "colgroup": {}, "dd": {},
	"details": {}, "dialog": {}, "dir": {}, "div": {},
	"dl": {}, "dt": {}, "fieldset": {}, "figcaption": {},
	"figure": {}, "footer": {}, "form": {}, "frame": {},
	"frameset": {}, "h1": {}, "h2": {}, "h3": {}, "h4": {},
	"h5": {}, "h6": {}, "head": {}, "header": {}, "hr": {},
	"html": {}, "iframe": {}, "legend": {}, "li": {},
	"link": {}, "main": {}, "menu": {}, "menuitem": {},
	"meta": {}, "nav": {}, "noframes": {}, "ol": {},
	"optgroup": {}, "option": {}, "p": {}, "param": {},
	"search": {}, "section": {}, "summary": {}, "table": {},
	"tbody": {}, "td": {}, "tfoot": {}, "th": {}, "thead": {},
	"title": {}, "tr": {}, "track": {}, "ul": {},
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
	htmlType1Open  = regexp.MustCompile(`(?i)^[ ]{0,3}<(script|pre|style|textarea)(\s|>|/>|$)`)
	htmlType1Close = regexp.MustCompile(`(?i)</(script|pre|style|textarea)>`)
	htmlType6Open  = regexp.MustCompile(`^[ ]{0,3}</?([a-zA-Z][a-zA-Z0-9-]*)(\s|>|/>|$)`)
	htmlType7Open  = regexp.MustCompile(`^[ ]{0,3}<(/[ ]*)?[a-zA-Z][a-zA-Z0-9-]*(\s[^>]*)?[ ]*/?>[ \t\r]*$`)
)

// openHTMLBlock classifies line as an HTML block opener, returning the
// type (htmlNone when none). It mirrors the precedence in
// htmlBlockParser.Open: types 1–5 first, then type 7 (gated on an allowed
// or generic tag and unable to interrupt a paragraph), then type 6. The
// inParagraph flag suppresses type 7, which cannot interrupt a paragraph.
func openHTMLBlock(line []byte, inParagraph bool) htmlBlockType {
	// Every HTML-block opener is anchored `^[ ]{0,3}<`, so a line whose
	// first non-space byte (within the first 4 columns) is not `<` can
	// never open one. Gate the regexp battery on that cheap byte check so
	// ordinary prose lines — the overwhelming common case in the Layer 0
	// hot path — skip the regexp battery entirely.
	indent := leadingSpaces(line)
	if indent > 3 || indent >= len(line) || line[indent] != '<' {
		return htmlNone
	}
	rest := line[indent:]
	switch {
	case htmlType1Open.Match(line):
		return htmlType1
	case bytes.HasPrefix(rest, []byte("<!--")):
		return htmlType2
	case bytes.HasPrefix(rest, []byte("<?")):
		return htmlType3
	case len(rest) >= 3 && rest[1] == '!' && (rest[2] >= 'A' && rest[2] <= 'Z' || rest[2] >= 'a' && rest[2] <= 'z'):
		return htmlType4
	case bytes.HasPrefix(rest, []byte("<![CDATA[")):
		return htmlType5
	}
	if m := htmlType6Open.FindSubmatch(line); m != nil && tagInAllowedSet(m[1]) {
		return htmlType6
	}
	if !inParagraph && htmlType7Open.Match(line) && !type7TagIsRawText(line) {
		return htmlType7
	}
	return htmlNone
}

// tagName is a fixed-capacity stack buffer for an ASCII tag name, sized so
// the longest HTML tag (and longer non-tags, truncated harmlessly) fits
// without a heap allocation. lowerInto fills it.
type tagName struct {
	buf [32]byte
	n   int
}

// lowerInto copies the ASCII-lowercased bytes of b into the stack buffer,
// truncating anything past its capacity (a name that long is not in any
// recognised set, so truncation cannot cause a false match against the
// short tag names the sets contain).
func (t *tagName) lowerInto(b []byte) []byte {
	t.n = 0
	for _, c := range b {
		if t.n >= len(t.buf) {
			break
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		t.buf[t.n] = c
		t.n++
	}
	return t.buf[:t.n]
}

// tagInAllowedSet reports whether b (case-insensitively) is one of the
// type-6 HTML block tags, lowercasing into a stack buffer so the lookup
// allocates nothing.
func tagInAllowedSet(b []byte) bool {
	var t tagName
	_, ok := allowedBlockTags[string(t.lowerInto(b))]
	return ok
}

// type7TagIsRawText reports whether the type-7 opener's tag is one of the
// raw-text tags (script/style/pre/textarea) that are classified as type 1
// instead. It lowercases into a stack buffer to avoid an allocation.
func type7TagIsRawText(line []byte) bool {
	var t tagName
	switch string(t.lowerInto(type7TagBytes(line))) {
	case "script", "style", "pre", "textarea":
		return true
	}
	return false
}

// type7TagBytes returns the tag-name bytes of a type-7 HTML opener (the
// run of tag bytes after the `<` and any `/` close-tag slash), so the
// caller can fold and compare them without allocating.
func type7TagBytes(line []byte) []byte {
	i := leadingSpaces(line)
	if i < len(line) && line[i] == '<' {
		i++
	}
	for i < len(line) && (line[i] == '/' || line[i] == ' ') {
		i++
	}
	start := i
	for i < len(line) && isTagByte(line[i]) {
		i++
	}
	return line[start:i]
}

// isTagByte reports whether b can appear in an HTML tag name after the
// first letter.
func isTagByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '-'
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
		return bytes.IndexByte(line, '>') >= 0
	case htmlType5:
		return bytes.Contains(line, htmlClose5)
	}
	return false
}

var (
	htmlClose2 = []byte("-->")
	htmlClose3 = []byte("?>")
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
