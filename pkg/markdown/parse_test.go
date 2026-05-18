package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
)

func TestParse(t *testing.T) {
	t.Run("splits front matter and parses the body", func(t *testing.T) {
		src := []byte("---\ntitle: hi\n---\n# Heading\n\ntext\n")
		doc := Parse(src)
		require.NotNil(t, doc)
		assert.Equal(t, "---\ntitle: hi\n---\n", string(doc.FrontMatter))
		assert.Equal(t, "# Heading\n\ntext\n", string(doc.Body))
		require.NotNil(t, doc.AST)
		assert.Equal(t, ast.KindDocument, doc.AST.Kind())
		h, ok := doc.AST.FirstChild().(*ast.Heading)
		require.True(t, ok, "first child should be a heading")
		assert.Equal(t, 1, h.Level)
	})

	t.Run("no front matter leaves body equal to source", func(t *testing.T) {
		src := []byte("# Only body\n")
		doc := Parse(src)
		assert.Nil(t, doc.FrontMatter)
		assert.Equal(t, "# Only body\n", string(doc.Body))
		assert.Equal(t, ast.KindDocument, doc.AST.Kind())
	})

	t.Run("processing instruction in body is a PI node", func(t *testing.T) {
		src := []byte("---\na: b\n---\n<?include file: x ?>\n<?/include?>\n")
		doc := Parse(src)
		var found *ProcessingInstruction
		_ = ast.Walk(doc.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if entering && found == nil {
				if pi, ok := n.(*ProcessingInstruction); ok {
					found = pi
				}
			}
			return ast.WalkContinue, nil
		})
		require.NotNil(t, found, "<?include?> should parse as a ProcessingInstruction")
		assert.Equal(t, "include", found.Name)
	})

	t.Run("empty and front-matter-only inputs do not panic", func(t *testing.T) {
		assert.NotNil(t, Parse(nil).AST)
		assert.NotNil(t, Parse([]byte("")).AST)
		fmOnly := Parse([]byte("---\nx: 1\n---\n"))
		assert.Equal(t, "---\nx: 1\n---\n", string(fmOnly.FrontMatter))
		assert.Equal(t, "", string(fmOnly.Body))
		assert.NotNil(t, fmOnly.AST)
	})
}

func TestParseContext(t *testing.T) {
	src := []byte("see [ref]\n\n[ref]: https://example.com\n")
	ctx := parser.NewContext()
	root := ParseContext(src, ctx)
	require.NotNil(t, root)
	assert.Equal(t, ast.KindDocument, root.Kind())
	refs := ctx.References()
	require.Len(t, refs, 1)
	assert.Equal(t, "ref", string(refs[0].Label()))
}

func TestSplice(t *testing.T) {
	t.Run("removes ascending non-overlapping spans", func(t *testing.T) {
		body := []byte("0123456789")
		got := Splice(body, []Edit{{1, 3}, {5, 7}})
		assert.Equal(t, "034789", string(got))
	})

	t.Run("no edits returns the input unchanged", func(t *testing.T) {
		body := []byte("unchanged\n")
		assert.Equal(t, "unchanged\n", string(Splice(body, nil)))
	})

	t.Run("does not mutate the source slice", func(t *testing.T) {
		body := []byte("abcdef")
		_ = Splice(body, []Edit{{0, 2}})
		assert.Equal(t, "abcdef", string(body))
	})

	t.Run("spans covering the whole body yield empty output", func(t *testing.T) {
		body := []byte("gone")
		assert.Equal(t, "", string(Splice(body, []Edit{{0, 4}})))
	})
}
