package listscan

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// astItem is the set of per-item facts the five rules read from the
// goldmark AST, recovered with the same helper logic the rules use.
type astItem struct {
	line       int
	level      int
	ordered    bool
	number     int
	multiBlock bool
}

// astList mirrors the per-list facts the rules read.
type astList struct {
	ordered   bool
	start     int
	depth     int
	firstLine int
	lastLine  int
	topLevel  bool
	items     []astItem
}

func TestListscanMatchesAST(t *testing.T) {
	for name, src := range snippets(t) {
		t.Run(name, func(t *testing.T) {
			wantLists := astFacts(t, []byte(src))
			gotLists, gotItems := Parse(bytes.Split([]byte(src), []byte("\n")))
			compareLists(t, wantLists, gotLists)
			assertFlatMatchesLists(t, gotLists, gotItems)
		})
	}
}

// compareLists checks that the listscan lists carry the same facts as the
// AST-derived lists.
func compareLists(t *testing.T, want []astList, got []List) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("list count: AST=%d listscan=%d\nAST=%s\nlistscan=%s",
			len(want), len(got), fmtAST(want), fmtScan(got))
	}
	for i := range want {
		w, g := want[i], got[i]
		if w.ordered != g.Ordered || w.start != g.Start || w.depth != g.Depth ||
			w.firstLine != g.FirstLine || w.lastLine != g.LastLine || w.topLevel != g.TopLevel {
			t.Errorf(
				"list %d mismatch:\n AST: ordered=%v start=%d depth=%d first=%d last=%d top=%v"+
					"\n scan: ordered=%v start=%d depth=%d first=%d last=%d top=%v",
				i, w.ordered, w.start, w.depth, w.firstLine, w.lastLine, w.topLevel,
				g.Ordered, g.Start, g.Depth, g.FirstLine, g.LastLine, g.TopLevel)
		}
		if len(w.items) != len(g.Items) {
			t.Fatalf("list %d item count: AST=%d listscan=%d", i, len(w.items), len(g.Items))
		}
		for j := range w.items {
			wi, gi := w.items[j], g.Items[j]
			if wi.line != gi.Line || wi.level != gi.Level || wi.ordered != gi.Ordered ||
				wi.number != gi.Number || wi.multiBlock != gi.MultiBlock {
				t.Errorf(
					"list %d item %d mismatch:\n AST: line=%d level=%d ord=%v num=%d multi=%v"+
						"\n scan: line=%d level=%d ord=%v num=%d multi=%v",
					i, j, wi.line, wi.level, wi.ordered, wi.number, wi.multiBlock,
					gi.Line, gi.Level, gi.Ordered, gi.Number, gi.MultiBlock)
			}
		}
	}
}

// assertFlatMatchesLists checks the flat item slice is the multiset of
// every list's items, so a rule that consumes only the flat slice sees
// the same facts as one that walks the lists.
func assertFlatMatchesLists(t *testing.T, lists []List, flat []Item) {
	t.Helper()
	total := 0
	for _, l := range lists {
		total += len(l.Items)
	}
	want := make([]Item, 0, total)
	for _, l := range lists {
		want = append(want, l.Items...)
	}
	if len(want) != len(flat) {
		t.Fatalf("flat item count %d != lists' item count %d", len(flat), len(want))
	}
	seen := map[Item]int{}
	for _, it := range want {
		seen[it]++
	}
	for _, it := range flat {
		seen[it]--
	}
	for it, n := range seen {
		if n != 0 {
			t.Errorf("flat slice multiset mismatch for %+v (delta %d)", it, n)
		}
	}
}

func fmtAST(ls []astList) string {
	var b bytes.Buffer
	for i, l := range ls {
		fmt.Fprintf(&b, "\n  [%d] ord=%v start=%d depth=%d first=%d last=%d top=%v items=%d",
			i, l.ordered, l.start, l.depth, l.firstLine, l.lastLine, l.topLevel, len(l.items))
		for _, it := range l.items {
			fmt.Fprintf(&b, "\n     item line=%d level=%d num=%d multi=%v", it.line, it.level, it.number, it.multiBlock)
		}
	}
	return b.String()
}

func fmtScan(ls []List) string {
	var b bytes.Buffer
	for i, l := range ls {
		fmt.Fprintf(&b, "\n  [%d] ord=%v start=%d depth=%d first=%d last=%d top=%v items=%d",
			i, l.Ordered, l.Start, l.Depth, l.FirstLine, l.LastLine, l.TopLevel, len(l.Items))
		for _, it := range l.Items {
			fmt.Fprintf(&b, "\n     item line=%d level=%d num=%d multi=%v", it.Line, it.Level, it.Number, it.MultiBlock)
		}
	}
	return b.String()
}

// astFacts parses src with goldmark and recovers the per-list facts in
// document order using the rules' helper logic.
func astFacts(t *testing.T, src []byte) []astList {
	t.Helper()
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	var out []astList
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		list, ok := n.(*ast.List)
		if !ok {
			return ast.WalkContinue, nil
		}
		al := astList{
			ordered:   list.IsOrdered(),
			start:     list.Start,
			depth:     computeListDepth(list),
			firstLine: lineOfNode(f, list),
			lastLine:  lastLineOfNode(f, list),
			topLevel:  !parentIsListItem(list),
		}
		for c := list.FirstChild(); c != nil; c = c.NextSibling() {
			item := c.(*ast.ListItem)
			line := firstLineOfListItem(f, item)
			al.items = append(al.items, astItem{
				line:       line,
				level:      nestingLevel(item),
				ordered:    list.IsOrdered(),
				number:     itemNumber(f, line, list.IsOrdered()),
				multiBlock: isMultiItem(item),
			})
		}
		out = append(out, al)
		return ast.WalkContinue, nil
	})
	return out
}

func parentIsListItem(n ast.Node) bool {
	_, ok := n.Parent().(*ast.ListItem)
	return ok
}

func itemNumber(f *lint.File, line int, ordered bool) int {
	if !ordered || line < 1 || line > len(f.Lines) {
		return 0
	}
	l := f.Lines[line-1]
	i := 0
	for i < len(l) && l[i] == ' ' {
		i++
	}
	start := i
	for i < len(l) && l[i] >= '0' && l[i] <= '9' {
		i++
	}
	if i == start {
		return 0
	}
	n, _ := strconv.Atoi(string(l[start:i]))
	return n
}

// --- helper logic copied from the rule packages ---

func nestingLevel(li *ast.ListItem) int {
	level := 0
	for p := li.Parent(); p != nil; p = p.Parent() {
		if _, ok := p.(*ast.ListItem); ok {
			level++
		}
	}
	return level
}

func computeListDepth(n ast.Node) int {
	depth := 0
	for p := n.Parent(); p != nil; p = p.Parent() {
		if _, ok := p.(*ast.List); ok {
			depth++
		}
	}
	return depth
}

func isMultiItem(item *ast.ListItem) bool {
	count := 0
	for c := item.FirstChild(); c != nil; c = c.NextSibling() {
		count++
	}
	return count > 1
}

func firstLineOfListItem(f *lint.File, li *ast.ListItem) int {
	if li.Lines().Len() > 0 {
		return f.LineOfOffset(li.Lines().At(0).Start)
	}
	for c := li.FirstChild(); c != nil; c = c.NextSibling() {
		if line := blockFirstLine(f, c); line > 0 {
			return line
		}
	}
	return 0
}

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

func isInlineNode(n ast.Node) bool {
	switch n.(type) {
	case *ast.Text, *ast.String, *ast.CodeSpan, *ast.Emphasis,
		*ast.Link, *ast.Image, *ast.AutoLink, *ast.RawHTML:
		return true
	}
	return false
}

func lineOfNode(f *lint.File, n ast.Node) int {
	if t, ok := n.(*ast.Text); ok {
		return f.LineOfOffset(t.Segment.Start)
	}
	if isInlineNode(n) {
		if n.HasChildren() {
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				if line := lineOfNode(f, c); line > 0 {
					return line
				}
			}
		}
		return 0
	}
	if n.Lines().Len() > 0 {
		return f.LineOfOffset(n.Lines().At(0).Start)
	}
	if n.HasChildren() {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if line := lineOfNode(f, c); line > 0 {
				return line
			}
		}
	}
	return 0
}

func lastLineOfNode(f *lint.File, n ast.Node) int {
	if t, ok := n.(*ast.Text); ok {
		return f.LineOfOffset(t.Segment.Stop - 1)
	}
	if isInlineNode(n) {
		if n.HasChildren() {
			for c := n.LastChild(); c != nil; c = c.PreviousSibling() {
				if line := lastLineOfNode(f, c); line > 0 {
					return line
				}
			}
		}
		return 0
	}
	if n.Lines().Len() > 0 {
		return f.LineOfOffset(n.Lines().At(n.Lines().Len() - 1).Start)
	}
	if n.HasChildren() {
		for c := n.LastChild(); c != nil; c = c.PreviousSibling() {
			if line := lastLineOfNode(f, c); line > 0 {
				return line
			}
		}
	}
	return 0
}

// snippets returns the markdown corpus to validate against, keyed by a
// descriptive name. It merges coreSnippets with the bad fixtures.
func snippets(t *testing.T) map[string]string {
	t.Helper()
	m := coreSnippets()
	for k, v := range fixtureSnippets(t) {
		m[k] = v
	}
	return m
}

// coreSnippets is the hand-written corpus: nested, loose, ordered, and
// edge-case inputs covering all CommonMark list constructs the five rules
// exercise.
func coreSnippets() map[string]string {
	return map[string]string{
		"nested-4space":               "- a\n    - nested item\n",
		"three-deep":                  "- one\n  - two\n    - three\n",
		"outer-inner":                 "- Outer\n  - Inner\n  - Another\n- Another outer\n",
		"loose-list":                  "- a\n\n- b\n",
		"ordered-start5":              "5. x\n6. y\n",
		"multiblock-item":             "- First paragraph\n\n  Second paragraph of same item\n",
		"ordered-paren":               "1) one\n2) two\n",
		"mixed-ord-unord":             "- a\n- b\n\n1. c\n2. d\n",
		"nested-ordered":              "1. a\n   1. nested\n   2. nested2\n2. b\n",
		"list-after-head":             "# Title\n\n- a\n- b\n",
		"list-after-para":             "Para text.\n- a\n- b\n",
		"deeply-nested":               "- a\n  - b\n    - c\n      - d\n",
		"sibling-then-deep":           "- a\n  - b\n- c\n  - d\n    - e\n",
		"ordered-then-bul":            "1. a\n2. b\n- c\n- d\n",
		"two-lists-gap":               "- a\n- b\n\ntext\n\n- c\n- d\n",
		"single-item":                 "- only\n",
		"continuation":                "- a\n  more text\n- b\n",
		"nested-loose":                "- a\n  - x\n\n  - y\n- b\n",
		"code-fence-in-item":          "- a\n\n  ```\n  code\n  ```\n\n- b\n",
		"indented-code-item":          "- a\n\n      indented code\n\n- b\n",
		"two-para-item":               "- one\n\n  two\n\n  three\n",
		"para-then-sublist":           "- top\n\n  - sub\n",
		"sublist-then-para":           "- top\n  - sub\n\n  more top text\n",
		"ordered-loose":               "1. a\n\n2. b\n\n3. c\n",
		"list-then-heading":           "- a\n- b\n# H\n- c\n",
		"heading-splits":              "- a\n\n# H\n\n- b\n",
		"three-spaces-mark":           "   - a\n   - b\n",
		"nested-then-outer":           "- a\n  - b\n  - c\n- d\n- e\n",
		"deep-then-shallow":           "- a\n  - b\n    - c\n  - d\n- e\n",
		"ordered-big-num":             "10. a\n11. b\n12. c\n",
		"item-with-blank-tail":        "- a\n\n- b\n\n",
		"mixed-marker-split":          "- a\n* b\n",
		"two-blank-between":           "- a\n\n\n- b\n",
		"nested-ordered-in-unordered": "- a\n  1. x\n  2. y\n- b\n",
		"paren-and-dot-split":         "1. a\n2) b\n",
		"fence-with-markers":          "```\n- not a list\n1. nope\n```\n\n- real\n",
		"ordered-start0":              "0. a\n1. b\n",
		"list-end-fence":              "- a\n\n  ```\n  x\n  ```\n",
		"sub-then-outer-para":         "- a\n  - b\n\n  back to a\n- c\n",
		"two-space-indent-mark":       "  - a\n  - b\n",
		"item-tight-then-loose":       "- a\n- b\n\n- c\n",
		"nested-3-space":              "- a\n   - b\n",
		"nested-2-then-deeper":        "1. a\n   - b\n     - c\n",
		"trailing-text-deep":          "- a\n  - b\n    text under b\n  - c\n",
		"empty-marker-item":           "-\n- a\n",
		"big-ordered-nested":          "1. a\n   10. x\n   11. y\n2. b\n",
		"empty-then-content":          "-\n  text on next line\n- b\n",
		"empty-ordered":               "1.\n2. b\n",
		"all-empty":                   "-\n-\n",
		"fence-inside-nested":         "- a\n  - b\n\n    ```\n    code\n    ```\n",
		"loose-three-items":           "1. a\n\n2. b\n\n3. c\n\ntext\n",
		"nested-then-fence-outer":     "- a\n  - b\n\n  ```\n  c\n  ```\n",
		"para-blank-para-nested":      "- a\n\n  more\n\n  - sub\n",
		"tilde-fence-in-item":         "- a\n\n  ~~~\n  x\n  ~~~\n\n- b\n",
	}
}

// fixtureSnippets reads the bad fixtures of the five rules and returns
// their bodies with the YAML front matter stripped.
func fixtureSnippets(t *testing.T) map[string]string {
	t.Helper()
	dirs := []string{
		"MDS016-list-indent",
		"MDS045-list-marker-style",
		"MDS046-ordered-list-numbering",
		"MDS061-list-marker-space",
		"MDS014-blank-line-around-lists",
	}
	out := map[string]string{}
	for _, d := range dirs {
		base := filepath.Join("..", d, "bad")
		entries, err := os.ReadDir(base)
		if err != nil {
			t.Fatalf("read fixture dir %s: %v", base, err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(base, e.Name()))
			if err != nil {
				t.Fatalf("read fixture %s: %v", e.Name(), err)
			}
			out[d+"/"+e.Name()] = stripFrontMatter(raw)
		}
	}
	return out
}

// stripFrontMatter removes a leading YAML front-matter block delimited by
// lines that are exactly `---`.
func stripFrontMatter(raw []byte) string {
	lines := bytes.Split(raw, []byte("\n"))
	if len(lines) == 0 || !bytes.Equal(bytes.TrimRight(lines[0], "\r"), []byte("---")) {
		return string(raw)
	}
	for i := 1; i < len(lines); i++ {
		if bytes.Equal(bytes.TrimRight(lines[i], "\r"), []byte("---")) {
			return string(bytes.Join(lines[i+1:], []byte("\n")))
		}
	}
	return string(raw)
}
