package include

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/markdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// htmlHeadingCases pairs a snippet with a note on what the canonical
// parser builds. Every case is checked against the AST, so the notes
// are documentation, not the oracle.
var htmlHeadingCases = []struct {
	src  string
	note string
}{
	{"<!-- x -->\n---\n", "type 2 closes on its line; --- is a break"},
	{"<!-- x -->\n===\n", "type 2 closes; === is paragraph text"},
	{"<?toc?>\n---\n", "single-line PI; --- is a break"},
	{"<?toc\nfoo ?>\n---\n?>\n", "multi-line PI swallows --- until ?>"},
	{"<? x\n---\n", "nameless <? is HTML type 3, unclosed"},
	{"<? x ?>\n---\n", "type 3 closes on its line"},
	{"<div>\n---\n", "type 6 swallows --- until a blank line"},
	{"<DIV>\n---\n", "type 6 tag names are case-insensitive"},
	{"</div>\n---\n", "type 6 closing tag"},
	{"<div\t>\n---\n", "fork: a tab after the tag name is no type-6 start"},
	{"<div/\n---\n", "fork: a lone / after the tag name is no start"},
	{"<span>Title</span>\n---\n", "type 7 needs the tag alone; setext h2"},
	{"<span>Title</span>\n===\n", "setext h1"},
	{"<span>\n---\n", "type 7 open tag alone"},
	{"</span>\n---\n", "type 7 closing tag alone"},
	{"<span>\t\n---\n", "fork: only spaces may trail a type-7 tag"},
	{"<span =x>\n---\n", "malformed attribute; no type 7"},
	{"</span a>\n---\n", "closing tag with attributes; no type 7"},
	{"<span a=b c='d' e=\"f\" g>\n---\n", "type 7 with attributes"},
	{"<custom-tag>\n---\n", "type 7 custom element"},
	{"<x-y/>\n---\n", "type 7 self-closing"},
	{"<1div>\n---\n", "a tag name starts with a letter"},
	{"<script>\n---\n", "type 1 swallows --- until </script>"},
	{"<Script>\nx\n</SCRIPT>\n---\n", "type 1 closes case-insensitively"},
	{"<script>x</script>\n---\n", "type 1 closes on its opening line"},
	{"<pre\n---\n", "type 1 name at line end"},
	{"</script>\n---\n", "fork: raw-text closing tag opens no block"},
	{"<!DOCTYPE html>\n---\n", "type 4 closes on its line"},
	{"<!doctype html>\n---\n", "fork: type 4 needs an upper-case letter"},
	{"<!X\n>\n---\n", "type 4 closes on a later >"},
	{"<![CDATA[x]]>\n---\n", "type 5 closes on its line"},
	{"<![CDATA[\n]]>\n---\n", "type 5 closes on a later ]]>"},
	{"<!-- a\nb -->\n---\n", "type 2 closes on a later -->"},
	{"<!-- x\n# H\n-->\n# I\n", "ATX inside a comment is no heading"},
	{"<div>\n# H\n", "ATX inside a type-6 block is no heading"},
	{"<div>\ntext\n---\n", "type 6 block holds the underline"},
	{"<div>\n\nTitle\n---\n", "blank line ends type 6"},
	{"  <!-- x -->\n---\n", "three-space indent still opens"},
	{"<!-- x -->\r\n---\r\n", "CRLF"},
	{"<span>\r\n---\r\n", "CRLF type 7"},
	{"para\n<!-- x -->\n---\n", "type 2 interrupts a paragraph"},
	{"para\n<?toc?>\n---\n", "PI interrupts a paragraph"},
	{"para\n<div>\n---\n", "type 6 interrupts a paragraph"},
	{"para\n</div>\n---\n", "type 6 closing tag interrupts too"},
	{"para\n<script>\n---\n", "type 1 interrupts a paragraph"},
	{"para\n<custom-tag>\n---\n", "type 7 cannot interrupt; setext"},
	{"para\n<span>\n---\n", "type 7 cannot interrupt; setext"},
	{"x\n<span>Title</span>\n---\n", "paragraph continues; setext"},
	{"<!-- a -->\nTitle\n---\n", "heading after a closed comment"},
	{"<?toc?>\nTitle\n---\n", "heading after a closed PI"},
	{"<!DOCTYPE html>\nTitle\n---\n", "heading after a closed declaration"},
}

// astHasHeading reports whether the canonical parser builds a heading
// node for src.
func astHasHeading(src string) bool {
	doc := markdown.Parse([]byte(src))
	found := false
	_ = ast.Walk(doc.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindHeading {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

// TestFindMinHeadingLevel_HTMLMatchesParser checks that the include
// heading scan finds a heading in each snippet exactly when the
// canonical parser does.
func TestFindMinHeadingLevel_HTMLMatchesParser(t *testing.T) {
	for _, tc := range htmlHeadingCases {
		t.Run(tc.note, func(t *testing.T) {
			want := astHasHeading(tc.src)
			got := findMinHeadingLevel(strings.Split(tc.src, "\n")) > 0
			assert.Equal(t, want, got, "%q (%s)", tc.src, tc.note)
		})
	}
}

// TestAdjustHeadings_HTMLLineIsNotSetextText pins the rewrite itself:
// an HTML or PI line followed by an underline is left alone, while a
// paragraph line made of inline HTML is still a setext heading.
func TestAdjustHeadings_HTMLLineIsNotSetextText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"<!-- note -->\n---\n\n## A\n", "<!-- note -->\n---\n\n### A\n"},
		{"<?toc?>\n===\n\n## A\n", "<?toc?>\n===\n\n### A\n"},
		{"<div>\n---\n\n## A\n", "<div>\n---\n\n### A\n"},
		{"<span>Title</span>\n---\n", "### <span>Title</span>\n"},
		{"<div>\n# A\n</div>\n\n## B\n", "<div>\n# A\n</div>\n\n### B\n"},
		{"<?catalog\nglob: x\n?>\n## A\n<?/catalog?>\n", "<?catalog\nglob: x\n?>\n### A\n<?/catalog?>\n"},
	}
	for _, tt := range tests {
		require.True(t, strings.HasSuffix(tt.in, "\n"))
		assert.Equal(t, tt.want, adjustHeadings(tt.in, 2), "%q", tt.in)
	}
}
