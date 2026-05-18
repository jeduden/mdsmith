package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
)

// parseRoot parses src with the canonical parser and returns the
// document root, mirroring how the linter and sync-docs reach the AST.
func parseRoot(src string) ast.Node {
	return ParseContext([]byte(src), parser.NewContext())
}

// findPINodes returns all ProcessingInstruction nodes in the AST,
// searching the full tree recursively.
func findPINodes(root ast.Node) []*ProcessingInstruction {
	var nodes []*ProcessingInstruction
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if pi, ok := n.(*ProcessingInstruction); ok {
			nodes = append(nodes, pi)
		}
		return ast.WalkContinue, nil
	})
	return nodes
}

func TestPI_BasicSingleLine(t *testing.T) {
	pis := findPINodes(parseRoot("<?foo?>\n"))
	require.Len(t, pis, 1)
	assert.Equal(t, "foo", pis[0].Name)
	assert.True(t, pis[0].HasClosure(), "expected HasClosure() == true for single-line PI")
}

func TestPI_MultiLine(t *testing.T) {
	pis := findPINodes(parseRoot("<?foo\nbar\n?>\n"))
	require.Len(t, pis, 1)
	assert.Equal(t, "foo", pis[0].Name)
	assert.True(t, pis[0].HasClosure(), "expected HasClosure() == true")
}

func TestPI_MultiLineEmptyBody(t *testing.T) {
	pis := findPINodes(parseRoot("<?foo\n?>\n"))
	require.Len(t, pis, 1)
	assert.True(t, pis[0].HasClosure(), "expected HasClosure() == true")
}

func TestPI_SlashName(t *testing.T) {
	pis := findPINodes(parseRoot("<?/include?>\n"))
	require.Len(t, pis, 1)
	assert.Equal(t, "/include", pis[0].Name)
}

func TestPI_HTMLCommentStillHTMLBlock(t *testing.T) {
	root := parseRoot("<!-- comment -->\n")
	pis := findPINodes(root)
	assert.Empty(t, pis, "expected 0 PI nodes for HTML comment")
	found := false
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		if _, ok := n.(*ast.HTMLBlock); ok {
			found = true
		}
	}
	assert.True(t, found, "expected HTML comment to be parsed as HTMLBlock")
}

func TestPI_DivStillHTMLBlock(t *testing.T) {
	pis := findPINodes(parseRoot("<div>\nhello\n</div>\n"))
	assert.Empty(t, pis, "expected 0 PI nodes for div")
}

func TestPI_InsideFencedCodeBlock(t *testing.T) {
	pis := findPINodes(parseRoot("```\n<?foo?>\n```\n"))
	assert.Empty(t, pis, "expected 0 PI nodes inside code block")
}

func TestPI_FourSpaceIndent(t *testing.T) {
	pis := findPINodes(parseRoot("    <?foo?>\n"))
	assert.Empty(t, pis, "expected 0 PI nodes for 4-space indent")
}

func TestPI_OneToThreeSpaceIndent(t *testing.T) {
	for _, spaces := range []string{" ", "  ", "   "} {
		pis := findPINodes(parseRoot(spaces + "<?foo?>\n"))
		assert.Len(t, pis, 1, "indent %q: expected 1 PI", spaces)
	}
}

func TestPI_Unterminated(t *testing.T) {
	pis := findPINodes(parseRoot("<?foo\nbar"))
	require.Len(t, pis, 1)
	assert.False(t, pis[0].HasClosure(), "expected HasClosure() == false for unterminated PI")
}

func TestPI_MultiplePIs(t *testing.T) {
	pis := findPINodes(parseRoot("<?foo?>\n<?bar?>\n"))
	require.Len(t, pis, 2)
	assert.Equal(t, "foo", pis[0].Name)
	assert.Equal(t, "bar", pis[1].Name)
}

func TestPI_InterruptsParagraph(t *testing.T) {
	pis := findPINodes(parseRoot("some text\n<?foo?>\n"))
	require.Len(t, pis, 1)
}

func TestPI_WhitespaceOnlyBody(t *testing.T) {
	pis := findPINodes(parseRoot("<?foo\n   \n?>\n"))
	require.Len(t, pis, 1)
	assert.True(t, pis[0].HasClosure(), "expected HasClosure() == true")
}

func TestPI_InsideBlockquote(t *testing.T) {
	pis := findPINodes(parseRoot("> <?foo?>\n"))
	assert.Empty(t, pis, "expected 0 PI nodes inside blockquote")
}

func TestPI_EmptyName(t *testing.T) {
	for _, src := range []string{"<??>", "<? ?>"} {
		pis := findPINodes(parseRoot(src + "\n"))
		assert.Empty(t, pis, "input %q: expected 0 PI nodes for empty name", src)
	}
}

func TestPI_ConsecutiveWithoutBlankLine(t *testing.T) {
	pis := findPINodes(parseRoot("<?foo?>\n<?bar?>\n"))
	require.Len(t, pis, 2)
}

func TestPI_SingleLineClosesInOpen(t *testing.T) {
	pis := findPINodes(parseRoot("<?foo?>\n"))
	require.Len(t, pis, 1)
	assert.True(t, pis[0].HasClosure(), "expected HasClosure() == true for single-line PI")
}

func TestPI_SingleLineWithTrailingContent(t *testing.T) {
	pis := findPINodes(parseRoot("<?foo?> trailing\n"))
	require.Len(t, pis, 1)
	assert.True(t, pis[0].HasClosure(), "expected HasClosure() == true for PI with trailing content")
	assert.Equal(t, "foo", pis[0].Name)
}

func TestPI_MultiPIs(t *testing.T) {
	pis := findPINodes(parseRoot("<?foo?>\n\n<?bar\nbaz\n?>\n"))
	require.Len(t, pis, 2)
	assert.Equal(t, "foo", pis[0].Name)
	assert.Equal(t, "bar", pis[1].Name)
}

func TestKind_ProcessingInstruction(t *testing.T) {
	pi := &ProcessingInstruction{Name: "test"}
	assert.Equal(t, KindProcessingInstruction, pi.Kind())
	assert.NotEqual(t, ast.KindDocument, pi.Kind())
}

func TestDump_ProcessingInstruction(t *testing.T) {
	pi := &ProcessingInstruction{Name: "catalog"}
	pi.Dump([]byte("<?catalog?>"), 0)
}

func TestPI_Dump_WithSource(t *testing.T) {
	pi := &ProcessingInstruction{Name: "catalog"}
	pi.Dump([]byte("<?catalog\nglob: docs\n?>"), 1)
}

func TestPI_Dump_EmptySource(t *testing.T) {
	pi := &ProcessingInstruction{Name: "test"}
	pi.Dump([]byte{}, 0)
}

func TestIsRaw_ProcessingInstruction(t *testing.T) {
	pi := &ProcessingInstruction{Name: "test"}
	assert.True(t, pi.IsRaw())
}

func TestHasClosure_NoClosureLine(t *testing.T) {
	pi := &ProcessingInstruction{Name: "test"}
	assert.False(t, pi.HasClosure())
}

func TestNewPIBlockParser(t *testing.T) {
	p := NewPIBlockParser()
	require.NotNil(t, p)
	assert.Equal(t, []byte{'<'}, p.Trigger())
}

func TestPIBlockParser_Close(t *testing.T) {
	p := NewPIBlockParser()
	p.Close(&ProcessingInstruction{Name: "test"}, nil, nil)
}

func TestPIBlockParser_CanInterruptParagraph(t *testing.T) {
	assert.True(t, NewPIBlockParser().CanInterruptParagraph())
}

func TestPIBlockParser_CanAcceptIndentedLine(t *testing.T) {
	assert.False(t, NewPIBlockParser().CanAcceptIndentedLine())
}

func TestPIBlockParser_Trigger(t *testing.T) {
	assert.Equal(t, []byte{'<'}, NewPIBlockParser().Trigger())
}

func TestPIBlockParserPrioritized(t *testing.T) {
	pv := PIBlockParserPrioritized()
	assert.Equal(t, 850, pv.Priority)
	assert.NotNil(t, pv.Value)
}

func TestExtractPINameBytes(t *testing.T) {
	assert.Equal(t, "foo", string(extractPINameBytes([]byte("foo?>"))))
	assert.Equal(t, "catalog", string(extractPINameBytes([]byte("catalog key=val"))))
	assert.Equal(t, "/include", string(extractPINameBytes([]byte("/include?>"))))
	assert.Equal(t, "", string(extractPINameBytes([]byte("?>"))))
}
