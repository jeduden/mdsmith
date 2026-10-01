package mdhtml

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// openCases pins Open's verdict per line; TestOpen_MatchesFork checks
// the same shapes against the fork parser.
var openCases = []struct {
	line        string
	inParagraph bool
	want        Kind
}{
	{"", false, None},
	{"text", false, None},
	{"    <div>", false, None},
	{"\t<div>", false, None},
	{"<", false, None},
	{"<1div>", false, None},

	{"<script>", false, Type1},
	{"<SCRIPT src=a>", true, Type1},
	{"<pre", false, Type1},
	{"<style\r", false, Type1},
	{"<textarea/>", false, Type1},
	{"<scriptx>", false, Type7},
	{"</script>", false, None},
	{"</pre>", false, None},

	{"<!-- x -->", true, Type2},
	{"   <!--", false, Type2},
	{"<?toc?>", true, Type3},
	{"<? x", false, Type3},
	{"<!DOCTYPE html>", true, Type4},
	{"<!doctype html>", false, None},
	{"<![CDATA[x]]>", true, Type5},

	{"<div>", false, Type6},
	{"<DIV>", true, Type6},
	{"</div>", true, Type6},
	{"</ div>", true, Type6},
	{"<div", false, Type6},
	{"<div\r", false, Type6},
	{"<div class=\"a\">", true, Type6},
	{"<div/>", false, Type6},
	{"<div>text", true, Type6},
	{"<div\t>", false, None},
	{"<div/", false, None},
	{"<divx>", false, Type7},
	{"</div a>", false, Type6},

	{"<span>", false, Type7},
	{"<span>", true, None},
	{"</span>", false, Type7},
	{"<custom-tag>", false, Type7},
	{"<x-y/>", false, Type7},
	{"<span>  ", false, Type7},
	{"<span>\r", false, Type7},
	{"<span>\t", false, None},
	{"<span>Title</span>", false, None},
	{"<span =x>", false, None},
	{"</span a>", false, None},
	{"<span a=b c='d' e=\"f\" g>", false, Type7},
	{"<span a = b>", false, Type7},
	{"<span a=>", false, None},
	{"<span a=", false, None},
	{"<span a='b>", false, None},
	{"<a href=x/>", false, Type7},
	{"<span\t a>", false, Type7},
	{"</textarea>", false, Type7},
}

func TestOpen(t *testing.T) {
	for _, tt := range openCases {
		assert.Equal(t, tt.want, Open([]byte(tt.line), tt.inParagraph),
			"Open(%q, %v)", tt.line, tt.inParagraph)
	}
}

func TestCloses(t *testing.T) {
	tests := []struct {
		line string
		kind Kind
		want bool
	}{
		{"x </SCRIPT> y", Type1, true},
		{"</pre>", Type1, true},
		{"</textarea >", Type1, false},
		{"x", Type1, false},
		{"a --> b", Type2, true},
		{"--", Type2, false},
		{"?>", Type3, true},
		{"? >", Type3, false},
		{">", Type4, true},
		{"x", Type4, false},
		{"]]>", Type5, true},
		{"]>", Type5, false},
		{"", Type6, true},
		{" \t\r", Type6, true},
		{"x", Type6, false},
		{"", Type7, true},
		{"x", Type7, false},
		{"", None, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, Closes([]byte(tt.line), tt.kind),
			"Closes(%q, %v)", tt.line, tt.kind)
	}
}

func TestOpenCloses_ZeroAllocs(t *testing.T) {
	lines := [][]byte{
		[]byte("<DIV class=\"a\">"), []byte("<span a=b c='d'>"),
		[]byte("<Script>"), []byte("<!-- x -->"), []byte("plain text"),
	}
	allocs := testing.AllocsPerRun(100, func() {
		for _, l := range lines {
			k := Open(l, false)
			_ = Closes(l, k)
		}
	})
	assert.Equal(t, 0.0, allocs)
}
