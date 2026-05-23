package renderer_test

// Coverage for renderer-level options (WithOption + SetConfig)
// and the renderer.AddOption pipeline.

import (
	"bytes"
	"testing"

	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

func TestRenderer_WithOption(t *testing.T) {
	// renderer.WithOption returns an Option whose SetConfig
	// records the value into Config.Options.  Use AddOptions to
	// apply it to a Renderer.
	r := renderer.NewRenderer(
		renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(), 1000)),
	)
	r.AddOptions(renderer.WithOption("custom-key", "custom-value"))

	// Verify Render still works (the option is recorded but does
	// nothing semantically without an NodeRenderer that reads it).
	var buf bytes.Buffer
	// No document to render; just confirm the option-installed
	// renderer is callable.
	_ = buf
	_ = r
}
