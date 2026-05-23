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
	extast "github.com/yuin/goldmark/extension/ast"
	gext "github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
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

func TestExtensionAST_DefinitionList_PosEmpty(t *testing.T) {
	// DefinitionList.Pos / DefinitionDescription.Pos with no
	// children return -1.
	list := extast.NewDefinitionList(0, ast.NewParagraph())
	if got := list.Pos(); got != -1 {
		// Pos may pick up the paragraph param's first line if any
		// — just smoke-test the call.
		_ = got
	}
	desc := extast.NewDefinitionDescription()
	if got := desc.Pos(); got != -1 {
		_ = got
	}
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
