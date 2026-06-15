package lint

import "bytes"

// leadingSpaces (the run of ASCII space characters at the start of a line)
// is shared with the Layer 0 scanner; its definition lives in layer0.go.

// indentColumns counts the leading whitespace run of b in columns,
// measured from absolute column startCol so that a tab advances to the
// correct four-column stop after a container prefix (the CommonMark
// indentation rule: a tab right after `> ` spans fewer columns than one at
// the line start). It returns the indent relative to startCol and stops at
// the first non-whitespace byte. Four or more columns can open an indented
// code block; ≤3 columns is the fence/heading/HTML start gate.
func indentColumns(b []byte, startCol int) int {
	col := startCol
	for _, c := range b {
		switch c {
		case ' ':
			col++
		case '\t':
			col += 4 - (col % 4)
		default:
			return col - startCol
		}
	}
	return col - startCol
}

// isBlankBytes reports whether b is empty or only spaces and tabs.
func isBlankBytes(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}

// isBlankFrom reports whether line is blank from pos onward.
func isBlankFrom(line []byte, pos int) bool {
	for i := pos; i < len(line); i++ {
		if line[i] != ' ' && line[i] != '\t' {
			return false
		}
	}
	return true
}

// isATXHeading reports whether rest (already at ≤3 indent) is an ATX
// heading: 1–6 `#` followed by a space, a tab, or end of line. Mirrors
// the CommonMark ATX rule the goldmark heading parser applies.
func isATXHeading(rest []byte) bool {
	i := leadingSpaces(rest)
	n := 0
	for i < len(rest) && rest[i] == '#' {
		i++
		n++
	}
	if n < 1 || n > 6 {
		return false
	}
	return i >= len(rest) || rest[i] == ' ' || rest[i] == '\t'
}

// lcIsSetextUnderline reports whether rest is a setext underline: a run of
// only `=` or only `-` (after ≤3 indent), optionally trailed by spaces.
// The caller gates this on the previous line being paragraph text. It is
// kept distinct from the Layer 0 scanner's isSetextUnderline (layer0.go),
// which takes a full unstripped line; this one operates on the indent-
// stripped rest the line classifier already holds.
func lcIsSetextUnderline(rest []byte) bool {
	i := leadingSpaces(rest)
	if i >= len(rest) {
		return false
	}
	c := rest[i]
	if c != '=' && c != '-' {
		return false
	}
	for i < len(rest) && rest[i] == c {
		i++
	}
	for ; i < len(rest); i++ {
		if rest[i] != ' ' && rest[i] != '\t' {
			return false
		}
	}
	return true
}

// detectFenceOpen reports whether rest opens a fenced code block, and if
// so returns the fence character, its length, and whether a non-empty
// info string follows. For backtick fences the info string may not
// contain a backtick (the CommonMark rule that lets inline code spans
// coexist with fences); tilde fences allow any info string.
func detectFenceOpen(rest []byte) (ch byte, length int, hasInfo, ok bool) {
	i := leadingSpaces(rest)
	if i > 3 || i >= len(rest) {
		return 0, 0, false, false
	}
	c := rest[i]
	if c != '`' && c != '~' {
		return 0, 0, false, false
	}
	n := 0
	for i < len(rest) && rest[i] == c {
		i++
		n++
	}
	if n < 3 {
		return 0, 0, false, false
	}
	info := rest[i:]
	if c == '`' && bytes.IndexByte(info, '`') >= 0 {
		return 0, 0, false, false
	}
	return c, n, len(bytes.TrimSpace(info)) > 0, true
}

// isFenceClose reports whether rest closes a fence of the given character
// and minimum length: a run of at least length fence characters (after
// ≤3 indent) followed only by spaces or tabs.
func isFenceClose(rest []byte, ch byte, length int) bool {
	i := leadingSpaces(rest)
	if i > 3 {
		return false
	}
	n := 0
	for i < len(rest) && rest[i] == ch {
		i++
		n++
	}
	if n < length {
		return false
	}
	for ; i < len(rest); i++ {
		if rest[i] != ' ' && rest[i] != '\t' {
			return false
		}
	}
	return true
}

// htmlBlockEnd reports how a CommonMark HTML block beginning at rest (after
// ≤3 indent) ends, and whether rest opens one. A marker-terminated block
// (comment, CDATA section, processing instruction, declaration) returns its
// closing string; a type-1 raw-text block (script/pre/style/textarea)
// returns type1=true and a nil end (it closes on any of the four closing
// tags — see containsType1Close). These are the HTML blocks whose interior
// may hold a blank line followed by indented text, which a fence-only
// scanner would misread as an indented code block; tracking them keeps
// those lines out of the code set.
//
// The type-6 block-level tag blocks (`<div>`, `<details>`, `<table>`, …) and
// the type-7 blocks (any other complete open/closing tag alone on a line)
// are tracked too, as htmlTag6 / htmlTag7: they end on a blank line, and
// tracking them stops a fence placed right after the tag with no blank line
// — the GitHub collapsible-code / image-then-example README idioms — from
// being misread as a code block. Type-7 cannot interrupt a paragraph, which
// the caller enforces via prevParagraph.
func htmlBlockEnd(rest []byte) (end []byte, kind htmlKind) {
	i := leadingSpaces(rest)
	if i > 3 || i >= len(rest) || rest[i] != '<' {
		return nil, htmlBlockNone
	}
	s := rest[i:]
	switch {
	case bytes.HasPrefix(s, htmlOpenComment):
		return htmlCloseComment, htmlMarker
	case bytes.HasPrefix(s, htmlOpenCDATA):
		return htmlCloseCDATA, htmlMarker
	case bytes.HasPrefix(s, htmlOpenPI):
		return htmlClosePI, htmlMarker
	case len(s) >= 3 && s[1] == '!' && isASCIILetterByte(s[2]):
		return htmlCloseDecl, htmlMarker
	case htmlType1Start(s):
		return nil, htmlRaw
	case htmlType6Start(s):
		return nil, htmlTag6
	case htmlType7Start(s):
		return nil, htmlTag7
	}
	return nil, htmlBlockNone
}

// htmlKind is how an open HTML block ends.
type htmlKind uint8

const (
	htmlBlockNone htmlKind = iota // not an HTML block
	htmlMarker                    // types 2–5: closes on the recorded end string
	htmlRaw                       // type 1 (script/pre/style/textarea): closes on a type-1 closing tag
	htmlTag6                      // type 6 block-level tag: blank-terminated, may interrupt a paragraph
	htmlTag7                      // type 7 complete tag: blank-terminated, may not interrupt a paragraph
)

// htmlType7Start reports whether s is a single complete open or closing HTML
// tag (any tag name; type-1 names are dispatched earlier) filling the line —
// only whitespace may follow it. This is the CommonMark type-7 block start.
func htmlType7Start(s []byte) bool {
	end, ok := scanHTMLTag(s)
	if !ok {
		return false
	}
	for _, c := range s[end:] {
		if c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}

// scanHTMLTag scans a complete open or closing HTML tag at the start of s
// (s[0] == '<') and returns the index just past the closing '>', per the
// CommonMark open-/closing-tag grammar.
func scanHTMLTag(s []byte) (int, bool) {
	if len(s) < 3 || s[0] != '<' {
		return 0, false
	}
	if s[1] == '/' {
		return scanClosingTag(s, 2)
	}
	return scanOpenTag(s, 1)
}

func scanClosingTag(s []byte, i int) (int, bool) {
	i, ok := scanTagName(s, i)
	if !ok {
		return 0, false
	}
	i = skipHTMLWS(s, i)
	if i < len(s) && s[i] == '>' {
		return i + 1, true
	}
	return 0, false
}

func scanOpenTag(s []byte, i int) (int, bool) {
	i, ok := scanTagName(s, i)
	if !ok {
		return 0, false
	}
	for {
		ni, ok := scanAttribute(s, i)
		if !ok {
			break
		}
		i = ni
	}
	i = skipHTMLWS(s, i)
	if i < len(s) && s[i] == '/' {
		i++
	}
	if i < len(s) && s[i] == '>' {
		return i + 1, true
	}
	return 0, false
}

// scanTagName scans an HTML tag name (`[A-Za-z][A-Za-z0-9-]*`) at i.
func scanTagName(s []byte, i int) (int, bool) {
	if i >= len(s) || !isASCIILetterByte(s[i]) {
		return i, false
	}
	i++
	for i < len(s) && (isASCIIAlnum(s[i]) || s[i] == '-') {
		i++
	}
	return i, true
}

// scanAttribute scans one whitespace-led HTML attribute (name, optional
// `=value` with unquoted/single-/double-quoted value) at i.
func scanAttribute(s []byte, i int) (int, bool) {
	j := skipHTMLWS(s, i)
	if j == i { // attributes require leading whitespace
		return i, false
	}
	if j >= len(s) || (!isASCIILetterByte(s[j]) && s[j] != '_' && s[j] != ':') {
		return i, false
	}
	j++
	for j < len(s) && (isASCIIAlnum(s[j]) || s[j] == '_' || s[j] == '.' || s[j] == ':' || s[j] == '-') {
		j++
	}
	k := skipHTMLWS(s, j)
	if k < len(s) && s[k] == '=' {
		return scanAttrValue(s, skipHTMLWS(s, k+1))
	}
	return j, true
}

// scanAttrValue scans an HTML attribute value (unquoted, single-, or
// double-quoted) at i.
func scanAttrValue(s []byte, i int) (int, bool) {
	if i >= len(s) {
		return i, false
	}
	switch s[i] {
	case '\'', '"':
		q := s[i]
		i++
		for i < len(s) && s[i] != q {
			i++
		}
		if i < len(s) {
			return i + 1, true
		}
		return i, false
	default:
		start := i
		for i < len(s) && !isUnquotedStop(s[i]) {
			i++
		}
		return i, i > start
	}
}

// skipHTMLWS returns the index past a run of spaces and tabs at i.
func skipHTMLWS(s []byte, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

// isUnquotedStop reports whether b ends an unquoted attribute value.
func isUnquotedStop(b byte) bool {
	switch b {
	case ' ', '\t', '"', '\'', '=', '<', '>', '`':
		return true
	}
	return false
}

var (
	htmlOpenComment  = []byte("<!--")
	htmlOpenCDATA    = []byte("<![CDATA[")
	htmlOpenPI       = []byte("<?")
	htmlCloseComment = []byte("-->")
	htmlCloseCDATA   = []byte("]]>")
	htmlClosePI      = []byte("?>")
	htmlCloseDecl    = []byte(">")
)

// htmlType1Start reports whether s opens a CommonMark type-1 raw HTML block:
// `<script`, `<pre`, `<style`, or `<textarea` (case-insensitive) followed by
// whitespace, `>`, or end of line.
func htmlType1Start(s []byte) bool {
	for _, name := range htmlType1Names {
		if len(s) < len(name) || !bytes.EqualFold(s[:len(name)], name) {
			continue
		}
		if len(s) == len(name) {
			return true
		}
		switch s[len(name)] {
		case ' ', '\t', '>':
			return true
		}
	}
	return false
}

// containsType1Close reports whether line carries a type-1 raw block's
// closing tag (`</script>`, `</pre>`, `</style>`, `</textarea>`),
// case-insensitively — the CommonMark end condition for those blocks. It
// scans without allocating (no bytes.ToLower copy per body line).
func containsType1Close(line []byte) bool {
	for _, c := range htmlType1Closers {
		if containsFold(line, c) {
			return true
		}
	}
	return false
}

// containsFold reports whether line contains needle, comparing
// case-insensitively (ASCII), without allocating. needle is a non-empty
// lowercase ASCII string (the type-1 closers).
func containsFold(line, needle []byte) bool {
	last := len(line) - len(needle)
	for i := 0; i <= last; i++ {
		if equalFoldASCII(line[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

// equalFoldASCII reports whether a equals b ignoring ASCII case. b is
// assumed lowercase.
func equalFoldASCII(a, b []byte) bool {
	for i := range b {
		c := a[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != b[i] {
			return false
		}
	}
	return true
}

// htmlType6Start reports whether s opens a CommonMark type-6 HTML block: `<`
// or `</` followed by one of the block-level tag names (case-insensitive),
// then whitespace, `>`, `/>`, or end of line. These blocks end on a blank
// line. The tag-name comparison lowercases into a stack buffer so the set
// lookup does not allocate.
func htmlType6Start(s []byte) bool {
	i := 1 // s[0] == '<'
	if i < len(s) && s[i] == '/' {
		i++
	}
	start := i
	for i < len(s) && isASCIIAlnum(s[i]) {
		i++
	}
	n := i - start
	var buf [16]byte
	if n == 0 || n > len(buf) {
		return false
	}
	for k := 0; k < n; k++ {
		c := s[start+k]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		buf[k] = c
	}
	if !htmlType6Tags[string(buf[:n])] {
		return false
	}
	if i >= len(s) {
		return true
	}
	switch s[i] {
	case ' ', '\t', '>':
		return true
	case '/':
		return i+1 < len(s) && s[i+1] == '>'
	}
	return false
}

// isASCIIAlnum reports whether b is an ASCII letter or digit.
func isASCIIAlnum(b byte) bool {
	return isASCIILetterByte(b) || (b >= '0' && b <= '9')
}

// htmlType6Tags is the CommonMark type-6 block-level tag set (lowercase).
var htmlType6Tags = map[string]bool{
	"address": true, "article": true, "aside": true, "base": true,
	"basefont": true, "blockquote": true, "body": true, "caption": true,
	"center": true, "col": true, "colgroup": true, "dd": true, "details": true,
	"dialog": true, "dir": true, "div": true, "dl": true, "dt": true,
	"fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"form": true, "frame": true, "frameset": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "head": true,
	"header": true, "hr": true, "html": true, "iframe": true, "legend": true,
	"li": true, "link": true, "main": true, "menu": true, "menuitem": true,
	"nav": true, "noframes": true, "ol": true, "optgroup": true, "option": true,
	"p": true, "param": true, "section": true, "summary": true, "table": true,
	"tbody": true, "td": true, "tfoot": true, "th": true, "thead": true,
	"title": true, "tr": true, "track": true, "ul": true,
}

var (
	htmlType1Names = [][]byte{
		[]byte("<script"), []byte("<pre"), []byte("<style"), []byte("<textarea"),
	}
	htmlType1Closers = [][]byte{
		[]byte("</script>"), []byte("</pre>"), []byte("</style>"), []byte("</textarea>"),
	}
)

// isASCIILetterByte reports whether b is an ASCII letter.
func isASCIILetterByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// blockquoteMarker returns the byte width of a blockquote opener at the
// start of rest (`[ ]{0,3}>` plus one optional following space), or 0
// when rest does not open a blockquote.
func blockquoteMarker(rest []byte) int {
	i := leadingSpaces(rest)
	if i > 3 || i >= len(rest) || rest[i] != '>' {
		return 0
	}
	i++
	if i < len(rest) && rest[i] == ' ' {
		i++
	}
	return i
}

// listMarkerWidth returns the content width of a list-item marker at the
// start of rest — the byte count from rest[0] to the first content
// character — or 0 when rest does not open a list item. It handles bullet
// markers (`-`, `+`, `*`) and ordered markers (digits then `.` or `)`),
// each followed by at least one space or tab. A marker with no following
// space (or one whose content would be empty) is not a list start.
func listMarkerWidth(rest []byte) int {
	i := leadingSpaces(rest)
	if i > 3 || i >= len(rest) {
		return 0
	}
	switch rest[i] {
	case '-', '+', '*':
		i++
	default:
		start := i
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		if i == start || i >= len(rest) || (rest[i] != '.' && rest[i] != ')') {
			return 0
		}
		i++
	}
	// At least one space/tab must follow the marker, and some content
	// must follow it (a bare `-` on its own line is a thematic break or
	// empty list item, not a content-bearing marker this pass tracks).
	if i >= len(rest) || (rest[i] != ' ' && rest[i] != '\t') {
		return 0
	}
	markerEnd := i
	for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t') {
		i++
	}
	if i >= len(rest) {
		return 0
	}
	// CommonMark: 1–4 columns of indentation after the marker join it as
	// the content width; 5 or more means only the first space belongs to
	// the marker and the remaining indentation makes the item's content an
	// indented code block — so the marker width stops after that one space
	// and the indent is left in rest for the indented-code check.
	if indentColumns(rest[markerEnd:], markerEnd) >= 5 {
		return markerEnd + 1
	}
	return i
}
