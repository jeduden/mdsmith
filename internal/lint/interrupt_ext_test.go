package lint

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/jeduden/mdsmith/pkg/markdown/flavor"
	"github.com/stretchr/testify/assert"
)

// extInterruptingLines end a paragraph under a Markdown extension that
// the canonical parser leaves off. The parser check puts each after the
// header line "aa | bb", which has two cells, as each row here does.
var extInterruptingLines = []string{
	// GFM table delimiter rows: cells of '-' runs with optional colons,
	// split by '|', with optional outer pipes.
	"|-|-|", "-|-", "--- | ---", ":--|--:", "| :-: | - |", "   |-|-|",
	"-|-|", "|-|-", " - | - ",
	// Footnote definitions: "[^", a label that is not blank and has no
	// unescaped bracket, then "]:".
	"[^1]: x", "[^1]:x", "[^1]:", "[^a b]: x", "   [^note]: x", `[^a\]b]: x`, `[^a\[b]: x`, `[^a\b]: x`,
	// Definition-list descriptions: ':' then a space or tab.
	": x", ":\tx", ":  x", ": ",
}

// extSpecOnlyLines interrupt a paragraph under PHP Markdown Extra and
// markdown-it, which allow up to three spaces before a definition's ':',
// but not under goldmark, which wants the ':' at the line start.
var extSpecOnlyLines = []string{" : x", "   : x"}

// extOneColumnLines are one-column delimiter rows. They build a table
// after a header line with no pipe, such as "aa".
var extOneColumnLines = []string{":-", "-:", ":-:", "|-|", "| - |", "|--", "--|"}

// extContinuingLines stay paragraph text under every extension.
var extContinuingLines = []string{
	// Not delimiter rows: a cell without '-', text, or a bare pipe.
	"|", "||", "|:|", "-||-", "-|x", "a|-", "|-|-|x", ":", "::", "-:-", "-- -|-",
	"    |-|-|",
	// Not footnote definitions: a blank label, a bracket in the label,
	// no colon right after the label, or a link reference definition,
	// which cannot interrupt a paragraph.
	"[^]: x", "[^ ]: x", "[^a[b]: x", "[^1] x", "[^1]", "[^1] : x", "[1]: x",
	"^1]: x", "    [^1]: x",
	// Not definition-list lines: no space or tab after the ':', or four
	// columns of indent.
	":x", ":", "::", ":-x", "    : x",
}

// flavorKeepsParagraph reports whether the flavor parser keeps both
// lines of first+"\n"+line in one paragraph.
func flavorKeepsParagraph(first, line string) bool {
	src := []byte(first + "\n" + line + "\n")
	var kept bool
	flavor.WithSharedParser(func(p parser.Parser) {
		doc := p.Parse(text.NewReader(src))
		c := doc.FirstChild()
		kept = c != nil && c.NextSibling() == nil && c.Kind() == ast.KindParagraph && c.Lines().Len() == 2
	})
	return kept
}

func TestExtensionInterruptsParagraph(t *testing.T) {
	for _, line := range append(append([]string{}, extInterruptingLines...), extOneColumnLines...) {
		assert.True(t, ExtensionInterruptsParagraph([]byte(line)), "%q must interrupt a paragraph", line)
	}
	for _, line := range extContinuingLines {
		assert.False(t, ExtensionInterruptsParagraph([]byte(line)), "%q must continue a paragraph", line)
	}
	for _, line := range extSpecOnlyLines {
		assert.True(t, ExtensionInterruptsParagraph([]byte(line)), "%q must interrupt a paragraph", line)
	}
}

// TestExtensionInterruptsParagraph_MatchesFlavorParser checks each table
// entry against the flavor parser, which enables every extension.
func TestExtensionInterruptsParagraph_MatchesFlavorParser(t *testing.T) {
	for _, line := range extInterruptingLines {
		assert.False(t, flavorKeepsParagraph("aa | bb", line), "parser keeps %q in the paragraph", line)
	}
	for _, line := range extOneColumnLines {
		assert.False(t, flavorKeepsParagraph("aa", line), "parser keeps %q in the paragraph", line)
	}
	for _, line := range extContinuingLines {
		assert.True(t, flavorKeepsParagraph("aa | bb", line), "parser ends the paragraph at %q", line)
		assert.True(t, flavorKeepsParagraph("aa", line), "parser ends the paragraph at %q", line)
	}
	for _, line := range extSpecOnlyLines {
		assert.True(t, flavorKeepsParagraph("aa", line), "parser ends the paragraph at %q", line)
	}
}

// TestIsTableDelimiterRow pins the edges of the row shape: a line of '-'
// alone is left to the setext and thematic-break checks, and four
// columns of indent make indented code.
func TestIsTableDelimiterRow(t *testing.T) {
	for _, line := range []string{"|-|", "-|", "-|-", ":-", "  --- | ---\t", "|:-:|"} {
		assert.True(t, isTableDelimiterRow([]byte(line)), "%q", line)
	}
	for _, line := range []string{"", "-", "---", "|", "    |-|", "|x|", "-|x", "|-||"} {
		assert.False(t, isTableDelimiterRow([]byte(line)), "%q", line)
	}
}

func TestIsDelimiterCell(t *testing.T) {
	for _, cell := range []string{"-", " --- ", ":-", "-:", "\t:-:\t"} {
		assert.True(t, isDelimiterCell([]byte(cell)), "%q", cell)
	}
	for _, cell := range []string{"", " ", ":", "::", "-:-", ": -", "- -", "x"} {
		assert.False(t, isDelimiterCell([]byte(cell)), "%q", cell)
	}
}

// TestIsFootnoteDefinition pins the label scan: a backslash escapes only
// ASCII punctuation, and a line that ends inside the label is no
// definition.
func TestIsFootnoteDefinition(t *testing.T) {
	for _, line := range []string{"[^1]:", `[^a\b]: x`, `[^a\[b]: x`, "   [^x y]:z"} {
		assert.True(t, isFootnoteDefinition([]byte(line)), "%q", line)
	}
	for _, line := range []string{"[^1", `[^a\`, `[^a\]`, "[^]:", "[^\t]:", "[^1]", "[^1]x", "    [^1]:", "[1]:"} {
		assert.False(t, isFootnoteDefinition([]byte(line)), "%q", line)
	}
}

func TestIsDefinitionDescription(t *testing.T) {
	for _, line := range []string{": x", ":\tx", ": ", "   : x"} {
		assert.True(t, isDefinitionDescription([]byte(line)), "%q", line)
	}
	for _, line := range []string{"", ":", ":x", "::", "    : x", "x: y", "\t: x"} {
		assert.False(t, isDefinitionDescription([]byte(line)), "%q", line)
	}
}

// TestStartsListItem checks each line after a footnote definition's
// first line, under the flavor parser. Lazy continuation keeps a line in
// the footnote's paragraph unless it opens a block, and there every list
// marker opens a list, empty or not, whatever its number.
func TestStartsListItem(t *testing.T) {
	items := []string{
		"- x", "-", "+ x", "*", "1. x", "1.", "2. x", "2)", "1999. x", "1999.", "10) x", "   2. x", "2.\tx",
	}
	plain := []string{"x", "-x", "2.x", "2:", "**", "***", "    2. x", "1234567890. x", "#", ".", ""}
	opensList := func(line string) bool {
		src := []byte("x[^1]\n\n[^1]: a\n" + line + "\n")
		found := false
		flavor.WithSharedParser(func(p parser.Parser) {
			_ = ast.Walk(p.Parse(text.NewReader(src)), func(n ast.Node, entering bool) (ast.WalkStatus, error) {
				found = found || entering && n.Kind() == ast.KindList
				return ast.WalkContinue, nil
			})
		})
		return found
	}
	for _, line := range items {
		assert.True(t, StartsListItem([]byte(line)), "%q", line)
		assert.True(t, opensList(line), "%q must open a list in the footnote", line)
	}
	for _, line := range plain {
		assert.False(t, StartsListItem([]byte(line)), "%q", line)
		if line != "***" { // a thematic break, not a list
			assert.False(t, opensList(line), "%q must stay in the footnote", line)
		}
	}
}

func TestEmptyFootnoteDefinition(t *testing.T) {
	for _, line := range []string{"[^1]:", "[^1]:  \t", "[^a b]:", `   [^a\]b]:`} {
		assert.True(t, EmptyFootnoteDefinition([]byte(line)), "%q", line)
	}
	for _, line := range []string{"[^1]: x", "[^1]:x", "[^a b]: x", `[^a\]b]: x`, "[^1]", ": ", "text", ""} {
		assert.False(t, EmptyFootnoteDefinition([]byte(line)), "%q", line)
	}
}
