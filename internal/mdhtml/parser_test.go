package mdhtml

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/stretchr/testify/assert"
)

// oracleLines are single lines whose HTML-block verdict Open must share
// with the goldmark fork, both at a block start and after a paragraph
// line.
var oracleLines = []string{
	"text", "<", "<1div>", "    <div>", "<!-- x -->", "<!--", "<?toc?>", "<? x",
	"<!DOCTYPE html>", "<!doctype html>", "<!X", "<![CDATA[x]]>",
	"<script>", "<SCRIPT src=a>", "<pre", "<textarea/>", "<scriptx>",
	"</script>", "</pre>", "</style>", "</textarea>",
	"<div>", "<DIV>", "</div>", "</ div>", "<div", "<div class=\"a\">",
	"<div/>", "<div>text", "<div\t>", "<div/", "<divx>", "</div a>",
	"<span>", "</span>", "<custom-tag>", "<x-y/>", "<span>  ", "<span>\t",
	"<span>Title</span>", "<span =x>", "</span a>", "</ span>",
	"<span a=b c='d' e=\"f\" g>", "<span a = b>", "<span a=>", "<span a='b>",
	"<span a=",
	"<a href=x/>", "<span\t a>", "<span a\t>", "<span a >", "<span/ >",
	"<veryveryverylongcustomtagname>", "<blockquotex>", "<h7>",
}

// forkOpensHTML reports whether the fork parses the last line of src as
// an HTML block, with prefix lines before it.
func forkOpensHTML(src string) bool {
	root := goldmark.DefaultParser().Parse(text.NewReader([]byte(src)))
	last := root.LastChild()
	return last != nil && last.Kind() == ast.KindHTMLBlock && last.Lines().Len() == 1
}

func TestOpen_MatchesFork(t *testing.T) {
	for _, line := range oracleLines {
		want := forkOpensHTML(line + "\n")
		assert.Equal(t, want, Open([]byte(line), false) != None, "block start %q", line)
		want = forkOpensHTML("para\n" + line + "\n")
		assert.Equal(t, want, Open([]byte(line), true) != None, "after paragraph %q", line)
	}
}
