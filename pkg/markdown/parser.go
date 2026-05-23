package markdown

import (
	"sync"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// linkRefResetter is implemented by the fork's
// linkReferenceParagraphTransformer (internal/goldmark/parser/link_ref.go).
// The asserter lives here so pkg/markdown can clear the transformer's
// pinned document source bytes before returning the parent parser
// to the pool, without taking a hard dependency on the unexported
// transformer type.
type linkRefResetter interface {
	parser.ParagraphTransformer
	Reset()
}

// NewParser returns mdsmith's canonical goldmark parser: the default
// CommonMark block, inline, and paragraph parsers plus the
// processing-instruction block parser, so a <?include ... ?> block is
// a ProcessingInstruction node rather than a raw HTML block. This is
// the one parser configuration in the tree; the linter, sync-docs,
// and every other parse path consume it (directly or via
// internal/lint's forwards) so parsing decisions stay consistent
// across surfaces.
//
// The "goldmark" the import path resolves to is the in-tree fork at
// internal/goldmark/ (plan 197+198), wired via a go.mod replace
// directive. The fork's parser.DefaultParagraphTransformers returns
// a FRESH linkReferenceParagraphTransformer per call, so each parser
// built here owns its own transformer with its own reusable
// text.BlockReader — the per-paragraph allocation of upstream
// goldmark@v1.8.2 (parser/link_ref.go:18) is gone.
func NewParser() parser.Parser {
	p, _ := newPooledParser()
	return p
}

// newPooledParser builds one parser plus the link-ref transformer
// driving its paragraph pass, returning both so the pool can Reset
// the transformer's pinned document source between Get/Put pairs.
func newPooledParser() (parser.Parser, linkRefResetter) {
	defaults := parser.DefaultParagraphTransformers()
	// DefaultParagraphTransformers builds a fresh
	// linkReferenceParagraphTransformer at priority 100; locate it
	// by interface assertion so we can Reset it on Put. Any other
	// entries are preserved verbatim.
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
				PIBlockParserPrioritized(),
			)...,
		),
		parser.WithInlineParsers(
			parser.DefaultInlineParsers()...,
		),
		parser.WithParagraphTransformers(defaults...),
	)
	return p, lrp
}

// pooledParser pairs a parser.Parser with the link-ref transformer
// it owns, so ParseContext can Reset the Transformer's pinned
// document source bytes before returning the parser to the pool.
type pooledParser struct {
	parser parser.Parser
	lrp    linkRefResetter
}

// parserPool reuses canonical parsers across ParseContext calls.
// NewParser rebuilds a substantial config (default block, inline,
// and paragraph parsers plus the PI block parser) every call;
// constructing one per parse was a measurable share of allocations
// over the 600-file check gate (plan 175 profiling). A sync.Pool is
// the proven house pattern: each Get caller holds exclusive access
// to one parser-with-transformer pair until the matching Put, so
// there is no shared mutable parser even though parsing is driven
// from many goroutines at once (parallel check, the LSP serving
// concurrent documents). goldmark Parse keeps all per-parse state
// in the per-call parser.Context.
var parserPool = sync.Pool{
	New: func() any {
		p, lrp := newPooledParser()
		return &pooledParser{parser: p, lrp: lrp}
	},
}

// ParseContext parses src verbatim — no front-matter handling — with
// the canonical pooled parser, recording link-reference definitions
// and other parse state in ctx. The parser is borrowed for the
// duration of the Parse call only and returned immediately, so
// concurrent callers each hold a distinct instance. Most callers
// want Parse; this lower-level entry exists for callers that need
// the goldmark parser.Context (e.g. the linter file model reading
// link reference definitions).
//
// Before returning the parser to the pool, the link-ref transformer
// is Reset so that the last-parsed document's source bytes and
// BlockReader are not pinned by the idle pool slot.
func ParseContext(src []byte, ctx parser.Context) ast.Node {
	pp := parserPool.Get().(*pooledParser)
	defer func() {
		if pp.lrp != nil {
			pp.lrp.Reset()
		}
		parserPool.Put(pp)
	}()
	return pp.parser.Parse(text.NewReader(src), parser.WithContext(ctx))
}
