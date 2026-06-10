package extract

import (
	"regexp"

	"github.com/yuin/goldmark/ast"
)

// taskMarkerRE matches a leading GFM task-list marker — `[ ]`, `[x]`,
// or `[X]`, optionally followed by trailing whitespace. It is the
// regexp the goldmark task-list extension uses verbatim
// (`pkg/goldmark/extension/tasklist.go`), applied here over the item's
// already-extracted plain text rather than the AST: mdsmith's schema
// content walker parses with CommonMark + Table only (no task-list
// inline parser), so a marker reaches extract as literal text, not a
// TaskCheckBox node. Matching goldmark byte-for-byte keeps tree-mode
// task detection identical to what the renderer would treat as a
// checkbox — including a bare `[x]` and a no-space `[x]done`. The
// capture group is the marker's state character: whitespace means
// unchecked, `x`/`X` checked.
var taskMarkerRE = regexp.MustCompile(`^\[([\sxX])\]\s*`)

// listTree projects a list as a typed recursive item tree (plan 244's
// `projection: tree`). Each item becomes an object carrying its own
// `text`; a GFM task item additionally carries a `checked` bool with
// the marker stripped from `text`; an item that nests a sub-list
// carries recursive `children`. `children` is present only when the
// nested list has items, and `checked` only on task items, so a plain
// leaf item projects as just `{text}`.
func (p *projector) listTree(n ast.Node) []any {
	lst, ok := n.(*ast.List)
	if !ok {
		return nil
	}
	items := make([]any, 0)
	for c := lst.FirstChild(); c != nil; c = c.NextSibling() {
		items = append(items, p.treeItem(c))
	}
	return items
}

// treeItem builds the object for one list item: its own text (task
// marker split off into `checked`), then any nested sub-list as
// `children`.
func (p *projector) treeItem(item ast.Node) map[string]any {
	obj := map[string]any{}
	text := p.itemOwnText(item)
	if marker := taskMarkerRE.FindString(text); marker != "" {
		obj["checked"] = isCheckedMarker(marker)
		text = text[len(marker):]
	}
	obj["text"] = text
	if kids := p.childItems(item); len(kids) > 0 {
		obj["children"] = kids
	}
	return obj
}

// childItems returns the projected items of an item's first nested
// sub-list, or nil when the item nests no list. A list item holds at
// most one direct child List in goldmark's model; the first one wins.
func (p *projector) childItems(item ast.Node) []any {
	for c := item.FirstChild(); c != nil; c = c.NextSibling() {
		if sub, ok := c.(*ast.List); ok {
			return p.listTree(sub)
		}
	}
	return nil
}

// isCheckedMarker reports whether a task marker matched by
// taskMarkerRE is in the checked state. The state character is the
// first byte after the opening bracket; only `x`/`X` mean checked.
func isCheckedMarker(marker string) bool {
	state := marker[1]
	return state == 'x' || state == 'X'
}
