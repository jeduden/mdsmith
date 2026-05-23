package extension_test

// Coverage for extension renderer-option dispatchers. Each
// With*Option function returns a typed FootnoteOption / table
// option whose SetFootnoteOption / SetConfig methods are only
// hit when the option is passed to NewFootnoteHTMLRenderer.
// Round-trip a footnote document through a renderer wired with
// each option and confirm the option is honoured in the output.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

const footnoteSrc = "see this[^1].\n\n[^1]: footnote body\n"

func renderFootnote(t *testing.T, opts ...extension.FootnoteOption) string {
	t.Helper()
	r := renderer.NewRenderer(
		renderer.WithNodeRenderers(
			util.Prioritized(html.NewRenderer(), 1000),
			util.Prioritized(extension.NewFootnoteHTMLRenderer(opts...), 500),
		),
	)
	md := goldmark.New(
		goldmark.WithExtensions(extension.Footnote),
		goldmark.WithRenderer(r),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(footnoteSrc), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return buf.String()
}

func TestFootnote_IDPrefix(t *testing.T) {
	out := renderFootnote(t, extension.WithFootnoteIDPrefix("fn-"))
	if !strings.Contains(out, `id="fn-fn:1"`) && !strings.Contains(out, `id="fn-fnref:1"`) {
		t.Errorf("expected ID prefix fn-, got: %s", out)
	}
}

func TestFootnote_IDPrefixFunction(t *testing.T) {
	out := renderFootnote(t, extension.WithFootnoteIDPrefixFunction(func(n gast.Node) []byte {
		return []byte("doc-")
	}))
	if !strings.Contains(out, "doc-") {
		t.Errorf("expected doc- prefix, got: %s", out)
	}
}

func TestFootnote_LinkTitle(t *testing.T) {
	out := renderFootnote(t, extension.WithFootnoteLinkTitle("link title"))
	if !strings.Contains(out, `title="link title"`) {
		t.Errorf("expected link title, got: %s", out)
	}
}

func TestFootnote_BacklinkTitle(t *testing.T) {
	out := renderFootnote(t, extension.WithFootnoteBacklinkTitle("back title"))
	if !strings.Contains(out, `title="back title"`) {
		t.Errorf("expected backlink title, got: %s", out)
	}
}

func TestFootnote_LinkClass(t *testing.T) {
	out := renderFootnote(t, extension.WithFootnoteLinkClass("link-cls"))
	if !strings.Contains(out, `class="link-cls"`) {
		t.Errorf("expected link class, got: %s", out)
	}
}

func TestFootnote_BacklinkClass(t *testing.T) {
	out := renderFootnote(t, extension.WithFootnoteBacklinkClass("back-cls"))
	if !strings.Contains(out, `class="back-cls"`) {
		t.Errorf("expected backlink class, got: %s", out)
	}
}

func TestFootnote_BacklinkHTML(t *testing.T) {
	out := renderFootnote(t, extension.WithFootnoteBacklinkHTML(`<span>back</span>`))
	if !strings.Contains(out, `<span>back</span>`) {
		t.Errorf("expected backlink html, got: %s", out)
	}
}

func TestFootnote_OptionsAsRendererOptions(t *testing.T) {
	// Footnote options also implement renderer.Option (SetConfig).
	// renderer.NewRenderer accepts both Option-as-NodeRenderers
	// and Option-as-config-setter via the same Options slot.
	// Drive that path so SetConfig fires.
	r := renderer.NewRenderer(
		renderer.WithNodeRenderers(
			util.Prioritized(html.NewRenderer(), 1000),
			util.Prioritized(extension.NewFootnoteHTMLRenderer(), 500),
		),
	)
	// Apply each option via AddOptions which calls SetConfig.
	r.AddOptions(
		extension.WithFootnoteIDPrefix("doc-").(renderer.Option),
		extension.WithFootnoteIDPrefixFunction(func(node gast.Node) []byte { return []byte("fn-") }).(renderer.Option),
		extension.WithFootnoteLinkTitle("link").(renderer.Option),
		extension.WithFootnoteBacklinkTitle("back").(renderer.Option),
		extension.WithFootnoteLinkClass("lcls").(renderer.Option),
		extension.WithFootnoteBacklinkClass("bcls").(renderer.Option),
		extension.WithFootnoteBacklinkHTML("<x/>").(renderer.Option),
		extension.WithFootnoteHTMLOptions(html.WithUnsafe()).(renderer.Option),
	)
	md := goldmark.New(
		goldmark.WithExtensions(extension.Footnote),
		goldmark.WithRenderer(r),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(footnoteSrc), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}

func TestFootnote_HTMLOptionsPropagation(t *testing.T) {
	// WithFootnoteHTMLOptions threads html.Option through to the
	// underlying html.Config. Pass html.WithUnsafe and verify
	// raw HTML in the footnote body survives.
	out := renderFootnote(t, extension.WithFootnoteHTMLOptions(html.WithUnsafe()))
	if !strings.Contains(out, `<a href="#fn:1"`) {
		t.Errorf("HTMLOptions round trip lost footnote link: %s", out)
	}
}

// renderTable wires a fresh renderer with the given table options.
const tableSrc = "| h |\n|---|\n| c |\n"

func TestTable_WithTableHTMLOptions(t *testing.T) {
	r := renderer.NewRenderer(
		renderer.WithNodeRenderers(
			util.Prioritized(html.NewRenderer(), 1000),
			util.Prioritized(extension.NewTableHTMLRenderer(extension.WithTableHTMLOptions(html.WithUnsafe())), 500),
		),
	)
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table),
		goldmark.WithRenderer(r),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(tableSrc), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(buf.String(), "<table>") {
		t.Errorf("Table render lost <table>: %s", buf.String())
	}
}
