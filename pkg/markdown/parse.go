package markdown

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
)

// Document is the result of Parse: the YAML front matter split off the
// top of the source, the body it was split from, and the goldmark AST
// of that body built with mdsmith's canonical parser.
type Document struct {
	// FrontMatter is the raw front-matter prefix including its ---
	// fences, or nil when the source has no front matter.
	FrontMatter []byte
	// Body is source with the FrontMatter prefix removed. AST indexes
	// into these bytes; pass Body to Splice when editing.
	Body []byte
	// AST is the goldmark document node parsed from Body.
	AST ast.Node
}

// Parse splits YAML front matter off source and parses the remaining
// body with the canonical parser (CommonMark plus the
// <?...?> processing-instruction block). It never errors and never
// panics: empty, body-only, and front-matter-only inputs all yield a
// document node (with an empty body for the front-matter-only case).
func Parse(source []byte) *Document {
	fm, body := StripFrontMatter(source)
	root := ParseContext(body, parser.NewContext())
	return &Document{FrontMatter: fm, Body: body, AST: root}
}
