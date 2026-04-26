package headingstyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestHeadingLine_ManualATXWithTextChild(t *testing.T) {
	// A manually constructed ATX heading with Lines().Len()==0 and a Text child;
	// headingLine should fall back to the child segment offset (line 1).
	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	h := ast.NewHeading(1)
	textNode := ast.NewText()
	textNode.Segment = text.NewSegment(2, 7) // "Title" in "# Title\n"
	h.AppendChild(h, textNode)

	line := headingLine(h, f)
	if line < 1 {
		t.Errorf("expected headingLine >= 1, got %d", line)
	}
}

func TestHeadingLine_ManualATXWithEmphasisChild(t *testing.T) {
	// Heading with Lines==0 and first child is Emphasis wrapping Text;
	// headingLine should still return a valid line number (>= 1).
	src := []byte("# **bold**\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	h := ast.NewHeading(1)
	em := ast.NewEmphasis(2)
	textNode := ast.NewText()
	textNode.Segment = text.NewSegment(3, 7)
	em.AppendChild(em, textNode)
	h.AppendChild(h, em)

	line := headingLine(h, f)
	if line < 1 {
		t.Errorf("expected headingLine >= 1, got %d", line)
	}
}
