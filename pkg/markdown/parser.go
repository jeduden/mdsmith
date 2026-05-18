package markdown

import (
	"sync"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// NewParser returns mdsmith's canonical goldmark parser: the default
// CommonMark block, inline, and paragraph parsers plus the
// processing-instruction block parser, so a <?include ... ?> block is
// a ProcessingInstruction node rather than a raw HTML block. This is
// the one parser configuration in the tree; the linter, sync-docs,
// and every other parse path consume it (directly or via
// internal/lint's forwards) so parsing decisions stay consistent
// across surfaces.
func NewParser() parser.Parser {
	return parser.NewParser(
		parser.WithBlockParsers(
			append(parser.DefaultBlockParsers(),
				PIBlockParserPrioritized(),
			)...,
		),
		parser.WithInlineParsers(
			parser.DefaultInlineParsers()...,
		),
		parser.WithParagraphTransformers(
			parser.DefaultParagraphTransformers()...,
		),
	)
}

// parserPool reuses canonical parsers across ParseContext calls.
// NewParser rebuilds a substantial config (default block, inline, and
// paragraph parsers plus the PI block parser) every call; constructing
// one per parse was a measurable share of allocations over the
// 600-file check gate (plan 175 profiling). A sync.Pool is the proven
// house pattern: each goroutine Gets its own instance and Puts it
// back, so there is no shared mutable parser even though parsing is
// driven from many goroutines at once (parallel check, the LSP serving
// concurrent documents). goldmark Parse keeps all per-parse state in
// the per-call parser.Context.
var parserPool = sync.Pool{
	New: func() any { return NewParser() },
}

// ParseContext parses src verbatim — no front-matter handling — with
// the canonical pooled parser, recording link-reference definitions
// and other parse state in ctx. The parser is borrowed for the
// duration of the Parse call only and returned immediately, so
// concurrent callers each hold a distinct instance. Most callers want
// Parse; this lower-level entry exists for callers that need the
// goldmark parser.Context (e.g. the linter file model reading link
// reference definitions).
func ParseContext(src []byte, ctx parser.Context) ast.Node {
	p := parserPool.Get().(parser.Parser)
	defer parserPool.Put(p)
	return p.Parse(text.NewReader(src), parser.WithContext(ctx))
}
