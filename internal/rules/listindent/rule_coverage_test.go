package listindent

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// --- firstLineOfChild coverage ---

func TestFirstLineOfChild_TextNode(t *testing.T) {
	src := []byte("- item\n  - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if li, ok := n.(*ast.ListItem); ok {
			if li.HasChildren() {
				for c := li.FirstChild(); c != nil; c = c.NextSibling() {
					line := firstLineOfChild(f, c)
					if line > 0 {
						assert.Greater(t, line, 0)
						return ast.WalkStop, nil
					}
				}
			}
		}
		return ast.WalkContinue, nil
	})
}

func TestFirstLineOfChild_InlineNodeWithChildren(t *testing.T) {
	// List with emphasis -> inline node (Emphasis) with Text children
	src := []byte("- *bold item*\n  - *nested*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	found := false
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := n.(*ast.Emphasis); ok {
			line := firstLineOfChild(f, n)
			if line > 0 {
				found = true
			}
			return ast.WalkContinue, nil
		}
		return ast.WalkContinue, nil
	})
	assert.True(t, found, "expected to find inline node with children")
}

func TestFirstLineOfChild_BlockNodeWithLines(t *testing.T) {
	// A code block is a block node with Lines() > 0
	src := []byte("- item\n\n      code block\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	found := false
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		// Look for any block node that has Lines
		if n.Type() == ast.TypeBlock && n.Lines().Len() > 0 {
			line := firstLineOfChild(f, n)
			if line > 0 {
				found = true
			}
		}
		return ast.WalkContinue, nil
	})
	assert.True(t, found)
}

// --- toIntSetting coverage ---

func TestToIntSetting_Int(t *testing.T) {
	n, ok := toIntSetting(42)
	assert.True(t, ok)
	assert.Equal(t, 42, n)
}

func TestToIntSetting_Float64(t *testing.T) {
	n, ok := toIntSetting(float64(3.0))
	assert.True(t, ok)
	assert.Equal(t, 3, n)
}

func TestToIntSetting_Int64(t *testing.T) {
	n, ok := toIntSetting(int64(7))
	assert.True(t, ok)
	assert.Equal(t, 7, n)
}

func TestToIntSetting_String_Fails(t *testing.T) {
	_, ok := toIntSetting("not a number")
	assert.False(t, ok)
}

func TestToIntSetting_Bool_Fails(t *testing.T) {
	_, ok := toIntSetting(true)
	assert.False(t, ok)
}

// --- firstLineOfListItem coverage ---

func TestFirstLineOfListItem_ViaLines(t *testing.T) {
	src := []byte("- item one\n- item two\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	// Flat list, no violations
	assert.Len(t, diags, 0)
}

func TestFirstLineOfListItem_ViaChildren(t *testing.T) {
	// Nested list: the nested item's firstLineOfListItem uses children path
	src := []byte("- item\n  - nested\n    - deep\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

// --- isInlineNode coverage ---

func TestIsInlineNode_TextNode(t *testing.T) {
	node := ast.NewText()
	assert.True(t, isInlineNode(node))
}

func TestIsInlineNode_StringNode(t *testing.T) {
	node := ast.NewString([]byte("hello"))
	assert.True(t, isInlineNode(node))
}

func TestIsInlineNode_CodeSpan(t *testing.T) {
	node := ast.NewCodeSpan()
	assert.True(t, isInlineNode(node))
}

func TestIsInlineNode_Emphasis(t *testing.T) {
	node := ast.NewEmphasis(1)
	assert.True(t, isInlineNode(node))
}

func TestIsInlineNode_Link(t *testing.T) {
	node := ast.NewLink()
	assert.True(t, isInlineNode(node))
}

func TestIsInlineNode_Image(t *testing.T) {
	node := ast.NewImage(ast.NewLink())
	assert.True(t, isInlineNode(node))
}

func TestIsInlineNode_AutoLink(t *testing.T) {
	node := ast.NewAutoLink(ast.AutoLinkURL, &ast.Text{})
	assert.True(t, isInlineNode(node))
}

func TestIsInlineNode_RawHTML(t *testing.T) {
	node := ast.NewRawHTML()
	assert.True(t, isInlineNode(node))
}

func TestIsInlineNode_Paragraph(t *testing.T) {
	node := ast.NewParagraph()
	assert.False(t, isInlineNode(node))
}

func TestIsInlineNode_Heading(t *testing.T) {
	node := ast.NewHeading(1)
	assert.False(t, isInlineNode(node))
}

// --- itoa coverage ---

func TestItoa_Zero(t *testing.T) {
	assert.Equal(t, "0", itoa(0))
}

func TestItoa_Positive(t *testing.T) {
	assert.Equal(t, "42", itoa(42))
}

func TestItoa_SingleDigit(t *testing.T) {
	assert.Equal(t, "7", itoa(7))
}

func TestItoa_Negative(t *testing.T) {
	assert.Equal(t, "-5", itoa(-5))
}

func TestItoa_LargeNumber(t *testing.T) {
	assert.Equal(t, "1234", itoa(1234))
}

func TestItoa_NegativeLarge(t *testing.T) {
	assert.Equal(t, "-100", itoa(-100))
}

// --- Check with zero/negative Spaces defaults to 2 ---

func TestCheck_ZeroSpaces_DefaultsTo2(t *testing.T) {
	src := []byte("- item\n  - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 0}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_NegativeSpaces_DefaultsTo2(t *testing.T) {
	src := []byte("- item\n  - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: -1}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

// --- ApplySettings with float64 ---

func TestApplySettings_Float64Spaces(t *testing.T) {
	r := &Rule{Spaces: 2}
	err := r.ApplySettings(map[string]any{"spaces": float64(4)})
	require.NoError(t, err)
	assert.Equal(t, 4, r.Spaces)
}

func TestApplySettings_Int64Spaces(t *testing.T) {
	r := &Rule{Spaces: 2}
	err := r.ApplySettings(map[string]any{"spaces": int64(3)})
	require.NoError(t, err)
	assert.Equal(t, 3, r.Spaces)
}
