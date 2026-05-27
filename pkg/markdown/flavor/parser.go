package flavor

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	"github.com/jeduden/mdsmith/pkg/markdown"
	"github.com/jeduden/mdsmith/pkg/markdown/flavor/ext"
)

// allFlavorExtensions is the canonical extension list NewParser
// installs: the five goldmark built-ins (table, strikethrough, task
// list, footnote, definition list) plus the five custom MDS034
// extensions (superscript, subscript, math block, math inline,
// abbreviations). The heading-ID attribute parser is enabled via
// parser.WithAttribute in the constructor itself, not as an extender.
func allFlavorExtensions() []goldmark.Extender {
	return []goldmark.Extender{
		extension.Table,
		extension.Strikethrough,
		extension.TaskList,
		extension.Footnote,
		extension.DefinitionList,
		ext.Superscript,
		ext.Subscript,
		ext.MathBlock,
		ext.MathInline,
		ext.Abbreviation,
	}
}

// linkRefResetter is implemented by goldmark's link-reference
// paragraph transformer. It is identified by duck typing so pool
// callers can clear the transformer's retained document bytes
// between Get/Put without taking a hard dependency on the unexported
// transformer type. The transformer is part of
// parser.DefaultParagraphTransformers.
type linkRefResetter interface {
	parser.ParagraphTransformer
	Reset()
}

// NewParser returns a goldmark parser configured with the full
// flavor extension set: the five built-in goldmark extensions, the
// heading-ID attribute parser, and the five custom MDS034
// extensions. The canonical processing-instruction block parser is
// registered alongside so <?include ... ?> blocks parse as
// ProcessingInstruction nodes — just as pkg/markdown.NewParser
// produces — rather than as HTML blocks.
//
// Building one parser is expensive; callers MUST cache or pool.
// Pool consumers should use NewPooledParser to also receive a reset
// closure that clears the link-reference transformer's retained
// document bytes.
func NewParser() parser.Parser {
	p, _ := newParserInternal(allFlavorExtensions())
	return p
}

// NewParserWith returns a goldmark parser with the given extensions
// (plus the PI block parser and the heading-ID attribute parser).
// Callers that need only a subset of the flavor extensions — e.g.
// internal/schema's table-only re-parse — use this to avoid the
// AST-shape changes that other extensions would introduce.
func NewParserWith(exts ...goldmark.Extender) parser.Parser {
	p, _ := newParserInternal(exts)
	return p
}

// NewPooledParser returns NewParser paired with a reset closure that
// clears the link-reference transformer's retained document bytes.
// sync.Pool consumers MUST call reset before Put.
func NewPooledParser() (p parser.Parser, reset func()) {
	return newPooled(allFlavorExtensions())
}

// NewPooledParserWith is the parameterised form of NewPooledParser:
// like NewParserWith but pool-aware.
func NewPooledParserWith(exts ...goldmark.Extender) (p parser.Parser, reset func()) {
	return newPooled(exts)
}

func newPooled(exts []goldmark.Extender) (parser.Parser, func()) {
	p, lrp := newParserInternal(exts)
	return p, func() {
		if lrp != nil {
			lrp.Reset()
		}
	}
}

// newParserInternal is the single goldmark.New call site in the tree.
// It builds the canonical parser plus extracts the link-reference
// paragraph transformer so pool callers can Reset its retained
// document bytes.
func newParserInternal(exts []goldmark.Extender) (parser.Parser, linkRefResetter) {
	defaults := parser.DefaultParagraphTransformers()
	var lrp linkRefResetter
	for _, pv := range defaults {
		if r, ok := pv.Value.(linkRefResetter); ok {
			lrp = r
			break
		}
	}
	md := goldmark.New(
		goldmark.WithExtensions(exts...),
		goldmark.WithParserOptions(
			parser.WithAttribute(),
			parser.WithBlockParsers(
				markdown.PIBlockParserPrioritized(),
			),
			parser.WithParagraphTransformers(defaults...),
		),
	)
	return md.Parser(), lrp
}
