package html_test

// Coverage for html.With* options' SetHTMLOption dispatchers
// (one per option) plus the option-only renderer construction
// path. Upstream commonmark_test.go exercises rendering with
// default options; the html.With* paths only fire when callers
// pass them to html.NewRenderer(), which the spec test does not.

import (
	"bytes"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

func TestHTMLOptions_ApplyViaNewRenderer(t *testing.T) {
	// Each option's SetHTMLOption is dispatched when the option is
	// passed to html.NewRenderer. Round-tripping a tiny document
	// through goldmark.New(renderer = html.NewRenderer(opt)) is
	// the path that drives every dispatcher.
	render := func(t *testing.T, nr renderer.NodeRenderer) {
		t.Helper()
		r := renderer.NewRenderer(
			renderer.WithNodeRenderers(util.Prioritized(nr, 1000)),
		)
		md := goldmark.New(goldmark.WithRenderer(r))
		var buf bytes.Buffer
		if err := md.Convert([]byte("Hello *world*\n"), &buf); err != nil {
			t.Fatalf("Convert: %v", err)
		}
		if buf.Len() == 0 {
			t.Error("rendered output is empty")
		}
	}
	t.Run("HardWraps", func(t *testing.T) { render(t, html.NewRenderer(html.WithHardWraps())) })
	t.Run("XHTML", func(t *testing.T) { render(t, html.NewRenderer(html.WithXHTML())) })
	t.Run("Unsafe", func(t *testing.T) { render(t, html.NewRenderer(html.WithUnsafe())) })
	t.Run("EastAsianLineBreaks-Simple", func(t *testing.T) {
		render(t, html.NewRenderer(html.WithEastAsianLineBreaks(html.EastAsianLineBreaksSimple)))
	})
	t.Run("EastAsianLineBreaks-CSS3Draft", func(t *testing.T) {
		render(t, html.NewRenderer(html.WithEastAsianLineBreaks(html.EastAsianLineBreaksCSS3Draft)))
	})
}

func TestHTMLOptions_WithWriter(t *testing.T) {
	w := html.NewWriter()
	r := renderer.NewRenderer(
		renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(html.WithWriter(w)), 1000)),
	)
	md := goldmark.New(goldmark.WithRenderer(r))
	var buf bytes.Buffer
	if err := md.Convert([]byte("paragraph\n"), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}

// IsDangerousURL is used by the renderer to decide whether to
// strip javascript:, vbscript:, file:, data:image/svg+xml URLs.
// Drive each prefix to lift its coverage from 33 % to 100 %.
func TestIsDangerousURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		// Plain dangerous schemes.
		{"javascript:alert(1)", true},
		{"vbscript:msgbox()", true},
		{"file:///etc/passwd", true},
		{"data:text/html;base64,xxx", true}, // non-image data: is dangerous
		// data:image/* with a recognised image format is safe.
		{"data:image/png;base64,xxx", false},
		{"data:image/gif;base64,xxx", false},
		{"data:image/jpeg;base64,xxx", false},
		{"data:image/webp;base64,xxx", false},
		{"data:image/svg+xml;base64,xxx", false},
		// data:image/* with an unrecognised format trips the
		// trailing `return true` inside the bDataImage branch.
		{"data:image/exe;base64,xxx", true},
		// Plain safe URLs.
		{"https://example.com", false},
		{"./relative", false},
	}
	for _, tc := range cases {
		if got := html.IsDangerousURL([]byte(tc.url)); got != tc.want {
			t.Errorf("IsDangerousURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}
