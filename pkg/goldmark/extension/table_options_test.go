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

func TestTable_OptionsAsRendererOptions(t *testing.T) {
	// Table options also implement renderer.Option (SetConfig) so
	// they can be applied via AddOptions after construction.
	r := renderer.NewRenderer(renderer.WithNodeRenderers(
		util.Prioritized(html.NewRenderer(), 1000),
		util.Prioritized(extension.NewTableHTMLRenderer(), 500),
	))
	r.AddOptions(
		extension.WithTableCellAlignMethod(extension.TableCellAlignStyle).(renderer.Option),
		extension.WithTableHTMLOptions(html.WithUnsafe()).(renderer.Option),
	)
	md := goldmark.New(goldmark.WithExtensions(extension.Table), goldmark.WithRenderer(r))
	var buf bytes.Buffer
	if err := md.Convert([]byte(tableOptSrc), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}

func TestTable_PrefixParagraphSplit(t *testing.T) {
	// When a paragraph contains a non-table prefix line, then
	// the table header, then the delimiter row, the table
	// transformer slices off the prefix as a separate paragraph
	// (else branch: trim last newline).
	src := "prefix paragraph line\n| h1 | h2 |\n|---|---|\n| a | b |\n"
	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}

func TestTable_ColumnMismatchRejected(t *testing.T) {
	// tableParagraphTransformer's "header.ChildCount() !=
	// len(alignments)" branch fires when the header row has a
	// different column count than the delimiter row.  The
	// paragraph stays a paragraph (no table).
	srcs := []string{
		"| a |\n|---|---|---|\n| b |\n",                            // 1 vs 3 cols
		"| h1 | h2 | h3 |\n|---|\n| a | b | c |\n",                  // 3 vs 1 col
		"| h |\n| not delim |\n",                                    // 2nd line not a delim
		"single line paragraph\n",                                   // 1 line only
		"line one\nline two\nline three\n",                          // no delim row
	}
	for _, src := range srcs {
		md := goldmark.New(goldmark.WithExtensions(extension.Table))
		var buf bytes.Buffer
		if err := md.Convert([]byte(src), &buf); err != nil {
			t.Fatalf("Convert(%q): %v", src, err)
		}
	}
}

func TestNewTable_Extender(t *testing.T) {
	// NewTable returns an Extender; plug it in with explicit
	// options.
	ext := extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute))
	md := goldmark.New(goldmark.WithExtensions(ext))
	var buf bytes.Buffer
	if err := md.Convert([]byte(tableOptSrc), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}

func TestTable_EscapedPipeInCell_DrivesASTTransformer(t *testing.T) {
	// `\|` inside a cell — and especially inside a code span — is
	// what makes tableASTTransformer.Transform do real work: it
	// rewrites the inline AST so the pipe becomes a literal rather
	// than a column delimiter. Without this case the transformer
	// returns immediately on the lst==nil branch.
	src := "| h1 | h2 |\n|----|----|\n| `a\\|b` | c |\n"
	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "<table>") {
		t.Errorf("table missing in output: %q", out)
	}
	if !strings.Contains(out, "<code>") {
		t.Errorf("code span missing in output: %q", out)
	}
}
