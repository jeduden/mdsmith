package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// These tests exhaustively pin the branches of the flat-classifier byte
// scanners. They are pure functions, so a table covers every arm —
// including the ones no corpus fixture happens to exercise (HTML
// declarations, over-indented fences, malformed list markers).

func TestLeadingSpacesAndIndent(t *testing.T) {
	assert.Equal(t, 0, leadingSpaces([]byte("abc")))
	assert.Equal(t, 2, leadingSpaces([]byte("  abc")))
	assert.Equal(t, 3, leadingSpaces([]byte("   ")))
	assert.Equal(t, 0, leadingSpaces([]byte("\tabc")))

	assert.Equal(t, 0, indentColumns([]byte(""), 0))
	assert.Equal(t, 0, indentColumns([]byte("x"), 0))
	assert.Equal(t, 1, indentColumns([]byte(" x"), 0))
	assert.Equal(t, 4, indentColumns([]byte("\tx"), 0))
	assert.Equal(t, 4, indentColumns([]byte("  \tx"), 0)) // 2 spaces then tab → next stop
	assert.Equal(t, 4, indentColumns([]byte("    x"), 0))
	assert.Equal(t, 5, indentColumns([]byte("     "), 0)) // all spaces, no content
	// startCol shifts the tab stop: a tab at absolute column 2 advances to
	// column 4 (2 columns), not a full 4.
	assert.Equal(t, 2, indentColumns([]byte("\tx"), 2))
	assert.Equal(t, 3, indentColumns([]byte("\t x"), 2)) // tab→col4 (2) + space (1)
}

func TestBlankScanners(t *testing.T) {
	assert.True(t, isBlankBytes([]byte("")))
	assert.True(t, isBlankBytes([]byte("  \t ")))
	assert.False(t, isBlankBytes([]byte("x")))
	assert.False(t, isBlankBytes([]byte("  x")))

	assert.True(t, isBlankFrom([]byte("abc"), 3))
	assert.True(t, isBlankFrom([]byte("ab  "), 2))
	assert.False(t, isBlankFrom([]byte("ab c"), 2))
}

func TestHeadingScanners(t *testing.T) {
	assert.True(t, isATXHeading([]byte("# h")))
	assert.True(t, isATXHeading([]byte("###### h")))
	assert.True(t, isATXHeading([]byte("#")))          // EOL after marker
	assert.True(t, isATXHeading([]byte("#\tt")))       // tab after marker
	assert.False(t, isATXHeading([]byte("####### h"))) // 7 hashes → not ATX
	assert.False(t, isATXHeading([]byte("#h")))        // no space
	assert.False(t, isATXHeading([]byte("text")))

	assert.True(t, isSetextUnderline([]byte("===")))
	assert.True(t, isSetextUnderline([]byte("---")))
	assert.True(t, isSetextUnderline([]byte("===   "))) // trailing spaces
	assert.False(t, isSetextUnderline([]byte("")))
	assert.False(t, isSetextUnderline([]byte("= ="))) // gap then more
	assert.False(t, isSetextUnderline([]byte("abc")))
}

func TestFenceScanners(t *testing.T) {
	ch, n, info, ok := detectFenceOpen([]byte("```"))
	assert.True(t, ok)
	assert.Equal(t, byte('`'), ch)
	assert.Equal(t, 3, n)
	assert.False(t, info)
	_, _, info, ok = detectFenceOpen([]byte("```go"))
	assert.True(t, ok)
	assert.True(t, info)
	_, _, _, ok = detectFenceOpen([]byte("``"))
	assert.False(t, ok, "fewer than three backticks")
	_, _, _, ok = detectFenceOpen([]byte("```a`b"))
	assert.False(t, ok, "backtick in a backtick-fence info string")
	_, _, info, ok = detectFenceOpen([]byte("~~~a`b"))
	assert.True(t, ok, "tilde fence allows a backtick in its info")
	assert.True(t, info)
	_, _, _, ok = detectFenceOpen([]byte("    ```"))
	assert.False(t, ok, "four-space indent is code, not a fence")
	_, _, _, ok = detectFenceOpen([]byte("text"))
	assert.False(t, ok)

	assert.True(t, isFenceClose([]byte("```"), '`', 3))
	assert.True(t, isFenceClose([]byte("````"), '`', 3), "longer than the open fence")
	assert.True(t, isFenceClose([]byte("```   "), '`', 3), "trailing whitespace allowed")
	assert.False(t, isFenceClose([]byte("``"), '`', 3), "shorter than the open fence")
	assert.False(t, isFenceClose([]byte("```x"), '`', 3), "non-space after the fence")
	assert.False(t, isFenceClose([]byte("    ```"), '`', 3), "over-indented")
}

func TestHTMLScanners(t *testing.T) {
	end, kind := htmlBlockEnd([]byte("<!-- x"))
	assert.Equal(t, htmlMarker, kind)
	assert.Equal(t, "-->", string(end))
	end, kind = htmlBlockEnd([]byte("<![CDATA[x"))
	assert.Equal(t, htmlMarker, kind)
	assert.Equal(t, "]]>", string(end))
	end, kind = htmlBlockEnd([]byte("<?php"))
	assert.Equal(t, htmlMarker, kind)
	assert.Equal(t, "?>", string(end))
	end, kind = htmlBlockEnd([]byte("<!DOCTYPE html>"))
	assert.Equal(t, htmlMarker, kind)
	assert.Equal(t, ">", string(end))
	_, kind = htmlBlockEnd([]byte("<pre>")) // type-1 raw block
	assert.Equal(t, htmlRaw, kind)
	_, kind = htmlBlockEnd([]byte("<SCRIPT type=x")) // case-insensitive
	assert.Equal(t, htmlRaw, kind)
	_, kind = htmlBlockEnd([]byte("<div>")) // type-6 block tag
	assert.Equal(t, htmlTag6, kind)
	_, kind = htmlBlockEnd([]byte("</Details>")) // closing tag, case-insensitive
	assert.Equal(t, htmlTag6, kind)
	_, kind = htmlBlockEnd([]byte(`<img src="x.png">`)) // type-7 complete tag
	assert.Equal(t, htmlTag7, kind)
	_, kind = htmlBlockEnd([]byte("  <!-- x")) // ≤3 indent still opens
	assert.Equal(t, htmlMarker, kind)
	_, kind = htmlBlockEnd([]byte("    <!-- x")) // 4-space indent does not
	assert.Equal(t, htmlNone, kind)
	_, kind = htmlBlockEnd([]byte("<!5 not a decl"))
	assert.Equal(t, htmlNone, kind)
	_, kind = htmlBlockEnd([]byte("<img> trailing text")) // tag not alone on line → not type-7
	assert.Equal(t, htmlNone, kind)
	_, kind = htmlBlockEnd([]byte("text"))
	assert.Equal(t, htmlNone, kind)
}

// TestHTMLType7Start exhaustively pins the complete-tag scanner for the
// type-7 block start: a single open or closing tag filling the line, with
// the full attribute grammar (unquoted / single- / double-quoted values,
// valueless attributes, self-closing), and the rejects.
func TestHTMLType7Start(t *testing.T) {
	// Valid type-7 starts.
	for _, s := range []string{
		"<img>", "<br>", "<br/>", "<br />", "<span>", "</div>", "</div >",
		"<a-b>", "<img src=x>", "<img src='x'>", `<img src="x">`,
		`<img src="x" alt='y'>`, "<input disabled>", "<x a='1' b=\"2\" c=d>",
		"<img\tsrc=x>", "<col :ns=v>", "<el _x=v>", "<el a.b-c:d=v>",
		"<img>   ", // trailing whitespace allowed
	} {
		assert.Truef(t, htmlType7Start([]byte(s)), "expected type-7 start: %q", s)
	}
	// Not type-7 starts.
	for _, s := range []string{
		"<img> text after", // not alone on the line
		"<img src=>",       // '=' with no value
		"<img src=",        // '=' at end of input, no value
		`<img src="x`,      // unclosed double quote
		"<img src='x",      // unclosed single quote
		"<1tag>",           // name may not start with a digit
		"< img>",           // space after '<'
		"<>",               // no name
		"</>",              // closing with no name
		"</div",            // closing with no '>'
		"<img src=\"x\"y>", // missing whitespace before next attribute
		"<img /x>",         // '/' not immediately before '>'
		"<img",             // no '>'
		"<=bad>",           // not a name
	} {
		assert.Falsef(t, htmlType7Start([]byte(s)), "expected NOT type-7: %q", s)
	}
}

// TestByteClassHelpers pins the small ASCII-class predicates.
func TestByteClassHelpers(t *testing.T) {
	assert.True(t, isASCIILetterByte('a'))
	assert.True(t, isASCIILetterByte('Z'))
	assert.False(t, isASCIILetterByte('5'))
	assert.False(t, isASCIILetterByte('!'))
	assert.True(t, isASCIIAlnum('a'))
	assert.True(t, isASCIIAlnum('7'))
	assert.False(t, isASCIIAlnum('-'))
}

// TestHTMLType6AndClose pins the type-6 tag-block opener arms and the
// case-insensitive type-1 close scan (containsType1Close / containsFold).
func TestHTMLType6AndClose(t *testing.T) {
	assert.True(t, htmlType6Start([]byte("<div>")))
	assert.True(t, htmlType6Start([]byte("<section foo")), "whitespace boundary")
	assert.True(t, htmlType6Start([]byte("<hr/>")), "self-closing boundary")
	assert.True(t, htmlType6Start([]byte("</details>")), "closing tag")
	assert.True(t, htmlType6Start([]byte("<TABLE")), "case-insensitive, EOL boundary")
	assert.False(t, htmlType6Start([]byte("<div=x")), "non-boundary char after a tag name")
	assert.False(t, htmlType6Start([]byte("<hr/x")), "slash not followed by >")
	assert.False(t, htmlType6Start([]byte("<notatag>")), "name not in the type-6 set")
	assert.False(t, htmlType6Start([]byte("<>")), "no tag name")
	assert.False(t, htmlType6Start([]byte("<verylongtagnamexceeds>")), "name longer than the buffer")

	assert.True(t, containsType1Close([]byte("x</PRE>y")), "case-insensitive close")
	assert.True(t, containsType1Close([]byte("lead </TextArea> trail")))
	assert.False(t, containsType1Close([]byte("x</em>y")))
	assert.False(t, containsType1Close([]byte("short")), "shorter than any closer")
}

func TestContainerMarkerScanners(t *testing.T) {
	assert.Equal(t, 2, blockquoteMarker([]byte("> x")))
	assert.Equal(t, 1, blockquoteMarker([]byte(">x")), "marker with no following space")
	assert.Equal(t, 1, blockquoteMarker([]byte(">")))
	assert.Equal(t, 4, blockquoteMarker([]byte("  > x")), "≤3 indent allowed")
	assert.Equal(t, 0, blockquoteMarker([]byte("    > x")), "4-space indent is not a marker")
	assert.Equal(t, 0, blockquoteMarker([]byte("text")))

	assert.Equal(t, 2, listMarkerWidth([]byte("- x")))
	assert.Equal(t, 2, listMarkerWidth([]byte("* x")))
	assert.Equal(t, 2, listMarkerWidth([]byte("+ x")))
	assert.Equal(t, 3, listMarkerWidth([]byte("1. x")))
	assert.Equal(t, 4, listMarkerWidth([]byte("12) x")))
	assert.Equal(t, 0, listMarkerWidth([]byte("-x")), "no space after the marker")
	assert.Equal(t, 0, listMarkerWidth([]byte("- ")), "no content after the marker")
	assert.Equal(t, 0, listMarkerWidth([]byte("1.x")), "ordered marker needs a space")
	assert.Equal(t, 0, listMarkerWidth([]byte("1")), "digits with no delimiter")
	assert.Equal(t, 0, listMarkerWidth([]byte("    - x")), "4-space indent is not a marker")
	assert.Equal(t, 0, listMarkerWidth([]byte("text")))
}

// TestContainerConsume pins both arms of each container's continuation
// matcher: the blockquote marker, its miss, the list width match, the
// list miss (a dedent), and the blank-line continuation of a list.
func TestContainerConsume(t *testing.T) {
	bq := lc0Container{blockquote: true}
	next, ok := bq.consume([]byte("> x"), 0)
	assert.True(t, ok)
	assert.Equal(t, 2, next)
	next, ok = bq.consume([]byte(">x"), 0) // marker with no following space
	assert.True(t, ok)
	assert.Equal(t, 1, next)
	next, ok = bq.consume([]byte("   > x"), 0) // ≤3 leading spaces then marker
	assert.True(t, ok)
	assert.Equal(t, 5, next)
	_, ok = bq.consume([]byte("plain"), 0)
	assert.False(t, ok, "a line with no marker closes the blockquote")
	_, ok = bq.consume([]byte("    > over-indented"), 0)
	assert.False(t, ok)

	li := lc0Container{width: 2}
	next, ok = li.consume([]byte("  x"), 0)
	assert.True(t, ok)
	assert.Equal(t, 2, next)
	next, ok = li.consume([]byte("\tx"), 0)
	assert.True(t, ok, "a tab satisfies the list-item width in columns")
	assert.Equal(t, 1, next)
	next, ok = li.consume([]byte(""), 0)
	assert.True(t, ok, "a blank line continues a list item")
	assert.Equal(t, 0, next)
	_, ok = li.consume([]byte(" x"), 0)
	assert.False(t, ok, "one space is under the width-2 list item")
	_, ok = li.consume([]byte("x"), 0)
	assert.False(t, ok, "a dedented line closes the list item")
}

// TestHTMLType1Start pins the type-1 raw-block opener recognition: the four
// names case-insensitively, the boundary after the name, and the rejects.
func TestHTMLType1Start(t *testing.T) {
	assert.True(t, htmlType1Start([]byte("<pre>")))
	assert.True(t, htmlType1Start([]byte("<TEXTAREA")), "EOL right after the name")
	assert.True(t, htmlType1Start([]byte("<style\tx")), "tab boundary")
	assert.True(t, htmlType1Start([]byte("<script foo")))
	assert.False(t, htmlType1Start([]byte("<prefix>")), "name is a prefix of a longer tag")
	assert.False(t, htmlType1Start([]byte("<div>")))
	assert.False(t, htmlType1Start([]byte("<pr")), "shorter than any name")
}
