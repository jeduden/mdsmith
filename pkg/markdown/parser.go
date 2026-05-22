package markdown

import (
	"sync"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/jeduden/mdsmith/internal/goldmark/linkrefparagraph"
)

// NewParser returns mdsmith's canonical goldmark parser: the default
// CommonMark block, inline, and paragraph parsers plus the
// processing-instruction block parser, so a <?include ... ?> block is
// a ProcessingInstruction node rather than a raw HTML block. This is
// the one parser configuration in the tree; the linter, sync-docs,
// and every other parse path consume it (directly or via
// internal/lint's forwards) so parsing decisions stay consistent
// across surfaces.
//
// Plan 197 substitutes goldmark's singleton
// LinkReferenceParagraphTransformer with a per-parser
// linkrefparagraph.Transformer that reuses a text.BlockReader across
// paragraphs. Every other entry in goldmark's
// DefaultParagraphTransformers list is preserved verbatim, so a
// future goldmark upgrade that adds a default transformer flows
// through unchanged.
func NewParser() parser.Parser {
	p, _ := newPooledParser()
	return p
}

// newPooledParser builds one parser plus the linkref Transformer
// that drives its link-reference paragraph pass, returning both so
// the pool can Reset the Transformer between Get/Put pairs.
func newPooledParser() (parser.Parser, *linkrefparagraph.Transformer) {
	lrp := linkrefparagraph.New()
	p := parser.NewParser(
		parser.WithBlockParsers(
			append(parser.DefaultBlockParsers(),
				PIBlockParserPrioritized(),
			)...,
		),
		parser.WithInlineParsers(
			parser.DefaultInlineParsers()...,
		),
		parser.WithParagraphTransformers(
			substituteLinkRef(parser.DefaultParagraphTransformers(), lrp)...,
		),
	)
	return p, lrp
}

// substituteLinkRef returns defaults with goldmark's
// LinkReferenceParagraphTransformer entry replaced by lrp at the
// same priority. Any other default transformers (none today, but a
// future goldmark upgrade may add them) are preserved verbatim.
func substituteLinkRef(defaults []util.PrioritizedValue, lrp *linkrefparagraph.Transformer) []util.PrioritizedValue {
	out := make([]util.PrioritizedValue, len(defaults))
	for i, pv := range defaults {
		if pv.Value == parser.LinkReferenceParagraphTransformer {
			out[i] = util.Prioritized(lrp, pv.Priority)
			continue
		}
		out[i] = pv
	}
	return out
}

// pooledParser pairs a parser.Parser with the linkref Transformer
// it owns, so ParseContext can Reset the Transformer's pinned
// document source bytes before returning the parser to the pool.
type pooledParser struct {
	parser parser.Parser
	lrp    *linkrefparagraph.Transformer
}

// parserPool reuses canonical parsers across ParseContext calls.
// NewParser rebuilds a substantial config (default block, inline, and
// paragraph parsers plus the PI block parser) every call; constructing
// one per parse was a measurable share of allocations over the
// 600-file check gate (plan 175 profiling). A sync.Pool is the proven
// house pattern: each Get caller holds exclusive access to one
// parser-with-transformer pair until the matching Put, so there is
// no shared mutable parser even though parsing is driven from many
// goroutines at once (parallel check, the LSP serving concurrent
// documents). goldmark Parse keeps all per-parse state in the
// per-call parser.Context.
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
// concurrent callers each hold a distinct instance. Most callers want
// Parse; this lower-level entry exists for callers that need the
// goldmark parser.Context (e.g. the linter file model reading link
// reference definitions).
//
// Before returning the parser to the pool, the link-ref Transformer
// is Reset so that the last-parsed document's source bytes and
// BlockReader are not pinned by the idle pool slot.
func ParseContext(src []byte, ctx parser.Context) ast.Node {
	pp := parserPool.Get().(*pooledParser)
	defer func() {
		pp.lrp.Reset()
		parserPool.Put(pp)
	}()
	return pp.parser.Parse(text.NewReader(src), parser.WithContext(ctx))
}
