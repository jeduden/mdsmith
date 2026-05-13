package linkgraph

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"

	"github.com/jeduden/mdsmith/internal/lint"
)

// LinkRef is one parsed reference-style link occurrence
// (`[text][label]`). Lines are body-relative — same coordinate
// system as Link.Line; see the Link doc for why.
type LinkRef struct {
	Line   int
	Column int
	Text   string
	// Label is the normalized link-reference label (lowercased and
	// whitespace-collapsed via goldmark's util.ToLinkReference), so it
	// can be matched against the keys of the reference-definition map
	// the parser produces.
	Label string
}

// ExtractLinkRefs walks f.AST and returns every reference-style link
// (`[text][label]`) in document order. Direct links (`[text](url)`)
// surface through ExtractLinks instead.
func ExtractLinkRefs(f *lint.File) []LinkRef {
	if f == nil || f.AST == nil {
		return nil
	}
	var out []LinkRef
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		l, ok := n.(*ast.Link)
		if !ok || l.Reference == nil {
			return ast.WalkContinue, nil
		}
		line, col := linkPosition(f, l)
		out = append(out, LinkRef{
			Line:   line,
			Column: col,
			Text:   linkText(l, f.Source),
			Label:  string(util.ToLinkReference(l.Reference.Value)),
		})
		return ast.WalkContinue, nil
	})
	return out
}
