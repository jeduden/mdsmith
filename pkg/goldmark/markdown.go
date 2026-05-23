// Package goldmark implements a Markdown parser. mdsmith vendors this
// fork to thread a per-parser BlockReader (plan 197) and absorb the
// four structural allocators (plan 198) without rebuilding goldmark
// from scratch. The package layout is identical to upstream so
// every consumer import path stays `github.com/yuin/goldmark/...`.
package goldmark

import (
	"io"

	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// DefaultParser returns a new Parser configured with goldmark's
// default block parsers, inline parsers, and paragraph transformers.
func DefaultParser() parser.Parser {
	return parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
}

// DefaultRenderer returns a new Renderer configured by default values.
func DefaultRenderer() renderer.Renderer {
	return renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(), 1000)))
}

var defaultMarkdown = New()

// Convert interprets a UTF-8 bytes source in Markdown and writes the
// rendered output to w. mdsmith does not call this — it parses only —
// but the upstream extension Extend methods register HTML node
// renderers, so the rendering pipeline stays wired.
func Convert(source []byte, w io.Writer, opts ...parser.ParseOption) error {
	return defaultMarkdown.Convert(source, w, opts...)
}

// A Markdown converts Markdown text to a desired format.
type Markdown interface {
	// Convert reads UTF-8 Markdown from source, parses it, and
	// writes rendered output to w.
	Convert(source []byte, writer io.Writer, opts ...parser.ParseOption) error

	// Parser returns the Parser that will be used to build the AST.
	Parser() parser.Parser

	// SetParser swaps the underlying Parser.
	SetParser(parser.Parser)

	// Renderer returns the Renderer that will be used to emit output.
	Renderer() renderer.Renderer

	// SetRenderer swaps the underlying Renderer.
	SetRenderer(renderer.Renderer)
}

// Option is a functional option type for Markdown objects.
type Option func(*markdown)

// WithExtensions adds the given extensions to the Markdown.
func WithExtensions(ext ...Extender) Option {
	return func(m *markdown) {
		m.extensions = append(m.extensions, ext...)
	}
}

// WithParser overrides the default parser.
func WithParser(p parser.Parser) Option {
	return func(m *markdown) {
		m.parser = p
	}
}

// WithParserOptions applies options to the parser.
func WithParserOptions(opts ...parser.Option) Option {
	return func(m *markdown) {
		m.parser.AddOptions(opts...)
	}
}

// WithRenderer overrides the default renderer.
func WithRenderer(r renderer.Renderer) Option {
	return func(m *markdown) {
		m.renderer = r
	}
}

// WithRendererOptions applies options to the renderer.
func WithRendererOptions(opts ...renderer.Option) Option {
	return func(m *markdown) {
		m.renderer.AddOptions(opts...)
	}
}

type markdown struct {
	parser     parser.Parser
	renderer   renderer.Renderer
	extensions []Extender
}

// New returns a new Markdown configured by the given options. Each
// registered extension's Extend method is invoked before return.
func New(options ...Option) Markdown {
	md := &markdown{
		parser:     DefaultParser(),
		renderer:   DefaultRenderer(),
		extensions: []Extender{},
	}
	for _, opt := range options {
		opt(md)
	}
	for _, e := range md.extensions {
		e.Extend(md)
	}
	return md
}

func (m *markdown) Convert(source []byte, writer io.Writer, opts ...parser.ParseOption) error {
	reader := text.NewReader(source)
	doc := m.parser.Parse(reader, opts...)
	return m.renderer.Render(writer, source, doc)
}

func (m *markdown) Parser() parser.Parser            { return m.parser }
func (m *markdown) SetParser(v parser.Parser)        { m.parser = v }
func (m *markdown) Renderer() renderer.Renderer      { return m.renderer }
func (m *markdown) SetRenderer(v renderer.Renderer)  { m.renderer = v }

// An Extender hooks additional parsers/renderers onto a Markdown.
type Extender interface {
	Extend(Markdown)
}
