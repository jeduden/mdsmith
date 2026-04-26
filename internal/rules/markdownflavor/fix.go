package markdownflavor

import (
	"sort"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/markdownflavor/ext"
)

// edit describes a single byte-range substitution to apply to source.
// applyEdits assumes non-overlapping spans and rewrites the buffer in
// one pass.
type edit struct {
	start, end int
	repl       []byte
}

// fixByteRangeFeatures collects edits for the six byte-range features
// (heading IDs, strikethrough, task lists, superscript, subscript, and
// bare-URL autolinks) and returns the rewritten source. Features that
// the configured flavor accepts are skipped. The function returns the
// source unchanged when no edit applies.
func (r *Rule) fixByteRangeFeatures(f *lint.File) []byte {
	var edits []edit

	if r.needsAnyDualFix() {
		doc := Parser().Parser().Parse(text.NewReader(f.Source))
		_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			edits = append(edits, r.dualNodeEdits(f, n)...)
			return ast.WalkContinue, nil
		})
	}

	if !r.Flavor.Supports(FeatureBareURLAutolinks) {
		for _, fin := range detectBareURLs(f) {
			edits = append(edits, wrapBareURL(f.Source, fin))
		}
	}

	if len(edits) == 0 {
		return f.Source
	}
	return applyEdits(f.Source, edits)
}

// needsAnyDualFix reports whether any dual-parser fixable feature is
// unsupported by the configured flavor. Skips the dual re-parse for
// flavors that accept every dual feature (e.g. FlavorAny, Pandoc).
func (r *Rule) needsAnyDualFix() bool {
	for _, feat := range []Feature{
		FeatureHeadingIDs, FeatureStrikethrough, FeatureTaskLists,
		FeatureSuperscript, FeatureSubscript,
	} {
		if !r.Flavor.Supports(feat) {
			return true
		}
	}
	return false
}

// dualNodeEdits returns the edits to remove an unsupported feature
// produced from a dual-parser AST node. Returns nil when the node is
// either supported or not a fixable feature.
func (r *Rule) dualNodeEdits(f *lint.File, n ast.Node) []edit {
	switch node := n.(type) {
	case *ast.Heading:
		if r.Flavor.Supports(FeatureHeadingIDs) {
			return nil
		}
		return headingIDEdits(f, node)
	case *extast.Strikethrough:
		if r.Flavor.Supports(FeatureStrikethrough) {
			return nil
		}
		return delimiterPairEdits(node, len("~~"))
	case *extast.TaskCheckBox:
		if r.Flavor.Supports(FeatureTaskLists) {
			return nil
		}
		return taskCheckBoxEdits(f, node)
	case *ext.SuperscriptNode:
		if r.Flavor.Supports(FeatureSuperscript) {
			return nil
		}
		return delimiterPairEdits(node, len("^"))
	case *ext.SubscriptNode:
		if r.Flavor.Supports(FeatureSubscript) {
			return nil
		}
		return delimiterPairEdits(node, len("~"))
	}
	return nil
}

// headingIDEdits returns the edit that drops a "{#id}" attribute block
// plus any whitespace separating it from the heading text. Returns nil
// when the heading carries no id attribute. findHeadingID always stores
// a HeadingIDExtra in fin.Extra, so the type assertion is unchecked.
func headingIDEdits(f *lint.File, h *ast.Heading) []edit {
	fin, ok := findHeadingID(f, h)
	if !ok {
		return nil
	}
	hx := fin.Extra.(HeadingIDExtra)
	start := hx.AttrStart
	for start > 0 && (f.Source[start-1] == ' ' || f.Source[start-1] == '\t') {
		start--
	}
	return []edit{{start: start, end: hx.AttrEnd}}
}

// delimiterPairEdits returns edits removing the opening and closing
// delimiter runs that wrap an inline node. Only nodes with a single
// Text child are fixed; goldmark merges adjacent text inlines so a
// second sibling (whether Text-typed or not) implies nested inline
// markup like `~~*bold*~~` or a softbreak inside the wrapper, where
// reconstructing each child's own marker span is brittle. The fix
// declines and the diagnostic remains for the user to resolve.
func delimiterPairEdits(n ast.Node, markerLen int) []edit {
	t, ok := n.FirstChild().(*ast.Text)
	if !ok || t.NextSibling() != nil {
		return nil
	}
	return []edit{
		{start: t.Segment.Start - markerLen, end: t.Segment.Start},
		{start: t.Segment.Stop, end: t.Segment.Stop + markerLen},
	}
}

// taskCheckBoxEdits removes the "[X]" run plus a single trailing
// space when present. Per the plan, the bullet itself is preserved.
// The dual parser places every TaskCheckBox at the start of a
// TextBlock so block.Lines().At(0).Start always points at '['.
func taskCheckBoxEdits(f *lint.File, n *extast.TaskCheckBox) []edit {
	block := nearestBlockAncestor(n)
	start := block.Lines().At(0).Start
	end := start + 3
	if end < len(f.Source) && f.Source[end] == ' ' {
		end++
	}
	return []edit{{start: start, end: end}}
}

// wrapBareURL wraps a bare URL in angle brackets so the renderer treats
// it as a CommonMark autolink. The detector reports a precise span via
// fin.Start / fin.End.
func wrapBareURL(source []byte, fin Finding) edit {
	url := source[fin.Start:fin.End]
	repl := make([]byte, 0, len(url)+2)
	repl = append(repl, '<')
	repl = append(repl, url...)
	repl = append(repl, '>')
	return edit{start: fin.Start, end: fin.End, repl: repl}
}

// applyEdits rewrites src by appending unchanged spans and replacement
// bytes in a single pass. Edits are sorted by ascending start offset;
// the detection layer never produces overlapping edits for the
// features we fix, so applyEdits assumes non-overlapping spans.
func applyEdits(src []byte, edits []edit) []byte {
	sort.SliceStable(edits, func(i, j int) bool {
		return edits[i].start < edits[j].start
	})
	size := len(src)
	for _, e := range edits {
		size += len(e.repl) - (e.end - e.start)
	}
	out := make([]byte, 0, size)
	cursor := 0
	for _, e := range edits {
		out = append(out, src[cursor:e.start]...)
		out = append(out, e.repl...)
		cursor = e.end
	}
	out = append(out, src[cursor:]...)
	return out
}
