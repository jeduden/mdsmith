package astutil

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/yuin/goldmark/ast"
)

// HeadingLine returns the 1-based source line of a heading node.
// Setext headings expose their line via Lines(); ATX headings are found
// by walking inline descendants until the first text segment. Returns 1
// as a safe fallback.
func HeadingLine(heading *ast.Heading, f *lint.File) int {
	lines := heading.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}

	line := 1
	_ = ast.Walk(heading, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n == heading {
			return ast.WalkContinue, nil
		}
		t, ok := n.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		line = f.LineOfOffset(t.Segment.Start)
		return ast.WalkStop, nil
	})

	return line
}

// ParagraphLine returns the 1-based source line of a paragraph node.
func ParagraphLine(para *ast.Paragraph, f *lint.File) int {
	lines := para.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	return 1
}

// IsTable reports whether a paragraph node is actually a GFM table
// (goldmark parses tables as paragraphs when the table extension is
// absent).  It checks whether the first line starts with "|".
func IsTable(para *ast.Paragraph, f *lint.File) bool {
	lines := para.Lines()
	if lines.Len() == 0 {
		return false
	}
	seg := lines.At(0)
	return bytes.HasPrefix(bytes.TrimSpace(f.Source[seg.Start:seg.Stop]), []byte("|"))
}

// HeadingText returns the plain-text content of a heading by
// recursively extracting all text segments from its children.
func HeadingText(heading *ast.Heading, source []byte) string {
	var buf bytes.Buffer
	for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
		ExtractText(c, source, &buf)
	}
	return buf.String()
}

// ExtractText recursively writes the text content of n and its
// descendants into buf.
func ExtractText(n ast.Node, source []byte, buf *bytes.Buffer) {
	if t, ok := n.(*ast.Text); ok {
		buf.Write(t.Segment.Value(source))
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		ExtractText(c, source, buf)
	}
}
