package linkgraph

import (
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/util"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
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
	// Label is the link-reference label, normalised via
	// util.ToLinkReference (lower-cased, internal whitespace
	// collapsed). Use this when keying into the parser-context ref
	// table or matching against a `[label]: url` definition.
	Label string

	// node is the AST node the reference link was extracted from, kept
	// so Text can flatten the visible label on demand. nil on a
	// RefLink built outside the walk, which Text reports as empty.
	node ast.Node
}

// Text returns the visible link text, flattened to plain text.
//
// Resolved on demand for the same reason as Link.Text: the only
// production consumer of ExtractRefLinks (internal/index/build.go)
// reads Line, Column and Label and never the label text, so building
// it during the walk allocated a bytes.Buffer plus a string copy per
// reference link in every file. See
// docs/development/high-performance-go.md#skip-work-you-dont-need.
//
// source must be the Source of the File the RefLink came from.
func (r RefLink) Text(source []byte) string {
	if r.node == nil {
		return ""
	}
	return mdtext.ExtractPlainText(r.node, source)
}

// RefLinkTargets returns every reference-style link whose definition has
// been resolved by the parser, memoized on the per-Check File. Two calls
// on the same File return the same backing slice; callers must treat it
// as read-only. The result is byte-identical to ExtractRefLinkTargets.
// nil is returned for a nil or AST-less File.
//
// Memoized via File.MemoFile: buildRefLinkTargets is a package-level
// function so the call adds no per-Memo-call heap allocation beyond the
// cold-path memoEntry.
func RefLinkTargets(f *lint.File) []Link {
	if f == nil {
		return nil
	}
	links, _ := f.MemoFile("linkgraph.reflinktargets", buildRefLinkTargets).([]Link)
	return links
}

// buildRefLinkTargets is the MemoFile-style builder for the RefLinkTargets memo.
func buildRefLinkTargets(f *lint.File) any {
	return ExtractRefLinkTargets(f)
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
		target, ok := ParseTargetBytes(l.Destination)
		if !ok {
			return ast.WalkContinue, nil
		}
		line, col := linkPosition(f, l)
		out = append(out, Link{
			Line:   line,
			Column: col,
			Target: target,
			node:   l,
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
			Label:  string(util.ToLinkReference(l.Reference.Value)),
			node:   l,
		})
		return ast.WalkContinue, nil
	})
	return out
}
