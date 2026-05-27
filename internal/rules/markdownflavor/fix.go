package markdownflavor

import (
	"sort"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	gparser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/markdown"
	"github.com/jeduden/mdsmith/pkg/markdown/flavor"
	"github.com/jeduden/mdsmith/pkg/markdown/flavor/ext"
)

// fixByteRangeFeatures collects edits for the six byte-range features
// (heading IDs, strikethrough, task lists, superscript, subscript, and
// bare-URL autolinks) and returns the rewritten source. Features that
// the configured flavor accepts are skipped. Returns f.Source unchanged
// when no edit applies.
func (r *Rule) fixByteRangeFeatures(f *lint.File) []byte {
	var edits []markdown.Edit

	if r.needsAnyDualFix() {
		flavor.WithSharedParser(func(p gparser.Parser) {
			doc := p.Parse(text.NewReader(f.Source))
			_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
				if !entering {
					return ast.WalkContinue, nil
				}
				edits = append(edits, r.dualNodeEdits(f, n)...)
				return ast.WalkContinue, nil
			})
		})
	}

	if !flavor.Supports(r.Flavor, flavor.FeatureBareURLAutolinks) {
		doc := &markdown.Document{Body: f.Source, AST: f.AST}
		acceptBareURLs := func(feat flavor.Feature) bool {
			return feat == flavor.FeatureBareURLAutolinks
		}
		for _, fin := range flavor.Detect(doc, acceptBareURLs) {
			edits = append(edits, wrapBareURL(f.Source, fin))
		}
	}

	if len(edits) == 0 {
		return f.Source
	}
	// markdown.Splice expects ascending, non-overlapping edits; the
	// detection layer never produces overlapping fixes, but a dual-AST
	// walk and a bare-URL pass merged here can land out of source order,
	// so sort before handing off.
	sort.SliceStable(edits, func(i, j int) bool {
		return edits[i].Start < edits[j].Start
	})
	return markdown.Splice(f.Source, edits)
}

// needsAnyDualFix reports whether any dual-parser fixable feature is
// unsupported by the configured flavor. Skips the dual re-parse for
// flavors that accept every dual feature (e.g. flavor.FlavorAny,
// flavor.FlavorPandoc).
func (r *Rule) needsAnyDualFix() bool {
	for _, feat := range []flavor.Feature{
		flavor.FeatureHeadingIDs, flavor.FeatureStrikethrough,
		flavor.FeatureTaskLists, flavor.FeatureSuperscript,
		flavor.FeatureSubscript,
	} {
		if !flavor.Supports(r.Flavor, feat) {
			return true
		}
	}
	return false
}

// dualNodeEdits returns the edits to remove an unsupported feature
// produced from a dual-parser AST node. Returns nil when the node is
// either supported or not a fixable feature.
func (r *Rule) dualNodeEdits(f *lint.File, n ast.Node) []markdown.Edit {
	switch node := n.(type) {
	case *ast.Heading:
		if flavor.Supports(r.Flavor, flavor.FeatureHeadingIDs) {
			return nil
		}
		return headingIDEdits(f, node)
	case *extast.Strikethrough:
		if flavor.Supports(r.Flavor, flavor.FeatureStrikethrough) {
			return nil
		}
		return delimiterPairEdits(node, len("~~"))
	case *extast.TaskCheckBox:
		if flavor.Supports(r.Flavor, flavor.FeatureTaskLists) {
			return nil
		}
		return taskCheckBoxEdits(f, node)
	case *ext.SuperscriptNode:
		if flavor.Supports(r.Flavor, flavor.FeatureSuperscript) {
			return nil
		}
		return delimiterPairEdits(node, len("^"))
	case *ext.SubscriptNode:
		if flavor.Supports(r.Flavor, flavor.FeatureSubscript) {
			return nil
		}
		return delimiterPairEdits(node, len("~"))
	}
	return nil
}

// headingIDEdits returns the edit that drops a "{#id}" attribute block
// plus any whitespace separating it from the heading text. Returns nil
// when the heading carries no id attribute.
func headingIDEdits(f *lint.File, h *ast.Heading) []markdown.Edit {
	hx, ok := flavor.FindHeadingID(f.Source, h)
	if !ok {
		return nil
	}
	start := hx.AttrStart
	for start > 0 && (f.Source[start-1] == ' ' || f.Source[start-1] == '\t') {
		start--
	}
	return []markdown.Edit{{Start: start, End: hx.AttrEnd}}
}

// delimiterPairEdits returns edits removing the opening and closing
// delimiter runs that wrap an inline node. Only nodes with a single
// Text child are fixed; goldmark merges adjacent text inlines so a
// second sibling (whether Text-typed or not) implies nested inline
// markup like `~~*bold*~~` or a softbreak inside the wrapper, where
// reconstructing each child's own marker span is brittle. The fix
// declines and the diagnostic remains for the user to resolve.
func delimiterPairEdits(n ast.Node, markerLen int) []markdown.Edit {
	t, ok := n.FirstChild().(*ast.Text)
	if !ok || t.NextSibling() != nil {
		return nil
	}
	return []markdown.Edit{
		{Start: t.Segment.Start - markerLen, End: t.Segment.Start},
		{Start: t.Segment.Stop, End: t.Segment.Stop + markerLen},
	}
}

// taskCheckBoxEdits removes the "[X]" run plus a single trailing
// space when present. Per the plan, the bullet itself is preserved.
//
// Relies on the goldmark task-list parser's invariant: it always
// wraps a TaskCheckBox in a TextBlock whose first Lines() segment
// starts at the '['. NearestBlockAncestor returns that TextBlock,
// so block.Lines().At(0).Start indexes the bracket. If a hand-built
// AST violates the invariant — TextBlock has empty Lines so
// NearestBlockAncestor returns the enclosing ListItem instead —
// start would point at the bullet ('- ') and start+3 would silently
// delete the wrong bytes. The fix declines via the block==nil /
// empty-Lines guards rather than producing corrupt output.
func taskCheckBoxEdits(f *lint.File, n *extast.TaskCheckBox) []markdown.Edit {
	block := flavor.NearestBlockAncestor(n)
	if block == nil {
		return nil
	}
	lines := block.Lines()
	if lines == nil || lines.Len() == 0 {
		return nil
	}
	start := lines.At(0).Start
	end := start + 3
	if end < len(f.Source) && f.Source[end] == ' ' {
		end++
	}
	return []markdown.Edit{{Start: start, End: end}}
}

// wrapBareURL wraps a bare URL in angle brackets so the renderer
// treats it as a CommonMark autolink. The detector reports a precise
// span via fin.Start / fin.End.
func wrapBareURL(source []byte, fin flavor.Finding) markdown.Edit {
	url := source[fin.Start:fin.End]
	repl := make([]byte, 0, len(url)+2)
	repl = append(repl, '<')
	repl = append(repl, url...)
	repl = append(repl, '>')
	return markdown.Edit{Start: fin.Start, End: fin.End, Repl: repl}
}
