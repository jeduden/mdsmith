package flavor

import (
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	"github.com/jeduden/mdsmith/pkg/markdown"
	"github.com/jeduden/mdsmith/pkg/markdown/flavor/ext"
)

// allFlavorExtensions is the canonical extension list the default
// flavor parser installs: the five goldmark built-ins (table,
// strikethrough, task list, footnote, definition list) plus the five
// custom MDS034 extensions (superscript, subscript, math block,
// math inline, abbreviations). The heading-ID attribute parser is
// enabled via parser.WithAttribute in the constructor itself, not as
// an extender.
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

// NewPooledParser returns a goldmark parser configured with the full
// flavor extension set (tables, strikethrough, task lists, footnotes,
// definition lists, heading IDs, plus the five custom MDS034
// extensions) and the canonical processing-instruction block parser,
// paired with a reset closure that clears the link-reference
// transformer's retained document bytes. sync.Pool consumers MUST
// call reset before Put.
//
// Building one parser is expensive; the in-package pool used by
// Detect and WithSharedParser caches one parser per goroutine. Most
// callers should prefer WithSharedParser, which borrows from that
// shared pool; use NewPooledParser only when you need to own the
// parser instance (long-lived caches, custom pools).
func NewPooledParser() (p parser.Parser, reset func()) {
	return newPooled(allFlavorExtensions())
}

// NewPooledParserWith is the parameterised form of NewPooledParser:
// pass the subset of goldmark extensions you need (plus the PI block
// parser and the heading-ID attribute parser are always installed).
// Callers that need only the Table extension — e.g. internal/schema's
// table-only re-parse — use this to avoid the AST-shape changes that
// other extensions would introduce.
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

// pooledParser pairs a borrowed goldmark parser with its reset
// closure so the pool can clear the link-reference transformer's
// pinned document bytes between Get/Put.
type pooledParser struct {
	parser parser.Parser
	reset  func()
}

// sharedPool reuses dual-extension parsers across WithSharedParser
// and Detect calls. Building one parser fans out goldmark.New plus
// every Extender's Extend hook; over a workspace check that cost
// shows up as a measurable fraction of CPU and allocations. The pool
// hands each goroutine its own instance and clears the link-reference
// transformer's pinned document bytes via the paired reset closure
// before Put.
var sharedPool = sync.Pool{
	New: func() any {
		p, reset := NewPooledParser()
		return &pooledParser{parser: p, reset: reset}
	},
}

// WithSharedParser borrows a dual-extension parser from the package
// pool, invokes fn with it, then resets the parser and returns it to
// the pool. The parser is pre-configured with the full flavor
// extension set, so callers that walk the dual AST themselves
// (rewriters that consume nodes Detect filters out, the rule's Fix
// path, contract tests) share the pool with Detect rather than
// rebuilding a parser each call.
//
// fn must complete synchronously. The borrowed parser is reused
// across goroutines via sync.Pool; do not retain it past the
// callback or pass it to another goroutine.
func WithSharedParser(fn func(p parser.Parser)) {
	pp := sharedPool.Get().(*pooledParser)
	defer func() {
		pp.reset()
		sharedPool.Put(pp)
	}()
	fn(pp.parser)
}

// newParserInternal is the single goldmark.New call site in the tree.
// It builds the canonical parser explicitly so the link-reference
// paragraph transformer captured below is the same instance the
// parser uses — and the only one. goldmark.New's own DefaultParser
// would install a second link-ref transformer that the returned
// reset closure could not reach, leaving pinned document bytes
// alive in the pool slot.
func newParserInternal(exts []goldmark.Extender) (parser.Parser, linkRefResetter) {
	defaults := parser.DefaultParagraphTransformers()
	var lrp linkRefResetter
	for _, pv := range defaults {
		if r, ok := pv.Value.(linkRefResetter); ok {
			lrp = r
			break
		}
	}
	p := parser.NewParser(
		parser.WithBlockParsers(
			append(parser.DefaultBlockParsers(),
				markdown.PIBlockParserPrioritized(),
			)...,
		),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(defaults...),
		parser.WithAttribute(),
	)
	// goldmark.New invokes each Extender's Extend(md) hook, which
	// calls md.Parser().AddOptions(...) to register additional
	// block / inline parsers on the parser installed by WithParser.
	goldmark.New(
		goldmark.WithParser(p),
		goldmark.WithExtensions(exts...),
	)
	return p, lrp
}
