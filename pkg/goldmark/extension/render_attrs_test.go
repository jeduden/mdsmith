package extension_test

// Coverage for the Attributes() != nil branch in each extension
// renderer (Strikethrough, DefinitionList, DefinitionTerm,
// DefinitionDescription, Table). Build AST nodes manually with
// SetAttribute and render them directly so the parse flow does
// not need to emit the rare attribute-bearing form.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
	gext "github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func newExtRenderer() renderer.Renderer {
	return renderer.NewRenderer(renderer.WithNodeRenderers(
		util.Prioritized(html.NewRenderer(), 1000),
		util.Prioritized(gext.NewStrikethroughHTMLRenderer(), 500),
		util.Prioritized(gext.NewDefinitionListHTMLRenderer(), 500),
		util.Prioritized(gext.NewTableHTMLRenderer(), 500),
	))
}

func TestNew_ExtensionHTMLRenderersWithOptions(t *testing.T) {
	// NewDefinitionListHTMLRenderer, NewStrikethroughHTMLRenderer,
	// and NewTaskCheckBoxHTMLRenderer each accept html.Option
	// variadic args.  The loop body is unreached when no options
	// are passed.  Drive each with an option.
	_ = gext.NewDefinitionListHTMLRenderer(html.WithUnsafe())
	_ = gext.NewStrikethroughHTMLRenderer(html.WithUnsafe())
	_ = gext.NewTaskCheckBoxHTMLRenderer(html.WithUnsafe())
	_ = gext.NewFootnoteHTMLRenderer(
		gext.WithFootnoteHTMLOptions(html.WithUnsafe()),
	)
}

func TestExtensionAST_DefinitionList_Pos(t *testing.T) {
	// DefinitionList.Pos returns child's Pos when populated.
	list := extast.NewDefinitionList(0, ast.NewParagraph())
	term := extast.NewDefinitionTerm()
	list.AppendChild(list, term)
	_ = list.Pos() // populated branch

	emptyList := extast.NewDefinitionList(0, ast.NewParagraph())
	_ = emptyList.Pos() // empty branch

	// DefinitionTerm.Pos: empty + populated.
	emptyTerm := extast.NewDefinitionTerm()
	_ = emptyTerm.Pos()
	populatedTerm := extast.NewDefinitionTerm()
	populatedTerm.Lines().Append(text.NewSegment(5, 10))
	if got := populatedTerm.Pos(); got != 5 {
		t.Errorf("DefinitionTerm.Pos populated = %d, want 5", got)
	}

	// DefinitionDescription.Pos: empty + populated.
	emptyDesc := extast.NewDefinitionDescription()
	_ = emptyDesc.Pos()
	populatedDesc := extast.NewDefinitionDescription()
	populatedDesc.Lines().Append(text.NewSegment(3, 8))
	_ = populatedDesc.Pos()
}

func TestRender_StrikethroughWithAttributes(t *testing.T) {
	doc := ast.NewDocument()
	p := ast.NewParagraph()
	doc.AppendChild(doc, p)
	s := extast.NewStrikethrough()
	s.SetAttribute([]byte("class"), []byte("strike"))
	p.AppendChild(p, s)

	var buf bytes.Buffer
	if err := newExtRenderer().Render(&buf, []byte("source"), doc); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), `class="strike"`) {
		t.Errorf("strikethrough attribute not rendered: %q", buf.String())
	}
}

func TestRender_DefinitionListWithAttributes(t *testing.T) {
	doc := ast.NewDocument()
	dl := extast.NewDefinitionList(0, ast.NewParagraph())
	dl.SetAttribute([]byte("class"), []byte("dl"))
	doc.AppendChild(doc, dl)

	dt := extast.NewDefinitionTerm()
	dt.SetAttribute([]byte("class"), []byte("dt"))
	dl.AppendChild(dl, dt)

	dd := extast.NewDefinitionDescription()
	dd.SetAttribute([]byte("class"), []byte("dd"))
	dl.AppendChild(dl, dd)

	var buf bytes.Buffer
	if err := newExtRenderer().Render(&buf, []byte("source"), doc); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`class="dl"`, `class="dt"`, `class="dd"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output: %q", want, out)
		}
	}
}

func TestRender_FootnoteListWithAttributes(t *testing.T) {
	// renderFootnoteList has Attributes() != nil branch.  Build
	// AST and inject attributes manually.
	doc := ast.NewDocument()
	list := extast.NewFootnoteList()
	list.SetAttribute([]byte("class"), []byte("fn-list"))
	doc.AppendChild(doc, list)
	fn := extast.NewFootnote([]byte("a"))
	fn.SetAttribute([]byte("class"), []byte("fn-item"))
	list.AppendChild(list, fn)

	r := renderer.NewRenderer(renderer.WithNodeRenderers(
		util.Prioritized(html.NewRenderer(), 1000),
		util.Prioritized(gext.NewFootnoteHTMLRenderer(), 500),
	))
	var buf bytes.Buffer
	if err := r.Render(&buf, []byte("source"), doc); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), `class="fn-list"`) {
		t.Errorf("FootnoteList attribute not rendered: %q", buf.String())
	}
}

func TestRender_TableCellWithAlignOverrides(t *testing.T) {
	// Drive renderTableCell's align/style attribute-override
	// branches by constructing cells with explicit align/style
	// attributes that override the cell's Alignment field.
	doc := ast.NewDocument()
	tbl := extast.NewTable()
	tbl.Alignments = []extast.Alignment{extast.AlignLeft, extast.AlignRight}
	doc.AppendChild(doc, tbl)

	row := extast.NewTableRow(tbl.Alignments)
	tbl.AppendChild(tbl, row)

	cellA := extast.NewTableCell()
	cellA.Alignment = extast.AlignLeft
	cellA.SetAttribute([]byte("align"), []byte("center")) // overrides Alignment
	row.AppendChild(row, cellA)

	cellB := extast.NewTableCell()
	cellB.Alignment = extast.AlignRight
	cellB.SetAttribute([]byte("style"), []byte("color: red")) // existing style; renderer appends text-align
	row.AppendChild(row, cellB)

	r := newExtRenderer()
	var buf bytes.Buffer
	if err := r.Render(&buf, []byte("source"), doc); err != nil {
		t.Fatalf("Render: %v", err)
	}
}

func TestRender_TableWithAttributes(t *testing.T) {
	doc := ast.NewDocument()
	tbl := extast.NewTable()
	tbl.SetAttribute([]byte("class"), []byte("tbl"))
	doc.AppendChild(doc, tbl)

	row := extast.NewTableRow(nil)
	row.SetAttribute([]byte("class"), []byte("row"))
	tbl.AppendChild(tbl, row)

	cell := extast.NewTableCell()
	cell.SetAttribute([]byte("class"), []byte("cell"))
	row.AppendChild(row, cell)

	var buf bytes.Buffer
	if err := newExtRenderer().Render(&buf, []byte("source"), doc); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`class="tbl"`, `class="row"`, `class="cell"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output: %q", want, out)
		}
	}
}
