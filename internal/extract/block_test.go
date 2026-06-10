package extract

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/jeduden/mdsmith/pkg/markdown/flavor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// topNodes returns body's top-level block children in document order,
// parsed table-aware so GFM tables surface as *extast.Table (the same
// parse the section blocks-projection feeds the walker). It is the
// unit-level driver for the block grammar.
func topNodes(t *testing.T, body string) (*projector, []ast.Node) {
	t.Helper()
	f := doc(t, body)
	parser, reset := flavor.NewPooledParserWith(extension.Table)
	defer reset()
	root := parser.Parse(text.NewReader(f.Source))
	p := &projector{f: f}
	var nodes []ast.Node
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		nodes = append(nodes, c)
	}
	return p, nodes
}

// walkBody parses body, walks its top-level blocks through the block
// grammar, and returns the typed list plus the projector (for diags).
func walkBody(t *testing.T, body string) ([]any, *projector) {
	t.Helper()
	p, nodes := topNodes(t, body)
	return p.blocksFromNodes(nodes, false), p
}

func TestBlockWalker_Paragraph(t *testing.T) {
	got, p := walkBody(t, "First paragraph.\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"block": "paragraph",
		"text":  "First paragraph.",
	}, got[0])
}

func TestBlockWalker_FencedCodeWithLang(t *testing.T) {
	got, p := walkBody(t, "```go\nfunc F() {}\n```\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"block": "code",
		"lang":  "go",
		"value": "func F() {}\n",
	}, got[0])
}

// A fence with no info string omits `lang` entirely.
func TestBlockWalker_FencedCodeNoLang(t *testing.T) {
	got, p := walkBody(t, "```\nplain\n```\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"block": "code",
		"value": "plain\n",
	}, got[0])
}

// An indented code block also projects as a `code` block, never with
// a `lang` (indented fences carry no info string).
func TestBlockWalker_IndentedCode(t *testing.T) {
	got, p := walkBody(t, "    indented\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"block": "code",
		"value": "indented\n",
	}, got[0])
}

func TestBlockWalker_List(t *testing.T) {
	got, p := walkBody(t, "- one item\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"block": "list",
		"items": []any{map[string]any{"text": "one item"}},
	}, got[0])
}

func TestBlockWalker_Table(t *testing.T) {
	got, p := walkBody(t, "| A |\n| - |\n| 1 |\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"block":   "table",
		"columns": []any{"A"},
		"rows":    []any{[]any{"1"}},
	}, got[0])
}

// A header-only table (valid GFM: header + delimiter, no body rows)
// projects `rows` as an empty, non-nil slice so it serialises to
// `"rows": []` — the published CUE `#Block` contract rejects the
// `"rows": null` a nil slice would produce.
func TestBlockWalker_TableHeaderOnly(t *testing.T) {
	got, p := walkBody(t, "| A | B |\n| - | - |\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"block":   "table",
		"columns": []any{"A", "B"},
		"rows":    []any{},
	}, got[0])
}

func TestBlockWalker_Quote(t *testing.T) {
	got, p := walkBody(t, "> A quoted line.\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"block": "quote",
		"blocks": []any{
			map[string]any{"block": "paragraph", "text": "A quoted line."},
		},
	}, got[0])
}

func TestBlockWalker_ThematicBreak(t *testing.T) {
	got, p := walkBody(t, "---\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{"block": "break"}, got[0])
}

func TestBlockWalker_HTMLBlock(t *testing.T) {
	got, p := walkBody(t, "<div>raw</div>\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"block": "html",
		"value": "<div>raw</div>",
	}, got[0])
}

// A multi-line HTML block with an explicit closing line (a comment
// block, goldmark HTML-block type 2) carries its closure line into the
// projected `value`, exercising htmlBlockValue's HasClosure() branch.
func TestBlockWalker_HTMLBlockWithClosure(t *testing.T) {
	got, p := walkBody(t, "<!-- open\nmiddle\nclose -->\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"block": "html",
		"value": "<!-- open\nmiddle\nclose -->",
	}, got[0])
}

// A deeper heading inside the slice opens a nested `section` block;
// the body beneath it recurses through the same grammar.
func TestBlockWalker_SectionFromHeading(t *testing.T) {
	got, p := walkBody(t, "### Sub\n\nbody text\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"block":   "section",
		"level":   3,
		"heading": "Sub",
		"blocks": []any{
			map[string]any{"block": "paragraph", "text": "body text"},
		},
	}, got[0])
}

// A heading of the same or shallower level closes the previous
// section's body — two sibling sections, not a nested one.
func TestBlockWalker_SiblingSectionsCloseAtSameLevel(t *testing.T) {
	got, p := walkBody(t, "### One\n\na\n\n### Two\n\nb\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 2)
	assert.Equal(t, 3, got[0].(map[string]any)["level"])
	assert.Equal(t, "One", got[0].(map[string]any)["heading"])
	assert.Equal(t, "Two", got[1].(map[string]any)["heading"])
}

// A deeper heading nests under its parent section recursively.
func TestBlockWalker_NestedSection(t *testing.T) {
	got, p := walkBody(t, "### Parent\n\np\n\n#### Child\n\nc\n")
	require.Empty(t, p.diags)
	require.Len(t, got, 1)
	parent := got[0].(map[string]any)
	inner := parent["blocks"].([]any)
	// parent's paragraph, then the nested child section
	require.Len(t, inner, 2)
	assert.Equal(t, "paragraph", inner[0].(map[string]any)["block"])
	child := inner[1].(map[string]any)
	assert.Equal(t, "section", child["block"])
	assert.Equal(t, 4, child["level"])
	assert.Equal(t, "Child", child["heading"])
}

// Document order is preserved across mixed block kinds.
func TestBlockWalker_DocumentOrder(t *testing.T) {
	body := "First paragraph.\n\n```go\nfunc F() {}\n```\n\n> A quoted line.\n\n" +
		"- one item\n\n| A |\n| - |\n| 1 |\n"
	got, p := walkBody(t, body)
	require.Empty(t, p.diags)
	require.Len(t, got, 5)
	kinds := make([]string, len(got))
	for i, b := range got {
		kinds[i] = b.(map[string]any)["block"].(string)
	}
	assert.Equal(t, []string{"paragraph", "code", "quote", "list", "table"}, kinds)
}

// headingAtOrAbove is a small predicate; pin both arms.
func TestHeadingAtOrAbove(t *testing.T) {
	h2 := ast.NewHeading(2)
	assert.True(t, headingAtOrAbove(h2, 2))
	assert.True(t, headingAtOrAbove(h2, 3))
	assert.False(t, headingAtOrAbove(h2, 1))
	assert.False(t, headingAtOrAbove(ast.NewParagraph(), 1))
}

// An unsupported block node records a hard diagnostic and is dropped
// from the output (extract treats any diagnostic as a hard failure).
// No Markdown source produces a node outside the grammar, so feed the
// walker a synthetic document node to drive the default arm.
func TestBlockWalker_UnsupportedBlock(t *testing.T) {
	p := &projector{f: doc(t, "x\n"), sch: &schema.Schema{}}
	got := p.blocksFromNodes([]ast.Node{ast.NewDocument()}, false)
	assert.Empty(t, got)
	require.NotEmpty(t, p.diags)
	assert.Contains(t, p.diags[0].Message, "unsupported block")
}
