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
}

// extOneColumnLines are one-column delimiter rows. They build a table
// after a header line with no pipe, such as "aa".
var extOneColumnLines = []string{":-", "-:", ":-:", "|-|", "| - |", "|--", "--|"}

// extContinuingLines stay paragraph text under every extension.
var extContinuingLines = []string{
	// Not delimiter rows: a cell without '-', text, or a bare pipe.
	"|", "||", "|:|", "-||-", "-|x", "a|-", "|-|-|x", ":", "::", "-:-", "-- -|-",
	"    |-|-|",
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
