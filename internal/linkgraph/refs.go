package linkgraph

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"

	"github.com/jeduden/mdsmith/internal/lint"
)

// RefLink is one reference-style link use (`[text][label]`,
// `[text][]`, or `[label]`).
//
// ExtractLinks skips these because reference-style destinations
// resolve through the link reference map at render time rather than
// via a URL, so callers that need to map "what file does this link
// point at" handle them separately (e.g. via the link-ref definition
// table in parser.Context).
//
// Line and Column are body-relative — same convention as Link.
type RefLink struct {
	Line   int
	Column int
	Text   string
	// Label is the link-reference label, normalised via
	// util.ToLinkReference (lower-cased, internal whitespace
	// collapsed). Use this when keying into the parser-context ref
	// table or matching against a `[label]: url` definition.
	Label string
}

// ExtractRefLinkTargets walks f.AST and returns every reference-style
// link whose definition has been resolved by the parser, as Link values
// with the resolved destination ready for the same file-existence
// resolver that ExtractLinks feeds. Images are not included — those
// come from ExtractImages. Lines are body-relative — same convention
// as Link.
func ExtractRefLinkTargets(f *lint.File) []Link {
	if f == nil || f.AST == nil {
		return nil
	}
	var out []Link
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		l, ok := n.(*ast.Link)
		if !ok || l.Reference == nil {
			return ast.WalkContinue, nil
		}
		target, ok := ParseTarget(string(l.Destination))
		if !ok {
			return ast.WalkContinue, nil
		}
		line, col := linkPosition(f, l)
		out = append(out, Link{
			Line:   line,
			Column: col,
			Text:   linkText(l, f.Source),
			Target: target,
		})
		return ast.WalkContinue, nil
	})
	return out
}

// ExtractRefLinks walks f.AST and returns every reference-style link
// in document order. Inline links (`[text](url)`) are intentionally
// excluded — those come from ExtractLinks.
func ExtractRefLinks(f *lint.File) []RefLink {
	if f == nil || f.AST == nil {
		return nil
	}
	var out []RefLink
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		l, ok := n.(*ast.Link)
		if !ok || l.Reference == nil {
			return ast.WalkContinue, nil
		}
		line, col := linkPosition(f, l)
		out = append(out, RefLink{
			Line:   line,
			Column: col,
			Text:   linkText(l, f.Source),
			Label:  string(util.ToLinkReference(l.Reference.Value)),
		})
		return ast.WalkContinue, nil
	})
	return out
}
