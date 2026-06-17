// Package listmarkerspace implements MDS061, which enforces a consistent
// number of spaces between a list marker and item text, configurable per
// single-line vs multi-paragraph items and ordered vs unordered lists.
package listmarkerspace

import (
	"bytes"
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/listscan"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

func init() {
	rule.Register(&Rule{ULSingle: 1, ULMulti: 1, OLSingle: 1, OLMulti: 1})
}

// Rule enforces the number of spaces between a list marker and item text.
type Rule struct {
	ULSingle int
	ULMulti  int
	OLSingle int
	OLMulti  int
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS061" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "list-marker-space" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "list" }

// Check implements rule.Rule. The per-item logic reads only the item's
// marker line (marker plus following spaces), the list's ordered-ness,
// and whether the item is multi-block — never the inline tree — so on a
// parsed File it folds into the engine's shared AST walk (rule.WalkNodes),
// and on a parse-skipped File (f.AST nil) it re-derives the same lists,
// item lines, and multi-block flag from f.Lines via listscan
// (checkLayer0). Both resolve the same per-item verdict, so the
// diagnostics are identical.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		return r.checkLayer0(f)
	}
	return rule.WalkNodes(r, f)
}

// checkLayer0 is the nil-AST counterpart of CheckNode. listscan groups
// the same lists goldmark would, with the same item marker lines,
// ordered-ness, and multi-block classification, so each item's
// marker-space verdict is byte-identical.
func (r *Rule) checkLayer0(f *lint.File) []lint.Diagnostic {
	lists, _ := listscan.Parse(f.Lines)
	var diags []lint.Diagnostic
	for _, l := range lists {
		for _, it := range l.Items {
			if d, ok := r.itemVerdict(f, l.Ordered, it.MultiBlock, it.Line); ok {
				diags = append(diags, d)
			}
		}
	}
	return diags
}

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	list, ok := n.(*ast.List)
	if !ok {
		return nil
	}
	return r.checkList(f, list)
}

func (r *Rule) checkList(f *lint.File, list *ast.List) []lint.Diagnostic {
	var diags []lint.Diagnostic
	ordered := list.IsOrdered()
	for c := list.FirstChild(); c != nil; c = c.NextSibling() {
		item := c.(*ast.ListItem)
		if d, ok := r.itemVerdict(f, ordered, isMultiItem(item), firstLineOfListItem(f, item)); ok {
			diags = append(diags, d)
		}
	}
	return diags
}

// itemVerdict is the shared per-item check both the AST (checkList via
// CheckNode) and Layer 0 (checkLayer0) paths drive: ordered is the list's
// ordered-ness, multi whether the item is multi-block, and line the
// item's 1-based marker line. It reads the marker and following spaces
// from the source line, so the two paths agree by construction.
func (r *Rule) itemVerdict(f *lint.File, ordered, multi bool, line int) (lint.Diagnostic, bool) {
	want := r.configuredSpaces(ordered, multi)
	if line <= 0 || line > len(f.Lines) {
		return lint.Diagnostic{}, false
	}
	markerEnd, got := parseMarkerAndSpaces(f.Lines[line-1])
	if markerEnd == 0 || got == want {
		return lint.Diagnostic{}, false
	}
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message: fmt.Sprintf(
			"list marker followed by %d %s; expected %d",
			got, pluralSpace(got), want,
		),
	}, true
}

func (r *Rule) configuredSpaces(ordered, multi bool) int {
	switch {
	case !ordered && !multi:
		return r.ULSingle
	case !ordered && multi:
		return r.ULMulti
	case ordered && !multi:
		return r.OLSingle
	default:
		return r.OLMulti
	}
}

// isMultiItem returns true when the list item has more than one block child.
func isMultiItem(item *ast.ListItem) bool {
	count := 0
	for c := item.FirstChild(); c != nil; c = c.NextSibling() {
		count++
	}
	return count > 1
}

// parseMarkerAndSpaces returns the byte offset just after the list marker
// and the count of space characters that follow it. Returns (0, 0) when
// the line contains no recognizable list marker.
func parseMarkerAndSpaces(line []byte) (markerEnd int, spaceCount int) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) {
		return 0, 0
	}
	if line[i] == '-' || line[i] == '*' || line[i] == '+' {
		markerEnd = i + 1
	} else if line[i] >= '0' && line[i] <= '9' {
		j := i
		for j < len(line) && line[j] >= '0' && line[j] <= '9' {
			j++
		}
		if j < len(line) && (line[j] == '.' || line[j] == ')') {
			markerEnd = j + 1
		} else {
			return 0, 0
		}
	} else {
		return 0, 0
	}
	j := markerEnd
	for j < len(line) && line[j] == ' ' {
		spaceCount++
		j++
	}
	return markerEnd, spaceCount
}

// firstLineOfListItem returns the source line number of the first
// text-bearing direct child of the list item. Items whose first child
// carries no source lines (e.g. a paragraphless outer item that contains
// only a nested sub-list) return 0 and are skipped by the caller.
func firstLineOfListItem(f *lint.File, li *ast.ListItem) int {
	for c := li.FirstChild(); c != nil; c = c.NextSibling() {
		if c.Lines().Len() > 0 {
			return f.LineOfOffset(c.Lines().At(0).Start)
		}
	}
	return 0
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	editMap := make(map[int]int)
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		list, ok := n.(*ast.List)
		if !ok {
			return ast.WalkContinue, nil
		}
		ordered := list.IsOrdered()
		for c := list.FirstChild(); c != nil; c = c.NextSibling() {
			item := c.(*ast.ListItem)
			multi := isMultiItem(item)
			want := r.configuredSpaces(ordered, multi)
			line := firstLineOfListItem(f, item)
			if line <= 0 || line > len(f.Lines) {
				continue
			}
			markerEnd, got := parseMarkerAndSpaces(f.Lines[line-1])
			if markerEnd == 0 || got == want || multi {
				continue
			}
			editMap[line] = want
		}
		return ast.WalkContinue, nil
	})

	resultLines := make([][]byte, len(f.Lines))
	for i, line := range f.Lines {
		lineNum := i + 1
		if want, ok := editMap[lineNum]; ok {
			resultLines[i] = adjustSpaces(line, want)
		} else {
			resultLines[i] = line
		}
	}
	return bytes.Join(resultLines, newlineSep)
}

// adjustSpaces replaces the spaces between the list marker and item text.
func adjustSpaces(line []byte, wantSpaces int) []byte {
	markerEnd, currentSpaces := parseMarkerAndSpaces(line)
	if markerEnd == 0 {
		return line
	}
	result := make([]byte, 0, len(line)-currentSpaces+wantSpaces)
	result = append(result, line[:markerEnd]...)
	for k := 0; k < wantSpaces; k++ {
		result = append(result, ' ')
	}
	result = append(result, line[markerEnd+currentSpaces:]...)
	return result
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "ul-single", "ul-multi", "ol-single", "ol-multi":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf(
					"list-marker-space: %s must be an integer, got %T", k, v,
				)
			}
			if n < 1 {
				return fmt.Errorf(
					"list-marker-space: %s must be >= 1, got %d", k, n,
				)
			}
			switch k {
			case "ul-single":
				r.ULSingle = n
			case "ul-multi":
				r.ULMulti = n
			case "ol-single":
				r.OLSingle = n
			case "ol-multi":
				r.OLMulti = n
			}
		default:
			return fmt.Errorf("list-marker-space: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"ul-single": 1,
		"ul-multi":  1,
		"ol-single": 1,
		"ol-multi":  1,
	}
}

func pluralSpace(n int) string {
	if n == 1 {
		return "space"
	}
	return "spaces"
}

// newlineSep is the bytes.Join separator; a package-level var avoids
// a heap allocation for []byte("\n") on every Fix call.
var newlineSep = []byte("\n")

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.FixableRule  = (*Rule)(nil)
	_ rule.NodeChecker  = (*Rule)(nil)
)

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Fix space after list marker" }

// enteringKinds is the static node-kind interest CheckNode declares
// via rule.KindScopedChecker; package-level so EnteringKinds returns
// it without allocating.
var enteringKinds = []ast.NodeKind{ast.KindList}

// EnteringKinds implements rule.KindScopedChecker: CheckNode only
// reacts to these node kinds, entering visits only.
func (r *Rule) EnteringKinds() []ast.NodeKind { return enteringKinds }

var _ rule.KindScopedChecker = (*Rule)(nil)
