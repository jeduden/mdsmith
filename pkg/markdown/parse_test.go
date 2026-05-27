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
		got := Splice(body, []Edit{{Start: 1, End: 3}, {Start: 5, End: 7}})
		assert.Equal(t, "034789", string(got))
	})

	t.Run("no edits returns the input unchanged", func(t *testing.T) {
		body := []byte("unchanged\n")
		assert.Equal(t, "unchanged\n", string(Splice(body, nil)))
	})

	t.Run("does not mutate the source slice", func(t *testing.T) {
		body := []byte("abcdef")
		_ = Splice(body, []Edit{{Start: 0, End: 2}})
		assert.Equal(t, "abcdef", string(body))
	})

	t.Run("spans covering the whole body yield empty output", func(t *testing.T) {
		body := []byte("gone")
		assert.Equal(t, "", string(Splice(body, []Edit{{Start: 0, End: 4}})))
	})

	t.Run("Repl bytes are spliced in at the edit position", func(t *testing.T) {
		// Wraps "url" in angle brackets at offset 4..7 of the body —
		// the rule-side bare-URL fix shape — and verifies that the
		// surrounding text is preserved untouched.
		body := []byte("see url here")
		edits := []Edit{{Start: 4, End: 7, Repl: []byte("<url>")}}
		assert.Equal(t, "see <url> here", string(Splice(body, edits)))
	})

	t.Run("Repl handles adjacent edits in one pass", func(t *testing.T) {
		// Two zero-byte deletions adjacent to a pure insertion at the
		// start guard the cursor advancement: prev = e.End, and after
		// appending Repl the loop must continue from the next edit's
		// Start without dropping or duplicating bytes between them.
		body := []byte("ab~~xy~~cd")
		edits := []Edit{
			{Start: 0, End: 0, Repl: []byte("> ")}, // pure insertion
			{Start: 2, End: 4},                     // opening "~~"
			{Start: 6, End: 8},                     // closing "~~"
		}
		assert.Equal(t, "> abxycd", string(Splice(body, edits)))
	})

}

// TestSpliceInvariantViolation pins the contract Splice's docstring
// promises: edits must be ascending, non-overlapping, well-formed
// (End >= Start), and within body bounds. Every violation surfaces
// as a descriptive panic at the entry point rather than as an
// opaque slice-bounds-out-of-range crash during the build loop.
func TestSpliceInvariantViolation(t *testing.T) {
	t.Run("overlapping edits panic with a descriptive message", func(t *testing.T) {
		body := []byte("0123456789")
		assert.PanicsWithValue(t,
			"markdown.Splice: edits must be ascending and "+
				"non-overlapping; edit 1 {Start:6, End:12} "+
				"overlaps previous edit ending at 10",
			func() {
				Splice(body, []Edit{
					{Start: 5, End: 10},
					{Start: 6, End: 12},
				})
			})
	})

	t.Run("descending edits panic with the same overlap message", func(t *testing.T) {
		body := []byte("0123456789")
		assert.PanicsWithValue(t,
			"markdown.Splice: edits must be ascending and "+
				"non-overlapping; edit 1 {Start:0, End:2} "+
				"overlaps previous edit ending at 5",
			func() {
				Splice(body, []Edit{
					{Start: 3, End: 5},
					{Start: 0, End: 2},
				})
			})
	})

	t.Run("End < Start panics", func(t *testing.T) {
		body := []byte("0123456789")
		assert.PanicsWithValue(t,
			"markdown.Splice: edit 0 has End<Start ({Start:5, End:3})",
			func() { Splice(body, []Edit{{Start: 5, End: 3}}) })
	})

	t.Run("edit exceeding body length panics", func(t *testing.T) {
		body := []byte("short")
		assert.PanicsWithValue(t,
			"markdown.Splice: edit 0 {Start:0, End:99} "+
				"exceeds body length 5",
			func() { Splice(body, []Edit{{Start: 0, End: 99}}) })
	})
}
