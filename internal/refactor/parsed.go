package refactor

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
)

// parseBody parses a front-matter-stripped body the way the engine
// does. It is a variable so tests can count parses.
var parseBody = func(body []byte) ast.Node {
	return lint.NewParser().Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
}

// parsedSource is one file's source split into body and front-matter
// offset, with its AST and validated reference definitions computed
// on first use and then shared, so the rename dispatch answers "is it
// a heading?", "is it a label?", "does the new label collide?", and
// "which bytes change?" from a single parse.
type parsedSource struct {
	source   []byte
	body     []byte
	fmOffset int

	ast      ast.Node
	defs     []validRefDefMatch
	defsDone bool

	lineIdx     bodyLineIndex
	lineIdxDone bool
}

// parseSource wraps source for shared, lazy parsing; it parses
// nothing until root or refDefs asks.
func parseSource(source []byte) *parsedSource {
	body, fm := bodyAndFMOffset(source)
	return &parsedSource{source: source, body: body, fmOffset: fm}
}

// root returns the body's AST, parsing it on the first call.
func (p *parsedSource) root() ast.Node {
	if p.ast == nil {
		p.ast = parseBody(p.body)
	}
	return p.ast
}

// refDefs returns the body's real reference definitions (see
// validRefDefMatches), computing them on the first call. A body with
// no `]:` holds no definition and is never parsed for them.
func (p *parsedSource) refDefs() []validRefDefMatch {
	if !p.defsDone {
		if bytes.Contains(p.body, []byte("]:")) {
			p.defs = refDefMatchesIn(p.body, p.root(), p.index())
		}
		p.defsDone = true
	}
	return p.defs
}

// index returns the body's line-start index, building it on the first
// call so every scan of one file shares it.
func (p *parsedSource) index() bodyLineIndex {
	if !p.lineIdxDone {
		p.lineIdx = newBodyLineIndex(p.body)
		p.lineIdxDone = true
	}
	return p.lineIdx
}
