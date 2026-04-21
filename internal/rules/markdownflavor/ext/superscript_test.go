package ext

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func walkFindKind(root ast.Node, kind ast.NodeKind) ast.Node {
	var found ast.Node
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == kind {
			found = n
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

func parseWith(t *testing.T, src string, exts ...goldmark.Extender) ast.Node {
	t.Helper()
	md := goldmark.New(goldmark.WithExtensions(exts...))
	doc := md.Parser().Parse(text.NewReader([]byte(src)))
	require.NotNil(t, doc)
	return doc
}

// countKind returns the number of nodes of the given kind anywhere
// in the tree rooted at root.
func countKind(root ast.Node, kind ast.NodeKind) int {
	n := 0
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && node.Kind() == kind {
			n++
		}
		return ast.WalkContinue, nil
	})
	return n
}

func TestSuperscriptParses(t *testing.T) {
	doc := parseWith(t, "x^2^ is fine.\n", Superscript)
	assert.NotNil(t, walkFindKind(doc, KindSuperscript),
		"expected Superscript node for x^2^")
}

func TestSuperscriptSingleCharOnly(t *testing.T) {
	// Double carets must not produce a Superscript node.
	doc := parseWith(t, "x^^2^^\n", Superscript)
	assert.Nil(t, walkFindKind(doc, KindSuperscript),
		"^^...^^ must not match superscript")
}

func TestSuperscriptUnbalancedCaret(t *testing.T) {
	// A lone `^` with no closing pair must not produce a node.
	doc := parseWith(t, "a^b c\n", Superscript)
	assert.Nil(t, walkFindKind(doc, KindSuperscript))
}

func TestSuperscriptContainsContent(t *testing.T) {
	src := []byte("E = mc^2^\n")
	doc := parseWith(t, string(src), Superscript)
	node := walkFindKind(doc, KindSuperscript)
	require.NotNil(t, node)
	// The child should carry the "2" text.
	child, ok := node.FirstChild().(*ast.Text)
	require.True(t, ok, "superscript first child should be a Text node")
	assert.Equal(t, "2", string(child.Segment.Value(src)))
}

func TestSuperscriptInsideCodeIsIgnored(t *testing.T) {
	doc := parseWith(t, "see `x^2^` here.\n", Superscript)
	assert.Nil(t, walkFindKind(doc, KindSuperscript),
		"content inside a code span must not be parsed as superscript")
}
