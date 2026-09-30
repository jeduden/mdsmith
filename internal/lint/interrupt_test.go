package lint

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// interruptingLines end an open paragraph when they follow one of its
// lines: a blank line, a setext underline, or a block start that
// CommonMark (plus mdsmith's `<?name` processing instructions) lets
// interrupt a paragraph.
var interruptingLines = []string{
	"", "   ",
	// ATX heading: 1-6 '#' then a space, tab, or end of line.
	"#", "# x", "###### x", "#\tx", "   ## x",
	// Thematic break, spaced or not.
	"---", "***", "___", "- - -", "* * *", "_ _ _", "-- -", "** *",
	// Fenced code, with or without an info string.
	"```", "````", "```go", "~~~", "~~~go", "~~~ `x`",
	// Block quote.
	">", ">x", "> x", "   > x",
	// Bullet list item with content.
	"- x", "+ x", "* x", "-\tx", "   - x",
	// Ordered list item starting at 1, with content.
	"1. x", "1) x", "01. x", "001) x", "1.\tx",
	// Setext underline.
	"=", "===", "=== ", "-", "--", "--  ",
	// HTML blocks of types 1-6, including processing instructions.
	"<div", "<div>", "</div>", "<DIV class=x>", "<p/>", "<script",
	"<pre>", "<!-- c", "<?pi", "<? x", "<!DOCTYPE", "<![CDATA[x",
	// The goldmark fork also reads spaces between "</" and a block
	// tag name as an HTML block opener.
	"</ div", "</  p>", "</ div x",
}

// specOnlyInterrupting interrupt a paragraph under the CommonMark spec
// but not under the goldmark fork. The fork still requires an uppercase
// letter after "<!" (the spec before 0.30), and a space rather than a tab
// after a block tag name. InterruptsParagraph follows the spec, so reflow
// never relies on the fork's narrower reading.
var specOnlyInterrupting = []string{"<!doctype html", "<div\tx"}

// continuingLines stay paragraph text after a paragraph line.
var continuingLines = []string{
	// '#' not followed by a space, or more than six.
	"#48", "#48](x),", "#tag", "##x", "####### x",
	// '=' or '-' runs with other text on the line.
	"= y", "== x", "-- x", "--- x", "*** x", "___ x", "_ _ x",
	// Marker characters not followed by a space.
	"-x", "+x", "*x", "**bold**", "**", "++", "__",
	// Empty list items cannot interrupt a paragraph.
	"*", "+", "* ", "1.", "1)", "1. ",
	// Ordered items that do not start at 1.
	"2. x", "12) x", "0. x", "10. x", "1999. Then",
	// Numbers that are not list markers.
	"1.5", "2024",
	// A backtick fence cannot carry a backtick in its info string.
	"``", "``x", "```go `x`",
	// HTML type 7 and non-block tags cannot interrupt.
	"<span>", "<span", "<a href=x>", "<divx", "<", "< div",
	// A spaced close tag needs a block tag name, then a space, ">",
	// "/>", or the line end.
	"</ span", "</ divx", "</\tdiv", "</ div\tx",
	// Four columns of indent make indented code, which cannot interrupt.
	"    # x", "    > x", "    ```", "    - x", "\t# x", "   \t> x",
	"plain text",
}

func TestInterruptsParagraph(t *testing.T) {
	for _, line := range interruptingLines {
		assert.True(t, InterruptsParagraph([]byte(line)), "%q must interrupt a paragraph", line)
	}
	for _, line := range continuingLines {
		assert.False(t, InterruptsParagraph([]byte(line)), "%q must continue a paragraph", line)
	}
	for _, line := range specOnlyInterrupting {
		assert.True(t, InterruptsParagraph([]byte(line)), "%q must interrupt a paragraph", line)
	}
}

// TestInterruptsParagraph_MatchesParser checks each table entry against
// the canonical parser: the entry follows a paragraph line, and the
// paragraph keeps both lines exactly when InterruptsParagraph is false.
// The spec-only entries are the exception: the parser keeps them in the
// paragraph.
func TestInterruptsParagraph_MatchesParser(t *testing.T) {
	check := func(line string, want bool) {
		f, err := NewFile("t.md", []byte("para\n"+line+"\n"))
		require.NoError(t, err)
		first := f.AST.FirstChild()
		require.NotNil(t, first)
		kept := first.Kind() == ast.KindParagraph && first.Lines().Len() == 2
		assert.Equal(t, want, !kept, "parser disagrees on %q", line)
	}
	for _, line := range interruptingLines {
		check(line, true)
	}
	for _, line := range continuingLines {
		check(line, false)
	}
	for _, line := range specOnlyInterrupting {
		check(line, false)
	}
}

// TestStartsInterruptingBlock checks that it agrees with
// InterruptsParagraph on every table entry except a blank line and a
// setext underline, which only end a paragraph in its own container.
// "---" is a thematic break as well as an underline, so it still counts.
func TestStartsInterruptingBlock(t *testing.T) {
	ownContainerOnly := map[string]bool{
		"": true, "   ": true, "=": true, "===": true, "=== ": true,
		"-": true, "--": true, "--  ": true,
	}
	for _, line := range interruptingLines {
		want := !ownContainerOnly[line]
		assert.Equal(t, want, StartsInterruptingBlock([]byte(line)), "%q", line)
	}
	for _, line := range continuingLines {
		assert.False(t, StartsInterruptingBlock([]byte(line)), "%q", line)
	}
	assert.True(t, StartsInterruptingBlock([]byte("---")))
}

// TestForkSpacedCloseTag covers the one HTML case the fork reads more
// broadly than the spec: spaces between "</" and a type-6 tag name.
func TestForkSpacedCloseTag(t *testing.T) {
	for _, line := range []string{"</ div", "</  p>", "   </ div/>", "</ DIV x"} {
		assert.True(t, forkSpacedCloseTag([]byte(line)), "%q", line)
	}
	for _, line := range []string{"</div", "    </ div", "</ span", "</ divx", "</\tdiv", "</ div\tx", "plain"} {
		assert.False(t, forkSpacedCloseTag([]byte(line)), "%q", line)
	}
}

func TestInterruptingOrderedMarker(t *testing.T) {
	cases := map[string]bool{
		"1. x": true, "1) x": true, "01. x": true, "1.\tx": true,
		"1.": false, "1. ": false, "2. x": false, "10. x": false, "0. x": false, "x": false,
	}
	for line, want := range cases {
		assert.Equal(t, want, interruptingOrderedMarker([]byte(line), 0), "%q", line)
	}
	assert.True(t, interruptingOrderedMarker([]byte("   1. x"), 3))
}
