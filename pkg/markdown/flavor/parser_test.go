package flavor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/jeduden/mdsmith/pkg/markdown"
)

func TestNewParserDetectsTables(t *testing.T) {
	src := []byte("| a | b |\n| - | - |\n| 1 | 2 |\n")
	doc := parseSource(t, src)
	assert.True(t, containsKind(doc, extast.KindTable),
		"expected table node in dual-parser AST")
}

func TestNewParserDetectsStrikethrough(t *testing.T) {
	src := []byte("hello ~~world~~\n")
	doc := parseSource(t, src)
	assert.True(t, containsKind(doc, extast.KindStrikethrough),
		"expected strikethrough node in dual-parser AST")
}

func TestNewParserDetectsTaskList(t *testing.T) {
	src := []byte("- [ ] todo\n- [x] done\n")
	doc := parseSource(t, src)
	assert.True(t, containsKind(doc, extast.KindTaskCheckBox),
		"expected task-list checkbox node in dual-parser AST")
}

func TestNewParserDetectsFootnote(t *testing.T) {
	src := []byte("A paragraph.[^1]\n\n[^1]: footnote body\n")
	doc := parseSource(t, src)
	assert.True(t, containsKind(doc, extast.KindFootnoteLink),
		"expected footnote link node in dual-parser AST")
}

func TestNewParserDetectsDefinitionList(t *testing.T) {
	src := []byte("term\n:   definition\n")
	doc := parseSource(t, src)
	assert.True(t, containsKind(doc, extast.KindDefinitionList),
		"expected definition-list node in dual-parser AST")
}

// TestNewParserRecognisesPIBlocks guards that the dual parser uses
// the same processing-instruction block parser as pkg/markdown's
// canonical parser so table / list markup embedded inside a
// <?include ... ?> block is not detected as real document markup by
// MDS034.
func TestNewParserRecognisesPIBlocks(t *testing.T) {
	src := []byte("<?include\nfile: x.md\n?>\n")
	doc := parseSource(t, src)
	found := false
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n.Kind() == markdown.KindProcessingInstruction {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.True(t, found,
		"expected ProcessingInstruction node in dual-parser AST")
}

func TestNewParserDetectsHeadingAttribute(t *testing.T) {
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

// TestNewParserWithCustomExtensions exercises the parameterised
// constructor: passing only the Table extension produces a parser
// that recognises tables but not strikethrough.
func TestNewParserWithCustomExtensions(t *testing.T) {
	src := []byte("hello ~~world~~\n\n| a | b |\n| - | - |\n| 1 | 2 |\n")
	p := NewParserWith()
	doc := p.Parse(text.NewReader(src))
	require.NotNil(t, doc)
	assert.False(t, containsKind(doc, extast.KindTable),
		"empty extension list must not enable table parsing")
	assert.False(t, containsKind(doc, extast.KindStrikethrough),
		"empty extension list must not enable strikethrough")
}

// TestNewPooledParserResetIsCallable confirms that the returned reset
// closure is safe to invoke between parses.
func TestNewPooledParserResetIsCallable(t *testing.T) {
	p, reset := NewPooledParser()
	require.NotNil(t, p)
	require.NotNil(t, reset)
	// Parse once, reset, parse again — both parses must succeed.
	doc1 := p.Parse(text.NewReader([]byte("# first\n")))
	require.NotNil(t, doc1)
	reset()
	doc2 := p.Parse(text.NewReader([]byte("# second\n")))
	require.NotNil(t, doc2)
}

// parseSource invokes NewParser().Parse on the given source and
// returns the resulting document node. Helper shared by parser
// detection tests.
func parseSource(t *testing.T, src []byte) ast.Node {
	t.Helper()
	p := NewParser()
	doc := p.Parse(text.NewReader(src))
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
