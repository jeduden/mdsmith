package mdhtml

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompleteTag(t *testing.T) {
	tests := []struct {
		s           string
		inParagraph bool
		want        Kind
	}{
		{"<span>", false, Type7},
		{"<span>", true, None},
		{"<div>", true, Type6},
		{"</ div a>", true, Type6},
		{"</span a>", false, None},
		{"</ span a>", false, Type7},
		{"</ span a>", true, None},
		{"<pre>", false, None},
		{"<span/>  ", false, Type7},
		{"<span> x", false, None},
		{"<span", false, None},
		{"<1", false, None},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, completeTag([]byte(tt.s), tt.inParagraph), "%q", tt.s)
	}
}

func TestType6Start(t *testing.T) {
	tests := map[string]bool{
		"<div": true, "<div x": true, "<div>x": true, "<div/>": true, "</  p>": true,
		"<div/": false, "<div\t": false, "<span>": false, "<divx": false, "<": false,
	}
	for s, want := range tests {
		assert.Equal(t, want, type6Start([]byte(s)), "%q", s)
	}
}

func TestType1Start(t *testing.T) {
	tests := map[string]bool{
		"<pre": true, "<PRE>": true, "<script\t": true, "<style\f": true, "<textarea/>": true,
		"<pre/": false, "<prex": false, "<span": false, "<scr": false,
	}
	for s, want := range tests {
		assert.Equal(t, want, type1Start([]byte(s)), "%q", s)
	}
}

func TestScanTagName(t *testing.T) {
	end, ok := scanTagName([]byte("<x-1y z"), 1)
	assert.True(t, ok)
	assert.Equal(t, 5, end)
	_, ok = scanTagName([]byte("<1"), 1)
	assert.False(t, ok)
	_, ok = scanTagName([]byte("<"), 1)
	assert.False(t, ok)
}

func TestScanAttribute(t *testing.T) {
	tests := []struct {
		s    string
		end  int
		want bool
	}{
		{" a", 2, true},
		{" a=b", 4, true},
		{" a = 'b c'", 10, true},
		{" _x:y.z-w", 9, true},
		{" a >", 2, true},
		{"a", 0, false},
		{" ", 0, false},
		{" =x", 0, false},
		{" a=", 0, false},
		{" a=\"b", 0, false},
	}
	for _, tt := range tests {
		end, ok := scanAttribute([]byte(tt.s), 0)
		assert.Equal(t, tt.want, ok, "%q", tt.s)
		assert.Equal(t, tt.end, end, "%q", tt.s)
	}
}

func TestScanAttrValue(t *testing.T) {
	tests := []struct {
		s    string
		end  int
		want bool
	}{
		{"b c", 1, true},
		{"'b c'x", 5, true},
		{`"b"`, 3, true},
		{"'b", 0, false},
		{">", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		end, ok := scanAttrValue([]byte(tt.s), 0)
		assert.Equal(t, tt.want, ok, "%q", tt.s)
		assert.Equal(t, tt.end, end, "%q", tt.s)
	}
}

func TestSkipSpace(t *testing.T) {
	assert.Equal(t, 4, skipSpace([]byte(" \t\r\nx"), 0))
	assert.Equal(t, 2, skipSpace([]byte("x x"), 1))
	assert.Equal(t, 0, skipSpace([]byte("x"), 0))
}

func TestIsUnquotedStop(t *testing.T) {
	for _, c := range []byte("\"'=<>` \t\x00") {
		assert.True(t, isUnquotedStop(c), "%q", c)
	}
	for _, c := range []byte("a/-.") {
		assert.False(t, isUnquotedStop(c), "%q", c)
	}
}

func TestIsAttrNameByte(t *testing.T) {
	for _, c := range []byte("aZ9:._-") {
		assert.True(t, isAttrNameByte(c), "%q", c)
	}
	for _, c := range []byte(" =>/") {
		assert.False(t, isAttrNameByte(c), "%q", c)
	}
}

func TestIsLetter(t *testing.T) {
	for _, c := range []byte("azAZ") {
		assert.True(t, isLetter(c), "%q", c)
	}
	for _, c := range []byte("@[`{09-\xe1") {
		assert.False(t, isLetter(c), "%q", c)
	}
}

func TestIsDigit(t *testing.T) {
	assert.True(t, isDigit('0'))
	assert.True(t, isDigit('9'))
	assert.False(t, isDigit('a'))
}

func TestTrimCR(t *testing.T) {
	assert.Equal(t, "<a>", string(trimCR([]byte("<a>\r"))))
	assert.Equal(t, "<a>", string(trimCR([]byte("<a>"))))
	assert.Empty(t, trimCR(nil))
}

func TestIsBlank(t *testing.T) {
	assert.True(t, isBlank(nil))
	assert.True(t, isBlank([]byte(" \t\r\n")))
	assert.False(t, isBlank([]byte(" x")))
	assert.False(t, isBlank([]byte("\v")))
}

func TestIsBlockTag(t *testing.T) {
	assert.True(t, isBlockTag([]byte("BlockQuote")))
	assert.True(t, isBlockTag([]byte("h1")))
	assert.False(t, isBlockTag([]byte("span")))
	assert.False(t, isBlockTag([]byte("averyveryverylongtag")))
}

func TestIsRawTextTag(t *testing.T) {
	for _, s := range []string{"script", "STYLE", "Pre"} {
		assert.True(t, isRawTextTag([]byte(s)), s)
	}
	for _, s := range []string{"textarea", "pres", "div"} {
		assert.False(t, isRawTextTag([]byte(s)), s)
	}
}

func TestContainsFold(t *testing.T) {
	assert.True(t, containsFold([]byte("a </PRE> b"), []byte("</pre>")))
	assert.False(t, containsFold([]byte("</pr"), []byte("</pre>")))
	assert.False(t, containsFold([]byte("</pro>"), []byte("</pre>")))
}

func TestEqualFold(t *testing.T) {
	assert.True(t, equalFold([]byte("ScRiPt"), []byte("script")))
	assert.False(t, equalFold([]byte("scrip"), []byte("script")))
	assert.False(t, equalFold([]byte("scripx"), []byte("script")))
}
