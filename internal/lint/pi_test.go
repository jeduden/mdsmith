package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

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
	src := "<?foo?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 1)
	assert.Equal(t, "foo", pis[0].Name)
	assert.True(t, pis[0].HasClosure(), "expected HasClosure() == true for single-line PI")
}

func TestPI_MultiLine(t *testing.T) {
	src := "<?foo\nbar\n?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 1)
	assert.Equal(t, "foo", pis[0].Name)
	assert.True(t, pis[0].HasClosure(), "expected HasClosure() == true")
}

func TestPI_MultiLineEmptyBody(t *testing.T) {
	src := "<?foo\n?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 1)
	assert.True(t, pis[0].HasClosure(), "expected HasClosure() == true")
}

func TestPI_SlashName(t *testing.T) {
	src := "<?/include?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 1)
	assert.Equal(t, "/include", pis[0].Name)
}

func TestPI_HTMLCommentStillHTMLBlock(t *testing.T) {
	src := "<!-- comment -->\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	assert.Empty(t, pis, "expected 0 PI nodes for HTML comment")
	// Verify it's an HTMLBlock.
	found := false
	for n := f.AST.FirstChild(); n != nil; n = n.NextSibling() {
		if _, ok := n.(*ast.HTMLBlock); ok {
			found = true
		}
	}
	assert.True(t, found, "expected HTML comment to be parsed as HTMLBlock")
}

func TestPI_DivStillHTMLBlock(t *testing.T) {
	src := "<div>\nhello\n</div>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	assert.Empty(t, pis, "expected 0 PI nodes for div")
}

func TestPI_InsideFencedCodeBlock(t *testing.T) {
	src := "```\n<?foo?>\n```\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	assert.Empty(t, pis, "expected 0 PI nodes inside code block")
}

func TestPI_FourSpaceIndent(t *testing.T) {
	src := "    <?foo?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	assert.Empty(t, pis, "expected 0 PI nodes for 4-space indent")
}

func TestPI_OneToThreeSpaceIndent(t *testing.T) {
	for _, spaces := range []string{" ", "  ", "   "} {
		src := spaces + "<?foo?>\n"
		f, err := NewFile("test.md", []byte(src))
		require.NoError(t, err)
		pis := findPINodes(f.AST)
		assert.Len(t, pis, 1, "indent %q: expected 1 PI", spaces)
	}
}

func TestPI_Unterminated(t *testing.T) {
	src := "<?foo\nbar"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 1)
	assert.False(t, pis[0].HasClosure(), "expected HasClosure() == false for unterminated PI")
}

func TestPI_MultiplePIs(t *testing.T) {
	src := "<?foo?>\n<?bar?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 2)
	assert.Equal(t, "foo", pis[0].Name)
	assert.Equal(t, "bar", pis[1].Name)
}

func TestPI_InterruptsParagraph(t *testing.T) {
	src := "some text\n<?foo?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 1)
}

func TestPI_WhitespaceOnlyBody(t *testing.T) {
	src := "<?foo\n   \n?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 1)
	assert.True(t, pis[0].HasClosure(), "expected HasClosure() == true")
}

func TestPI_InsideBlockquote(t *testing.T) {
	src := "> <?foo?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	assert.Empty(t, pis, "expected 0 PI nodes inside blockquote")
}

func TestPI_EmptyName(t *testing.T) {
	tests := []string{
		"<??>",
		"<? ?>",
	}
	for _, src := range tests {
		f, err := NewFile("test.md", []byte(src+"\n"))
		require.NoError(t, err)
		pis := findPINodes(f.AST)
		assert.Empty(t, pis, "input %q: expected 0 PI nodes for empty name", src)
	}
}

func TestPI_ConsecutiveWithoutBlankLine(t *testing.T) {
	src := "<?foo?>\n<?bar?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 2)
}

func TestPI_SingleLineClosesInOpen(t *testing.T) {
	src := "<?foo?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 1)
	assert.True(t, pis[0].HasClosure(), "expected HasClosure() == true for single-line PI")
}

func TestPI_SingleLineWithTrailingContent(t *testing.T) {
	src := "<?foo?> trailing\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 1)
	assert.True(t, pis[0].HasClosure(), "expected HasClosure() == true for PI with trailing content")
	assert.Equal(t, "foo", pis[0].Name)
}
