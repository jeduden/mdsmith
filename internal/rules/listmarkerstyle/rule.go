// Package listmarkerstyle implements MDS045, which pins the bullet
// character for unordered lists. CommonMark accepts `-`, `*`, and `+`
// interchangeably; this rule requires a single marker (or a rotation by
// depth) to reduce diff noise and aid visual scanning.
package listmarkerstyle

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/listscan"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// Style values for the rule's `style` setting.
const (
	StyleDash     = "dash"
	StyleAsterisk = "asterisk"
	StylePlus     = "plus"
)

func init() {
	rule.Register(&Rule{Style: StyleDash})
}

// Rule pins the marker character for unordered lists.
type Rule struct {
	Style  string
	Nested []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS045" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "list-marker-style" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "list" }

// EnabledByDefault implements rule.Defaultable. The rule is opt-in:
// users pick a project convention and turn the rule on.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule. The per-list logic reads only each item's
// marker byte from its source line and the list's nesting depth — never
// the inline tree — so on a parsed File it folds into the engine's shared
// AST walk (rule.WalkNodes), and on a parse-skipped File (f.AST nil) it
// re-derives the same lists and item lines from f.Lines via listscan
// (checkLayer0). Both resolve the same per-item verdict, so the
// diagnostics are identical.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		return r.checkLayer0(f)
	}
	return rule.WalkNodes(r, f)
}

// checkLayer0 is the nil-AST counterpart of CheckNode. listscan groups
// the same unordered lists goldmark would, with the same depth and item
// marker lines, so each item's marker verdict is byte-identical.
func (r *Rule) checkLayer0(f *lint.File) []lint.Diagnostic {
	lists, _ := listscan.Parse(f.Lines)
	var diags []lint.Diagnostic
	for _, l := range lists {
		if l.Ordered {
			continue
		}
		for _, it := range l.Items {
			if d, ok := r.itemVerdict(f, l.Depth, it.Line); ok {
				diags = append(diags, d)
			}
		}
	}
	return diags
}

// LinesCapable implements rule.LinesChecker: the rule's Check serves the
// nil-AST (parse-skip) path itself by re-deriving list structure from
// f.Lines via listscan, so the engine routes it to Check on a skipped File
// instead of dropping it. Always true.
func (r *Rule) LinesCapable() bool { return true }

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	list, ok := n.(*ast.List)
	if !ok || list.IsOrdered() {
		return nil
	}
	return r.checkList(f, list)
}

// checkList emits diagnostics when any list item's marker does not match
// the expected marker for the list's depth. Returns one diagnostic per
// mismatching item. Per-item checking is used because Goldmark's
// list.Marker may not reflect the actual bytes on each source line.
func (r *Rule) checkList(f *lint.File, list *ast.List) []lint.Diagnostic {
	depth := r.computeDepth(list)
	var diags []lint.Diagnostic
	for c := list.FirstChild(); c != nil; c = c.NextSibling() {
		item := c.(*ast.ListItem)
		line := r.firstLineOfListItem(f, item)
		if line <= 0 {
			continue
		}
		if d, ok := r.itemVerdict(f, depth, line); ok {
			diags = append(diags, d)
		}
	}
	return diags
}

// itemVerdict is the shared per-item check both the AST (checkList via
// CheckNode) and Layer 0 (checkLayer0) paths drive: depth is the
// containing list's nesting depth and line the item's 1-based marker
// line. It reads the actual marker byte from the source line, so the two
// paths agree by construction.
func (r *Rule) itemVerdict(f *lint.File, depth, line int) (lint.Diagnostic, bool) {
	if line <= 0 {
		return lint.Diagnostic{}, false
	}
	expected := r.expectedMarker(depth)
	actual := r.markerOnLine(f, line)
	if actual == 0 || actual == expected {
		return lint.Diagnostic{}, false
	}
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  r.formatMessage(actual, expected, depth),
	}, true
}

// markerOnLine reads the actual list marker byte from the given 1-based
// source line by skipping leading whitespace and returning the first
// character that is -, *, or +. Returns 0 if the line is out of range
// or no marker is found.
func (r *Rule) markerOnLine(f *lint.File, line int) byte {
	idx := line - 1
	if idx < 0 || idx >= len(f.Lines) {
		return 0
	}
	for _, b := range f.Lines[idx] {
		if b == ' ' || b == '\t' {
			continue
		}
		if b == '-' || b == '*' || b == '+' {
			return b
		}
		break
	}
	return 0
}

// computeDepth counts the number of *ast.List ancestors of the given node.
func (r *Rule) computeDepth(n ast.Node) int {
	depth := 0
	for p := n.Parent(); p != nil; p = p.Parent() {
		if _, ok := p.(*ast.List); ok {
			depth++
		}
	}
	return depth
}

// expectedMarker returns the marker byte that should be used at the
// given depth according to the rule's configuration.
func (r *Rule) expectedMarker(depth int) byte {
	if len(r.Nested) == 0 {
		return styleToMarker(r.Style)
	}
	idx := depth % len(r.Nested)
	return styleToMarker(r.Nested[idx])
}

// styleToMarker converts a style string to its marker byte.
func styleToMarker(style string) byte {
	switch style {
	case StyleDash:
		return '-'
	case StyleAsterisk:
		return '*'
	case StylePlus:
		return '+'
	default:
		return '-'
	}
}

// markerToStyle converts a marker byte to its style string.
func markerToStyle(marker byte) string {
	switch marker {
	case '-':
		return StyleDash
	case '*':
		return StyleAsterisk
	case '+':
		return StylePlus
	default:
		return "unknown"
	}
}

// formatMessage creates the diagnostic message.
func (r *Rule) formatMessage(actual, expected byte, depth int) string {
	if len(r.Nested) > 0 {
		return fmt.Sprintf(
			"unordered list at depth %d uses %s; expected %s",
			depth, markerToStyle(actual), markerToStyle(expected),
		)
	}
	return fmt.Sprintf(
		"unordered list uses %s; configured style is %s",
		markerToStyle(actual), markerToStyle(expected),
	)
}

// firstLineOfListItem returns the 1-based source line of an item's
// marker. When the ListItem carries line segments, the first segment's
// start offset gives the marker line directly. Otherwise the marker
// line is derived from the first block child.
func (r *Rule) firstLineOfListItem(f *lint.File, li *ast.ListItem) int {
	if li.Lines().Len() > 0 {
		seg := li.Lines().At(0)
		return f.LineOfOffset(seg.Start)
	}
	for c := li.FirstChild(); c != nil; c = c.NextSibling() {
		if line := blockFirstLine(f, c); line > 0 {
			return line
		}
	}
	return 0
}

// blockFirstLine returns the first source line of a block node.
// Recurses only through container blocks (whose Lines() is empty).
func blockFirstLine(f *lint.File, n ast.Node) int {
	if n.Lines().Len() > 0 {
		return f.LineOfOffset(n.Lines().At(0).Start)
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if l := blockFirstLine(f, c); l > 0 {
			return l
		}
	}
	return 0
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	// Map of line number to new marker byte
	markerEdits := map[int]byte{}

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		list, ok := n.(*ast.List)
		if !ok || list.IsOrdered() {
			return ast.WalkContinue, nil
		}
		r.collectListEdits(f, list, markerEdits)
		return ast.WalkContinue, nil
	})

	if len(markerEdits) == 0 {
		out := make([]byte, len(f.Source))
		copy(out, f.Source)
		return out
	}

	// Apply edits line by line. replaceMarker allocates a fresh copy when it
	// finds a marker; it returns the input unchanged when no marker is found.
	// Either way joinLines only reads the slices, so f.Lines aliases are safe.
	resultLines := make([][]byte, len(f.Lines))
	for i, line := range f.Lines {
		lineNum := i + 1
		if newMarker, ok := markerEdits[lineNum]; ok {
			resultLines[i] = replaceMarker(line, newMarker)
		} else {
			resultLines[i] = line
		}
	}

	return joinLines(resultLines)
}

// collectListEdits records marker replacements for all items in a list
// whose actual source marker differs from the expected marker.
func (r *Rule) collectListEdits(f *lint.File, list *ast.List, markerEdits map[int]byte) {
	depth := r.computeDepth(list)
	expected := r.expectedMarker(depth)

	// Collect edits for each list item whose source marker is wrong
	for c := list.FirstChild(); c != nil; c = c.NextSibling() {
		item := c.(*ast.ListItem)
		line := r.firstLineOfListItem(f, item)
		if line <= 0 {
			continue
		}
		actual := r.markerOnLine(f, line)
		if actual == 0 || actual == expected {
			continue
		}
		markerEdits[line] = expected
	}
}

// replaceMarker replaces the list marker character in a line.
// The marker is the first non-space character that is -, *, or +.
func replaceMarker(line []byte, newMarker byte) []byte {
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' || line[i] == '\t' {
			continue
		}
		if line[i] == '-' || line[i] == '*' || line[i] == '+' {
			newLine := make([]byte, len(line))
			copy(newLine, line)
			newLine[i] = newMarker
			return newLine
		}
		break
	}
	return line
}

// joinLines joins lines with newline separators.
func joinLines(lines [][]byte) []byte {
	if len(lines) == 0 {
		return nil
	}
	totalLen := 0
	for _, line := range lines {
		totalLen += len(line) + 1 // +1 for newline
	}
	totalLen-- // last line doesn't need newline at end

	result := make([]byte, 0, totalLen)
	for i, line := range lines {
		result = append(result, line...)
		if i < len(lines)-1 {
			result = append(result, '\n')
		}
	}
	return result
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "style":
			str, ok := v.(string)
			if !ok {
				return fmt.Errorf("list-marker-style: style must be a string, got %T", v)
			}
			if !isValidStyle(str) {
				return fmt.Errorf("list-marker-style: invalid style %q (valid: dash, asterisk, plus)", str)
			}
			r.Style = str
		case "nested":
			slice, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf("list-marker-style: nested must be a list of strings, got %T", v)
			}
			for i, str := range slice {
				if !isValidStyle(str) {
					return fmt.Errorf("list-marker-style: invalid nested[%d] style %q", i, str)
				}
			}
			r.Nested = slice
		default:
			return fmt.Errorf("list-marker-style: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"style":  StyleDash,
		"nested": []string{},
	}
}

func isValidStyle(s string) bool {
	return s == StyleDash || s == StyleAsterisk || s == StylePlus
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.FixableRule  = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
	_ rule.NodeChecker  = (*Rule)(nil)
)

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Convert to configured list marker" }

// enteringKinds is the static node-kind interest CheckNode declares
// via rule.KindScopedChecker; package-level so EnteringKinds returns
// it without allocating.
var enteringKinds = []ast.NodeKind{ast.KindList}

// EnteringKinds implements rule.KindScopedChecker: CheckNode only
// reacts to these node kinds, entering visits only.
func (r *Rule) EnteringKinds() []ast.NodeKind { return enteringKinds }

var _ rule.KindScopedChecker = (*Rule)(nil)
