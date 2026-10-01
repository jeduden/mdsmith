// Package mdhtml answers one question about a single Markdown source
// line: does it start a CommonMark HTML block, and if one is open, does
// this line end it?
//
// The rules mirror the goldmark fork's htmlBlockParser in
// pkg/goldmark/parser/html_block.go, quirks included, so a line scanner
// built on this package agrees with the parsed AST rather than with the
// letter of the spec where the two differ:
//
//   - Type 1: `<script`, `<pre`, `<style`, or `<textarea`
//     (case-insensitive) then whitespace, `>`, `/>`, or the line end.
//     Ends on a line holding `</script>`, `</pre>`, `</style>`, or
//     `</textarea>` (case-insensitive).
//   - Types 2–5: `<!--`, `<?`, `<!` plus an upper-case ASCII letter (the
//     fork does not accept a lower-case `<!doctype`), and `<![CDATA[`.
//     Each ends on a line holding `-->`, `?>`, `>`, or `]]>`.
//   - Type 6: `<` or `</` (spaces may follow the slash) plus one of the
//     block-level tag names (case-insensitive), then a space, `>`, `/>`,
//     or the line end. A tab after the name does not qualify. Ends on a
//     blank line.
//   - Type 7: one complete open or closing tag of any other name, alone
//     on the line except for trailing spaces. A closing tag may carry
//     no attributes, and the raw-text names script, style, and pre are
//     never type 7. Type 7 cannot interrupt a paragraph. Ends on a
//     blank line.
//
// Every opener allows up to three spaces of indentation and no tab. A
// line here carries no "\n"; a trailing "\r" from a CRLF line is read
// as part of the line ending, as goldmark reads "\r\n". The helpers
// never allocate and know nothing about containers: a caller scanning
// inside a list item or block quote passes the line with the container
// prefix stripped.
package mdhtml

import "bytes"

// Kind is the CommonMark HTML block type a line starts (1–7), or None.
// Each type has its own end condition, which Closes encodes.
type Kind uint8

// The HTML block types, numbered as in CommonMark §4.6.
const (
	None Kind = iota
	Type1
	Type2
	Type3
	Type4
	Type5
	Type6
	Type7
)

// Open reports which HTML block type line starts, or None.
// inParagraph reports whether line would otherwise continue an open
// paragraph; a type-7 start cannot interrupt one, so it then reports
// None for a line that would be type 7.
func Open(line []byte, inParagraph bool) Kind {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	if i > 3 || i >= len(line) || line[i] != '<' {
		return None
	}
	s := trimCR(line[i:])
	switch {
	case type1Start(s):
		return Type1
	case bytes.HasPrefix(s, openComment):
		return Type2
	case bytes.HasPrefix(s, openPI):
		return Type3
	case len(s) >= 3 && s[1] == '!' && s[2] >= 'A' && s[2] <= 'Z':
		return Type4
	case bytes.HasPrefix(s, openCDATA):
		return Type5
	}
	if k := completeTag(s, inParagraph); k != None {
		return k
	}
	if type6Start(s) {
		return Type6
	}
	return None
}

// Closes reports whether line ends an open HTML block of kind k. For
// types 1–5 that is a line holding the type's terminator, and the line
// belongs to the block; a caller also tests the opening line, since a
// block may end on the line that starts it. For types 6 and 7 it is a
// blank line, which does not belong to the block. Closes reports false
// for None.
func Closes(line []byte, k Kind) bool {
	switch k {
	case Type1:
		for _, c := range type1Closers {
			if containsFold(line, c) {
				return true
			}
		}
		return false
	case Type2:
		return bytes.Contains(line, closeComment)
	case Type3:
		return bytes.Contains(line, closePI)
	case Type4:
		return bytes.IndexByte(line, '>') >= 0
	case Type5:
		return bytes.Contains(line, closeCDATA)
	case Type6, Type7:
		return isBlank(line)
	}
	return false
}

// completeTag mirrors the fork's type-7 pattern, which it tests before
// the type-6 one: a complete open or closing tag alone on the line. A
// block-level tag name makes it type 6 (which may interrupt a
// paragraph); any other name makes it type 7 unless the tag is a
// raw-text name, a closing tag with attributes, or interrupts a
// paragraph. s starts at the '<'.
func completeTag(s []byte, inParagraph bool) Kind {
	i := 1
	closing := false
	if i < len(s) && s[i] == '/' {
		closing = true
		i++
		for i < len(s) && s[i] == ' ' {
			i++
		}
	}
	nameStart := i
	i, ok := scanTagName(s, i)
	if !ok {
		return None
	}
	name := s[nameStart:i]
	attrStart := i
	for {
		ni, ok := scanAttribute(s, i)
		if !ok {
			break
		}
		i = ni
	}
	hasAttr := i != attrStart
	for i < len(s) && s[i] == ' ' {
		i++
	}
	if i < len(s) && s[i] == '/' {
		i++
	}
	if i >= len(s) || s[i] != '>' {
		return None
	}
	for i++; i < len(s); i++ {
		if s[i] != ' ' {
			return None
		}
	}
	switch {
	case isBlockTag(name):
		return Type6
	case inParagraph || isRawTextTag(name) || closing && hasAttr:
		return None
	}
	return Type7
}

// type6Start mirrors the fork's type-6 pattern: `<` or `</` (spaces may
// follow the slash), a block-level tag name, then a space, `>`, `/>`,
// or the line end. s starts at the '<'.
func type6Start(s []byte) bool {
	i := 1
	if i < len(s) && s[i] == '/' {
		i++
		for i < len(s) && s[i] == ' ' {
			i++
		}
	}
	nameStart := i
	i, ok := scanTagName(s, i)
	if !ok || !isBlockTag(s[nameStart:i]) {
		return false
	}
	if i == len(s) {
		return true
	}
	switch s[i] {
	case ' ', '>':
		return true
	case '/':
		return i+1 < len(s) && s[i+1] == '>'
	}
	return false
}

// type1Start reports whether s opens a type-1 raw-text block: one of
// the type-1 names (case-insensitive) followed by whitespace, `>`,
// `/>`, or the line end. s starts at the '<'.
func type1Start(s []byte) bool {
	for _, name := range type1Names {
		if len(s) < len(name) || !equalFold(s[:len(name)], name) {
			continue
		}
		rest := s[len(name):]
		if len(rest) == 0 {
			return true
		}
		switch rest[0] {
		case ' ', '\t', '\n', '\f', '\r', '>':
			return true
		case '/':
			return len(rest) > 1 && rest[1] == '>'
		}
	}
	return false
}

// scanTagName scans a tag name, an ASCII letter then letters, digits,
// or '-', at i and returns the index past it.
func scanTagName(s []byte, i int) (int, bool) {
	if i >= len(s) || !isLetter(s[i]) {
		return i, false
	}
	i++
	for i < len(s) && (isLetter(s[i]) || isDigit(s[i]) || s[i] == '-') {
		i++
	}
	return i, true
}

// scanAttribute scans one attribute at i: whitespace, a name, and an
// optional `=` value (unquoted, single-, or double-quoted). It reports
// false, leaving i unchanged, when no attribute starts at i.
func scanAttribute(s []byte, i int) (int, bool) {
	j := skipSpace(s, i)
	if j == i || j >= len(s) {
		return i, false
	}
	if c := s[j]; !isLetter(c) && c != '_' && c != ':' {
		return i, false
	}
	j++
	for j < len(s) && isAttrNameByte(s[j]) {
		j++
	}
	k := skipSpace(s, j)
	if k >= len(s) || s[k] != '=' {
		return j, true
	}
	end, ok := scanAttrValue(s, skipSpace(s, k+1))
	if !ok {
		return i, false
	}
	return end, true
}

// scanAttrValue scans an attribute value at i and returns the index
// past it.
func scanAttrValue(s []byte, i int) (int, bool) {
	if i >= len(s) {
		return i, false
	}
	if q := s[i]; q == '\'' || q == '"' {
		end := bytes.IndexByte(s[i+1:], q)
		if end < 0 {
			return i, false
		}
		return i + 1 + end + 1, true
	}
	start := i
	for i < len(s) && !isUnquotedStop(s[i]) {
		i++
	}
	return i, i > start
}

// skipSpace returns the index past the attribute whitespace (space,
// tab, CR, LF) at i.
func skipSpace(s []byte, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
		i++
	}
	return i
}

// isUnquotedStop reports whether c ends an unquoted attribute value: a
// quote, '=', '<', '>', '`', or a control or space byte.
func isUnquotedStop(c byte) bool {
	switch c {
	case '"', '\'', '=', '<', '>', '`':
		return true
	}
	return c <= 0x20
}

// isAttrNameByte reports whether c may follow the first byte of an
// attribute name.
func isAttrNameByte(c byte) bool {
	return isLetter(c) || isDigit(c) || c == ':' || c == '.' || c == '_' || c == '-'
}

func isLetter(c byte) bool { return c|0x20 >= 'a' && c|0x20 <= 'z' }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// trimCR drops one trailing "\r", which goldmark reads with the "\n"
// after it as the line ending.
func trimCR(s []byte) []byte {
	if n := len(s); n > 0 && s[n-1] == '\r' {
		return s[:n-1]
	}
	return s
}

// isBlank reports whether line holds only goldmark whitespace (space,
// tab, CR, LF).
func isBlank(line []byte) bool {
	for _, c := range line {
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			return false
		}
	}
	return true
}

// isBlockTag reports whether name is (case-insensitively) a type-6
// block-level tag. It lowercases into a stack buffer so the set lookup
// allocates nothing; a name longer than the buffer is no block tag.
func isBlockTag(name []byte) bool {
	var buf [16]byte
	if len(name) > len(buf) {
		return false
	}
	for i, c := range name {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		buf[i] = c
	}
	_, ok := blockTags[string(buf[:len(name)])]
	return ok
}

// isRawTextTag reports whether name is (case-insensitively) script,
// style, or pre: the fork never opens a type-7 block on them.
func isRawTextTag(name []byte) bool {
	return equalFold(name, rawScript) || equalFold(name, rawStyle) || equalFold(name, rawPre)
}

// containsFold reports whether line holds needle, comparing ASCII case
// insensitively. needle is lower-case.
func containsFold(line, needle []byte) bool {
	for i := 0; i+len(needle) <= len(line); i++ {
		if equalFold(line[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

// equalFold reports whether a equals the lower-case ASCII b, ignoring
// the case of a.
func equalFold(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i, c := range a {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != b[i] {
			return false
		}
	}
	return true
}

var (
	openComment  = []byte("<!--")
	openPI       = []byte("<?")
	openCDATA    = []byte("<![CDATA[")
	closeComment = []byte("-->")
	closePI      = []byte("?>")
	closeCDATA   = []byte("]]>")

	rawScript = []byte("script")
	rawStyle  = []byte("style")
	rawPre    = []byte("pre")

	type1Names = [][]byte{
		[]byte("<script"), []byte("<pre"), []byte("<style"), []byte("<textarea"),
	}
	type1Closers = [][]byte{
		[]byte("</script>"), []byte("</pre>"), []byte("</style>"), []byte("</textarea>"),
	}
)

// blockTags is the type-6 block-level tag set, kept in step with the
// fork's allowedBlockTags.
var blockTags = map[string]struct{}{
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
