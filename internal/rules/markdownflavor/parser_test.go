package markdownflavor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

func TestParserCachesSingleInstance(t *testing.T) {
	p1 := Parser()
	p2 := Parser()
	assert.Same(t, p1, p2, "Parser() must return the cached instance")
}

func TestParserDetectsTables(t *testing.T) {
	src := []byte("| a | b |\n| - | - |\n| 1 | 2 |\n")
	doc := parseSource(t, src)
	assert.True(t, containsKind(doc, extast.KindTable),
		"expected table node in dual-parser AST")
}

func TestParserDetectsStrikethrough(t *testing.T) {
	src := []byte("hello ~~world~~\n")
	doc := parseSource(t, src)
	assert.True(t, containsKind(doc, extast.KindStrikethrough),
		"expected strikethrough node in dual-parser AST")
}

func TestParserDetectsTaskList(t *testing.T) {
	src := []byte("- [ ] todo\n- [x] done\n")
	doc := parseSource(t, src)
	assert.True(t, containsKind(doc, extast.KindTaskCheckBox),
		"expected task-list checkbox node in dual-parser AST")
}

func TestParserDetectsFootnote(t *testing.T) {
	src := []byte("A paragraph.[^1]\n\n[^1]: footnote body\n")
	doc := parseSource(t, src)
	assert.True(t, containsKind(doc, extast.KindFootnoteLink),
		"expected footnote link node in dual-parser AST")
}

func TestParserDetectsDefinitionList(t *testing.T) {
	src := []byte("term\n:   definition\n")
	doc := parseSource(t, src)
	assert.True(t, containsKind(doc, extast.KindDefinitionList),
		"expected definition-list node in dual-parser AST")
}

func TestParserDetectsLinkifiedAutolink(t *testing.T) {
	src := []byte("See https://example.com for details.\n")
	doc := parseSource(t, src)
	assert.True(t, containsKind(doc, ast.KindAutoLink),
		"expected auto-link node for bare URL in dual-parser AST")
}

func TestParserDetectsHeadingAttribute(t *testing.T) {
	src := []byte("# Heading {#custom-id}\n")
	doc := parseSource(t, src)
	// The heading attribute parser stores {#id} as an attribute on the
	// Heading node, not as a separate child.
	found := false
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok && h.Attributes() != nil {
			if _, ok := h.AttributeString("id"); ok {
				found = true
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	assert.True(t, found, "expected heading id attribute in dual-parser AST")
}

// parseSource invokes Parser().Parse on the given source and returns
// the resulting document node. Helper shared by parser detection tests.
func parseSource(t *testing.T, src []byte) ast.Node {
	t.Helper()
	p := Parser()
	doc := p.Parser().Parse(text.NewReader(src))
	require.NotNil(t, doc)
	return doc
}

// containsKind walks the tree rooted at root and reports whether any
// node has the given kind.
func containsKind(root ast.Node, kind ast.NodeKind) bool {
	found := false
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n.Kind() == kind {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}
