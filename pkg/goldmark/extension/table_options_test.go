package extension_test

// Coverage for the extension Table option dispatchers — each
// With*Option type's SetTableOption and SetConfig methods, plus
// the three TableCellAlignMethod variants and the
// NewTableASTTransformer constructor reachable only through the
// Extender path that's already wired by extension.Table.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

const tableOptSrc = "| h1 | h2 | h3 |\n|:---|:--:|---:|\n| a  | b  | c  |\n"

func renderTableWith(t *testing.T, opts ...extension.TableOption) string {
	t.Helper()
	r := renderer.NewRenderer(
		renderer.WithNodeRenderers(
			util.Prioritized(html.NewRenderer(), 1000),
			util.Prioritized(extension.NewTableHTMLRenderer(opts...), 500),
		),
	)
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table),
		goldmark.WithRenderer(r),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(tableOptSrc), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return buf.String()
}

func TestTable_WithCellAlignMethod_Default(t *testing.T) {
	out := renderTableWith(t, extension.WithTableCellAlignMethod(extension.TableCellAlignDefault))
	// Default emits style="text-align:..." per cell.
	if !strings.Contains(out, "text-align:") {
		t.Errorf("default cell-align should emit style=text-align: in output: %q", out)
	}
}

func TestTable_WithCellAlignMethod_Attribute(t *testing.T) {
	out := renderTableWith(t, extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute))
	if !strings.Contains(out, "align=") {
		t.Errorf("Attribute method should emit align= in output: %q", out)
	}
}

func TestTable_WithCellAlignMethod_Style(t *testing.T) {
	out := renderTableWith(t, extension.WithTableCellAlignMethod(extension.TableCellAlignStyle))
	if !strings.Contains(out, "style=") {
		t.Errorf("Style method should emit style= in output: %q", out)
	}
}

func TestTable_DefaultExtenderPath(t *testing.T) {
	// Verify the extension.Table Extender path: NewTableConfig,
	// NewTableParser, NewTableASTTransformer all run as part of
	// Extender wiring.
	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	var buf bytes.Buffer
	if err := md.Convert([]byte(tableOptSrc), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(buf.String(), "<table>") {
		t.Errorf("default Extender path produced no <table>: %q", buf.String())
	}
}
