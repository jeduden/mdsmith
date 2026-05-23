package parser_test

// List parser edge cases not reached by the simple corpus tests:
// blank-line continuation inside items, thematic-break-vs-list
// collision, setext-bar inside a list paragraph, ordered/unordered
// transitions, and the CanAcceptIndentedLine predicate.

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
)

func TestList_BlankLineContinuation(t *testing.T) {
	// A blank line inside a list item makes it a "loose" list and
	// drives the if-LastChild-empty branch in Continue.
	src := "- one\n\n  more in item one\n\n- two\n"
	root := parseWithDefaults(src)
	listCount := 0
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindList {
			listCount++
		}
		return ast.WalkContinue, nil
	})
	if listCount == 0 {
		t.Error("expected at least one List node")
	}
}

func TestList_EmptyItemFollowedByContent(t *testing.T) {
	// Empty list item with blank lines, then a continuation drives
	// the lastIsEmpty branch in Continue.
	src := "-\n\n  body\n"
	_ = parseWithDefaults(src)
}

func TestList_ThematicBreakInsideList(t *testing.T) {
	// A thematic break right after a list item closes the list
	// (thematic breaks take precedence over list continuation).
	src := "- one\n- two\n\n---\n\nafter\n"
	root := parseWithDefaults(src)
	hasThematic := false
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindThematicBreak {
			hasThematic = true
		}
		return ast.WalkContinue, nil
	})
	if !hasThematic {
		t.Error("expected ThematicBreak after list close")
	}
}

func TestList_SetextBarInListParagraph(t *testing.T) {
	// A `---` line right after a paragraph inside a list item
	// is interpreted as a setext heading bar, not as the start
	// of a thematic break. This drives the isHeading branch in
	// Continue.
	src := "- title\n  ---\n  body\n"
	_ = parseWithDefaults(src)
}

func TestList_OrderedToUnorderedTransition(t *testing.T) {
	// Switching between bullet types closes the first list and
	// opens a new one — drives the CanContinue Close branch.
	src := "1. first\n2. second\n\n- third\n- fourth\n"
	root := parseWithDefaults(src)
	listCount := 0
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindList {
			listCount++
		}
		return ast.WalkContinue, nil
	})
	if listCount != 2 {
		t.Errorf("expected 2 List nodes (ol then ul), got %d", listCount)
	}
}

func TestList_Parser_DirectMethodInvocation(t *testing.T) {
	// CanAcceptIndentedLine on the list parser is a constant-return
	// function not reached during normal Parse. Drive it directly.
	p := parser.NewListParser()
	if p.CanAcceptIndentedLine() {
		t.Error("list parser CanAcceptIndentedLine should be false")
	}
	if !p.CanInterruptParagraph() {
		t.Error("list parser CanInterruptParagraph should be true")
	}
}

func TestList_ThematicBreakInListContinuation(t *testing.T) {
	// listParser.Continue has a thematic-break-precedence branch
	// that fires when a `---` line appears INSIDE the list (vs
	// after the list closes).  When the last opened block is a
	// paragraph (e.g. a list item with paragraph content), the
	// `---` is interpreted as a setext heading bar rather than
	// a thematic break; otherwise Close fires.
	cases := []string{
		"- item with paragraph\n  ---\n  more\n",
		"- one\n---\n- two\n",
		"- one\n  ---\n",
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestList_ListItemContinue_BlankAndEmptyItemPaths(t *testing.T) {
	// listItemParser.Continue branches:
	//   - blank line -> Continue (covered)
	//   - isEmpty + new list item discovered -> Close
	//   - isEmpty + no new item -> Continue
	//   - non-empty + dedent -> Close
	cases := []string{
		"- \n\n- second item\n",        // empty + new item
		"-\n\n  continuation in body\n", // empty + continuation
		"- text\n  more text\n",         // continuation indent
		"- a\n- b\n\nback to root\n",    // close on dedent
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestList_EmptyItemFollowedByDedentedContent(t *testing.T) {
	// Empty list item followed by less-indented content closes the
	// list (lastIsEmpty + indent < offset branch).
	src := "- \nback to root paragraph\n"
	_ = parseWithDefaults(src)

	// Also drive the `!lastIsEmpty -> Close` branch with a similar
	// pattern but a non-empty last item.
	src2 := "- non-empty\nback to root\n"
	_ = parseWithDefaults(src2)
}

func TestList_BlankAfterEmptyItem(t *testing.T) {
	// A blank line after an empty list item triggers
	// emptyListItemWithBlankLines bookkeeping (the line 162
	// branch in listParser.Continue: blank line + LastChild has
	// no children).
	srcs := []string{
		"-\n\n  body\n",
		"-\n\n- next\n",
		"-\n\n",
		"1.\n\n  body\n",
	}
	for _, src := range srcs {
		_ = parseWithDefaults(src)
	}
}

func TestList_InterruptingParagraphRules(t *testing.T) {
	// A paragraph followed by a list marker.  Only bullet lists
	// and ordered lists starting with 1 can interrupt; an
	// ordered list starting with !=1 stays as paragraph text.
	cases := []string{
		"paragraph here\n- bullet interrupts\n",
		"paragraph here\n1. ordered-1 interrupts\n",
		"paragraph here\n3. ordered-3 does NOT interrupt\n",
		"paragraph here\n- \nempty item cannot interrupt\n",
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestList_DeepIndentTreatedAsCode(t *testing.T) {
	// List item with > 4 spaces after the marker triggers
	// calcListOffset's "offseted codeblock" branch (offset > 4
	// is clamped to 1).
	cases := []string{
		"-     5-space indent body\n",         // exactly 5 spaces after marker
		"-          10-space indent body\n",   // 10 spaces
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestList_ParseListItem_NotListBranches(t *testing.T) {
	// Drive each "return ret, notList" branch in parseListItem
	// via inputs that look like list markers but aren't.
	srcs := []string{
		"    - too-deeply-indented bullet\n",   // i > 3 -> not list
		"12345678901. way too long ordered\n", // > 9-digit ordered -> not list
		"5 missing period or paren\n",          // numbers but no . or )
		"abc\n",                                 // no marker at all
		"-no-space-after-marker\n",              // no IndentWidth
	}
	for _, src := range srcs {
		_ = parseWithDefaults(src)
	}
}

func TestList_LooseList_BlankLineBetweenItems(t *testing.T) {
	src := "- a\n\n- b\n\n- c\n"
	root := parseWithDefaults(src)
	var list *ast.List
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if l, ok := n.(*ast.List); ok {
				list = l
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	if list == nil {
		t.Fatal("no List node found")
	}
	if list.IsTight {
		t.Error("blank-line-separated list must be marked loose (IsTight=false)")
	}
}
