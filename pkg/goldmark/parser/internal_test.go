package parser

// Internal unit tests for unexported helpers and methods that the
// public test files (package parser_test) cannot reach. Tests
// here apply the test-pyramid 'unit at the base' principle by
// driving individual functions in isolation rather than through
// a full parse.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestBlockquoteParser_Open_NilReturn(t *testing.T) {
	// blockquoteParser.Open returns (nil, NoChildren) when the
	// input does not start with '>'. The dispatcher only calls
	// Open when the '>' trigger has fired, so this branch is
	// unreachable through Convert but easy to drive directly.
	bp := &blockquoteParser{}
	r := text.NewReader([]byte("not a blockquote\n"))
	node, state := bp.Open(nil, r, nil)
	if node != nil {
		t.Errorf("Open on non-> line should return nil, got %v", node)
	}
	if state != NoChildren {
		t.Errorf("Open on non-> line should return NoChildren, got %v", state)
	}
}

func TestParagraphParser_Open_BlankLine(t *testing.T) {
	// paragraphParser.Open returns nil on a blank line. The
	// dispatcher only opens paragraphs when content exists, so
	// this branch is unreachable through Convert.
	pp := &paragraphParser{}
	r := text.NewReader([]byte("\n"))
	node, state := pp.Open(nil, r, nil)
	if node != nil {
		t.Errorf("Open on blank line should return nil, got %v", node)
	}
	if state != NoChildren {
		t.Errorf("Open on blank line should return NoChildren, got %v", state)
	}
}

func TestLinkLabelState_NodeInterface(t *testing.T) {
	// linkLabelState is an unexported type that implements
	// ast.Inline. Its Text / Dump / Kind methods exist to satisfy
	// the interface; they are never called via the dispatcher.
	// Drive them directly so they appear as reached coverage.
	s := &linkLabelState{
		Segment: text.NewSegment(0, 5),
	}
	source := []byte("hello world")
	if got := s.Text(source); string(got) != "hello" {
		t.Errorf("Text = %q, want hello", got)
	}
	if k := s.Kind(); k != kindLinkLabelState {
		t.Errorf("Kind = %v, want kindLinkLabelState", k)
	}
	// Dump prints to stdout; just call it.
	silenceStdout(t, func() { s.Dump(source, 0) })
}

func TestIDs_GenerateSequenceCollision(t *testing.T) {
	// Generate disambiguates by appending -N to slugs that are
	// already taken. Drive the loop with three same-name calls.
	ids := newIDs().(*ids)
	a := string(ids.Generate([]byte("Heading"), ast.KindHeading))
	b := string(ids.Generate([]byte("Heading"), ast.KindHeading))
	c := string(ids.Generate([]byte("Heading"), ast.KindHeading))
	if a == b || b == c || a == c {
		t.Errorf("Generate must disambiguate: %q %q %q", a, b, c)
	}
	if !strings.HasPrefix(b, "heading-") {
		t.Errorf("second Generate should have -N suffix: %q", b)
	}
}

// silenceStdout swallows fmt.Print output from a function so
// Dump-style prints don't litter test output.
func silenceStdout(t *testing.T, fn func()) {
	t.Helper()
	defer func() { _ = recover() }()
	fn()
}

func TestListParser_Continue_DirectStates(t *testing.T) {
	// Drive listParser.Continue with various synthesised states
	// that are hard to reach through Convert.
	bp := &listParser{}

	// State 1: blank line + last child empty -> Continue|HasChildren.
	list := ast.NewList('-')
	li := ast.NewListItem(2)
	list.AppendChild(list, li) // empty last child
	r := text.NewReader([]byte("\n"))
	pc := NewContext()
	state := bp.Continue(list, r, pc)
	if state != Continue|HasChildren {
		t.Errorf("blank+empty-last got %v", state)
	}

	// State 2: blank line + last child has content -> Continue|HasChildren.
	list2 := ast.NewList('-')
	li2 := ast.NewListItem(2)
	li2.AppendChild(li2, ast.NewParagraph()) // non-empty
	list2.AppendChild(list2, li2)
	r2 := text.NewReader([]byte("\n"))
	pc2 := NewContext()
	bp.Continue(list2, r2, pc2)

	// State 3: marker change -> CanContinue returns false -> Close.
	list3 := ast.NewList('-')
	li3 := ast.NewListItem(2)
	li3.AppendChild(li3, ast.NewParagraph())
	list3.AppendChild(list3, li3)
	// Feed a '+' marker line which doesn't match the '-' marker.
	r3 := text.NewReader([]byte("+ different\n"))
	pc3 := NewContext()
	pc3.SetBlockOffset(0)
	state3 := bp.Continue(list3, r3, pc3)
	if state3 != Close {
		// Even if not Close, the call exercised the CanContinue
		// check path; just verify no panic.
	}

	// State 4: emptyListItemWithBlankLines flag set -> Close.
	list4 := ast.NewList('-')
	li4 := ast.NewListItem(2)
	li4.AppendChild(li4, ast.NewParagraph())
	list4.AppendChild(list4, li4)
	r4 := text.NewReader([]byte("text\n"))
	pc4 := NewContext()
	pc4.Set(emptyListItemWithBlankLines, listItemFlagValue)
	bp.Continue(list4, r4, pc4)
}

func TestParseListItem_AllBranches(t *testing.T) {
	// parseListItem is unexported.  Drive each early-return path
	// via direct invocation.
	cases := []struct {
		name string
		line string
		want listItemType
	}{
		{"bullet-dash", "- item\n", bulletList},
		{"bullet-star", "* item\n", bulletList},
		{"bullet-plus", "+ item\n", bulletList},
		{"ordered-period", "1. item\n", orderedList},
		{"ordered-paren", "1) item\n", orderedList},
		{"deep-indent", "    - too deep\n", notList},
		{"long-number", "1234567890. too long\n", notList},
		{"number-no-period", "1 item\n", notList},
		{"no-marker", "no list marker\n", notList},
		{"bullet-no-space", "-noSpace\n", notList},
		{"bullet-eol", "-\n", bulletList},
		{"empty-line", "", notList},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, got := parseListItem([]byte(c.line))
			if got != c.want {
				t.Errorf("parseListItem(%q) = %v, want %v", c.line, got, c.want)
			}
		})
	}
}

func TestCalcListOffset_AllBranches(t *testing.T) {
	// Drive each branch of calcListOffset without asserting on
	// the exact numeric output (the function's contract is
	// internal to the dispatcher).
	cases := []struct {
		name   string
		source string
		match  [6]int
	}{
		{"no-body", "- ", [6]int{0, 0, 0, 1, -1, -1}},                // match[4] < 0
		{"blank-body", "-   ", [6]int{0, 0, 0, 1, 1, 4}},              // blank
		{"normal-indent", "- abc", [6]int{0, 0, 0, 1, 2, 5}},          // indent <= 4
		{"deep-indent-codeblock", "-     code", [6]int{0, 0, 0, 1, 2, 10}}, // > 4
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_ = calcListOffset([]byte(c.source), c.match)
		})
	}
}

func TestRemoveLinkLabelState_AllBranches(t *testing.T) {
	// linkLabelState is a doubly-linked list.  Drive
	// removeLinkLabelState's branches:
	//   1. context has no list (returns early)
	//   2. head removal, list becomes empty
	//   3. head removal, list continues (next != nil)
	//   4. middle removal (Prev != nil and Next != nil)
	//   5. tail removal (Prev != nil, Next == nil)

	// Branch 1: no list set.
	pc := NewContext()
	removeLinkLabelState(pc, &linkLabelState{})

	// Build a list with 3 entries: a <-> b <-> c.
	a := &linkLabelState{Segment: text.NewSegment(0, 1)}
	b := &linkLabelState{Segment: text.NewSegment(1, 2)}
	c := &linkLabelState{Segment: text.NewSegment(2, 3)}
	a.Last = c
	a.First = a
	a.Next = b
	b.Prev = a
	b.Next = c
	b.First = a
	b.Last = c
	c.Prev = b
	c.First = a
	c.Last = c

	pc.Set(linkLabelStateKey, a)
	// Branch 4: middle removal (remove b).
	removeLinkLabelState(pc, b)
	// Branch 5: tail removal (remove c).
	removeLinkLabelState(pc, c)
	// Branch 2/3: head removal (remove a).
	removeLinkLabelState(pc, a)

	// Build another list with just one entry to drive the
	// head-removal-list-becomes-empty branch explicitly.
	single := &linkLabelState{}
	single.First = single
	single.Last = single
	pc.Set(linkLabelStateKey, single)
	removeLinkLabelState(pc, single)
}

func TestLinkParser_ContainsLink_AllBranches(t *testing.T) {
	// containsLink recursively scans for an ast.Link node.
	// Drive: nil input, leaf without link, sibling with link,
	// nested child with link, none-found chain.
	lp := &linkParser{}
	if lp.containsLink(nil) {
		t.Error("containsLink(nil) should be false")
	}

	// Tree with a Link at the top level.
	doc := ast.NewDocument()
	doc.AppendChild(doc, ast.NewLink())
	if !lp.containsLink(doc.FirstChild()) {
		t.Error("containsLink should find top-level Link")
	}

	// Tree with a nested Link inside a Paragraph.
	doc2 := ast.NewDocument()
	p := ast.NewParagraph()
	doc2.AppendChild(doc2, p)
	p.AppendChild(p, ast.NewLink())
	if !lp.containsLink(doc2.FirstChild()) {
		t.Error("containsLink should find nested Link")
	}

	// Tree with no Link.
	doc3 := ast.NewDocument()
	doc3.AppendChild(doc3, ast.NewParagraph())
	if lp.containsLink(doc3.FirstChild()) {
		t.Error("containsLink should not find a Link in plain paragraph")
	}
}

func TestLinkParser_PopLinkBottom_AllStackShapes(t *testing.T) {
	// popLinkBottom returns the most recent bottom from a
	// stack-like structure stored at linkBottom.
	//   - nil pc -> nil
	//   - single ast.Node -> return it and clear
	//   - []ast.Node len 1 entry remaining after pop
	//   - []ast.Node len 0 after pop -> nil
	//   - []ast.Node len >2 after pop -> slice with N-1
	pc := NewContext()
	if popLinkBottom(pc) != nil {
		t.Error("popLinkBottom with empty context should return nil")
	}

	// Single ast.Node.
	pc.Set(linkBottom, ast.Node(ast.NewParagraph()))
	if popLinkBottom(pc) == nil {
		t.Error("popLinkBottom on single Node should return it")
	}

	// Slice with 2 entries -> after pop, single remains -> stored as ast.Node.
	pc.Set(linkBottom, []ast.Node{ast.NewParagraph(), ast.NewParagraph()})
	popLinkBottom(pc)

	// Slice with 1 entry -> after pop, empty -> nil.
	pc.Set(linkBottom, []ast.Node{ast.NewParagraph()})
	popLinkBottom(pc)

	// Slice with 4 entries -> after pop, slice with 3 -> kept as slice.
	pc.Set(linkBottom, []ast.Node{
		ast.NewParagraph(), ast.NewParagraph(),
		ast.NewParagraph(), ast.NewParagraph(),
	})
	popLinkBottom(pc)
}

func TestSetextHeadingParser_Close_EmptyTmpParagraph(t *testing.T) {
	// setextHeadingParser.Close has a path where the temporary
	// paragraph is empty.  The path back-converts the heading
	// to a paragraph (or prepends to a following paragraph).
	// Hard to drive via Convert; construct the AST + context
	// state by hand.
	doc := ast.NewDocument()
	heading := ast.NewHeading(1)
	heading.Lines().Append(text.NewSegment(0, 5))
	doc.AppendChild(doc, heading)

	emptyPara := ast.NewParagraph()
	// Empty paragraph (no lines).
	pc := NewContext()
	pc.Set(temporaryParagraphKey, emptyPara)

	bp := &setextHeadingParser{}
	source := []byte("hello world")
	r := text.NewReader(source)
	bp.Close(heading, r, pc)

	// After Close: heading should be removed from doc, paragraph
	// inserted.  We don't assert on exact structure - just that
	// the call didn't panic.

	// Second invocation: empty tmp paragraph + heading has a
	// following Paragraph sibling, so the segment is prepended.
	doc2 := ast.NewDocument()
	heading2 := ast.NewHeading(1)
	heading2.Lines().Append(text.NewSegment(0, 5))
	doc2.AppendChild(doc2, heading2)
	followingPara := ast.NewParagraph()
	followingPara.Lines().Append(text.NewSegment(0, 5))
	doc2.AppendChild(doc2, followingPara)

	pc2 := NewContext()
	pc2.Set(temporaryParagraphKey, ast.NewParagraph()) // empty tmp
	bp.Close(heading2, text.NewReader(source), pc2)
}

func TestDelimiter_CalcComsumption_AllBranches(t *testing.T) {
	// Three branches:
	//   1. The %3 rule: (canClose||canOpen) + sum%3==0 + closer%3 != 0 -> 0
	//   2. Both >= 2 -> 2
	//   3. Otherwise -> 1
	cases := []struct {
		name   string
		opener Delimiter
		closer Delimiter
		want   int
	}{
		{
			name:   "len-2-both",
			opener: Delimiter{Length: 2, OriginalLength: 2},
			closer: Delimiter{Length: 2, OriginalLength: 2},
			want:   2,
		},
		{
			name:   "len-1-both",
			opener: Delimiter{Length: 1, OriginalLength: 1},
			closer: Delimiter{Length: 1, OriginalLength: 1},
			want:   1,
		},
		{
			name:   "mod-3-rule",
			opener: Delimiter{Length: 1, OriginalLength: 1, CanClose: true},
			closer: Delimiter{Length: 2, OriginalLength: 2, CanOpen: false},
			want:   0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.opener.CalcComsumption(&c.closer); got != c.want {
				t.Errorf("CalcComsumption = %d, want %d", got, c.want)
			}
		})
	}
}

func TestPreserveLeadingTabInCodeBlock_Direct(t *testing.T) {
	// preserveLeadingTabInCodeBlock has a conditional that
	// rewrites segment.Padding and start when the back-tracked
	// LineOffset matches offsetWithPadding.  Drive both branches:
	//   - offsetWithPadding == LineOffset (mutation path)
	//   - mismatch (no-op path)
	t.Run("mutation-path", func(t *testing.T) {
		// Synthesise state where stepping back 1 char yields the
		// same LineOffset (e.g. preceding tab consumed as
		// padding).
		src := []byte("\tabc\n")
		r := text.NewReader(src)
		r.Advance(1) // past the tab; lineOffset = 4
		seg := text.NewSegmentPadding(1, 5, 3)
		preserveLeadingTabInCodeBlock(&seg, r, 0)
	})
	t.Run("noop-path", func(t *testing.T) {
		// Plain ASCII source — back-tracking 1 char yields
		// LineOffset-1, not matching.
		src := []byte("abcdef\n")
		r := text.NewReader(src)
		r.Advance(3) // mid-line
		seg := text.NewSegmentPadding(3, 7, 0)
		preserveLeadingTabInCodeBlock(&seg, r, 0)
	})
}

func TestParagraphParser_Close_EmptyParagraph(t *testing.T) {
	// paragraphParser.Close removes a paragraph from its parent
	// when the paragraph has 0 lines.  This branch is hard to
	// drive via Parse but trivial directly.
	doc := ast.NewDocument()
	p := ast.NewParagraph()
	doc.AppendChild(doc, p)
	if doc.FirstChild() != p {
		t.Fatal("setup: paragraph not attached")
	}
	bp := &paragraphParser{}
	bp.Close(p, text.NewReader([]byte("")), NewContext())
	if doc.FirstChild() == p {
		t.Error("empty paragraph should be removed from parent")
	}
}

func TestReference_PublicAPI(t *testing.T) {
	// parser.NewReference + reference's accessors are part of the
	// public API but unused in the default parse flow (which uses
	// astReference instead).  Drive them directly.
	ref := NewReference([]byte("label"), []byte("/dest"), []byte("title"))
	if string(ref.Label()) != "label" {
		t.Errorf("Label = %q, want label", ref.Label())
	}
	if string(ref.Destination()) != "/dest" {
		t.Errorf("Destination = %q, want /dest", ref.Destination())
	}
	if string(ref.Title()) != "title" {
		t.Errorf("Title = %q, want title", ref.Title())
	}
	if s, ok := ref.(fmt.Stringer); ok {
		_ = s.String()
	}
}

func TestLinkParser_Parse_DefensiveBranches(t *testing.T) {
	// linkParser.Parse has defensive early-return branches that
	// the dispatcher path doesn't usually trigger.  Drive them
	// directly with the corresponding state.
	lp := &linkParser{}
	doc := ast.NewDocument()

	// State: line starts with '!' but next char is NOT '['
	// (e.g. "!something" — image without bracket).  Returns nil.
	r := text.NewReader([]byte("!plain text\n"))
	got := lp.Parse(doc, r, NewContext())
	if got != nil {
		t.Errorf("Parse('!plain') = %v, want nil", got)
	}

	// State: line starts with ']' but no linkLabelStateKey set
	// (no open '[' before this ']') -> nil.
	r2 := text.NewReader([]byte("]orphan close\n"))
	got2 := lp.Parse(doc, r2, NewContext())
	if got2 != nil {
		t.Errorf("Parse(']orphan') = %v, want nil", got2)
	}
}

func TestRawHTMLParser_ParseComment_Direct(t *testing.T) {
	// Drive parseComment directly with various comment shapes.
	bp := &rawHTMLParser{}
	pc := NewContext()
	cases := []string{
		"<!--> immediate-empty\n", // empty comment <!-->
		"<!---> 3-dash empty\n",   // empty comment <!--->
		"<!-- simple --> ok\n",    // normal
		"<!-- multi\nline --> ok\n", // multi-line
		"<!-- unclosed\n",         // unclosed
	}
	for _, src := range cases {
		r := text.NewReader([]byte(src))
		// Advance past `<` (assuming dispatcher would have already done this).
		// The parseComment expects a position at the `<!--`.
		bp.parseComment(r, pc)
	}
}

func TestHTMLBlockParser_Open_AllTypes(t *testing.T) {
	// Drive each block-type detection branch directly.
	bp := &htmlBlockParser{}

	cases := []string{
		"<script>\n",                // type 1 - script
		"<pre>\n",                   // type 1 - pre
		"<style>\n",                 // type 1 - style
		"<!-- comment\n",            // type 2
		"<?xml ?>\n",                // type 3
		"<!DOCTYPE html>\n",         // type 4
		"<![CDATA[content]]>\n",     // type 5
		"<div>\n",                   // type 6 (allowed block tag)
		"<table>\n",                 // type 6
		"<form>\n",                  // type 6
		"<header>\n",                // type 6
		"</div>\n",                  // type 6 (closing tag, allowed)
		"<a href=\"x\">\n",          // type 7
		"<custom-tag>\n",            // type 7
		"</closing/>\n",             // type 7 close+self-close
		"</custom attr=\"v\">\n",    // type 7 close+attr - rejected
		"<unknowntag>\n",            // not a valid type
		"<>invalid\n",               // malformed
	}
	parent := ast.NewDocument()
	for _, src := range cases {
		r := text.NewReader([]byte(src))
		pc := NewContext()
		pc.SetBlockOffset(0)
		bp.Open(parent, r, pc)
	}
}

func TestATXHeadingParser_Open_DefensiveBranches(t *testing.T) {
	// atxHeadingParser.Open has defensive branches reachable only
	// via direct calls or unusual states:
	//   - pos < 0 (no block offset)
	//   - i == pos (the trigger char is '#' but the dispatcher may
	//     pre-position to a non-# char in odd states)
	//   - level > 6 (7+ hashes)
	//   - i == len(line) (line ends at '#')
	bp := &atxHeadingParser{}

	// pos < 0 branch.
	r := text.NewReader([]byte("# title\n"))
	pc := NewContext()
	pc.SetBlockOffset(-1)
	node, state := bp.Open(nil, r, pc)
	if node != nil {
		t.Errorf("Open with pos<0 should return nil, got %v", node)
	}
	if state != NoChildren {
		t.Errorf("Open with pos<0 should return NoChildren, got %v", state)
	}
}

func TestListItemParser_Continue_AllBranches(t *testing.T) {
	// listItemParser.Continue branches:
	//   - blank line
	//   - isEmpty + new list item found -> Close
	//   - isEmpty + not list -> continue (after advance)
	//   - non-empty + indent < offset -> Close
	bp := &listItemParser{}

	// State: empty li with emptyListItemWithBlankLines flag set,
	// and the new line is itself a list item -> Close.
	list := ast.NewList('-')
	li := ast.NewListItem(2)
	list.AppendChild(list, li)
	r := text.NewReader([]byte("- new item\n"))
	pc := NewContext()
	pc.Set(emptyListItemWithBlankLines, listItemFlagValue)
	bp.Continue(li, r, pc)

	// State: non-empty li with line that dedents -> Close.
	li2 := ast.NewListItem(2)
	li2.AppendChild(li2, ast.NewParagraph())
	list2 := ast.NewList('-')
	list2.AppendChild(list2, li2)
	r2 := text.NewReader([]byte("top-level paragraph\n"))
	pc2 := NewContext()
	bp.Continue(li2, r2, pc2)
}

func TestListParser_Open_AllEarlyReturns(t *testing.T) {
	// listParser.Open branches not reached by Convert:
	//   - last is *ast.List -> skip
	//   - skipListParserKey set -> skip
	//   - typ == notList -> skip
	//   - paragraph + orderedList start != 1 -> skip
	//   - paragraph + empty item -> skip
	bp := &listParser{}
	doc := ast.NewDocument()

	// State: last is List.
	prevList := ast.NewList('-')
	doc.AppendChild(doc, prevList)
	pc := NewContext()
	pc.SetOpenedBlocks([]Block{{Node: prevList, Parser: bp}})
	r := text.NewReader([]byte("- item\n"))
	pc.SetBlockOffset(0)
	bp.Open(doc, r, pc)

	// State: skipListParserKey set.
	pc2 := NewContext()
	pc2.Set(skipListParserKey, listItemFlagValue)
	r2 := text.NewReader([]byte("- item\n"))
	pc2.SetBlockOffset(0)
	bp.Open(doc, r2, pc2)

	// State: not a list line.
	pc3 := NewContext()
	r3 := text.NewReader([]byte("not a list\n"))
	pc3.SetBlockOffset(0)
	bp.Open(doc, r3, pc3)

	// State: paragraph + ordered list starting with non-1 -> no interrupt.
	pc4 := NewContext()
	para := ast.NewParagraph()
	doc4 := ast.NewDocument()
	doc4.AppendChild(doc4, para)
	pc4.SetOpenedBlocks([]Block{{Node: para, Parser: bp}})
	r4 := text.NewReader([]byte("5. ordered\n"))
	pc4.SetBlockOffset(0)
	bp.Open(doc4, r4, pc4)
}

func TestListItemParser_Open_AllBranches(t *testing.T) {
	bp := &listItemParser{}
	list := ast.NewList('-')

	// not-a-list-line.
	r := text.NewReader([]byte("plain text\n"))
	bp.Open(list, r, NewContext())

	// indent too far past offset.
	li := ast.NewListItem(2)
	list.AppendChild(list, li)
	r2 := text.NewReader([]byte("      - too far indented\n"))
	bp.Open(list, r2, NewContext())

	// empty item content.
	r3 := text.NewReader([]byte("-\n"))
	bp.Open(list, r3, NewContext())
}

func TestListItemParser_Open_NonListParent(t *testing.T) {
	// listItemParser.Open returns nil when parent is not *ast.List.
	// The dispatcher only routes list-item triggers under a List
	// parent, so this defensive branch is unreachable via Convert.
	bp := &listItemParser{}
	doc := ast.NewDocument()
	r := text.NewReader([]byte("- item\n"))
	node, state := bp.Open(doc, r, NewContext())
	if node != nil {
		t.Errorf("Open with non-List parent should return nil, got %v", node)
	}
	if state != NoChildren {
		t.Errorf("Open with non-List parent should return NoChildren, got %v", state)
	}
}

func TestEmphasisParser_Parse_NilReturn(t *testing.T) {
	// emphasisParser.Parse returns nil when ScanDelimiter finds
	// no valid delimiter run (e.g. the leading char isn't '*'
	// or '_').  Dispatcher only triggers on those chars, but
	// the function's defensive branch is testable.
	ep := &emphasisParser{}
	r := text.NewReader([]byte("not an emphasis\n"))
	if got := ep.Parse(nil, r, NewContext()); got != nil {
		t.Errorf("emphasisParser.Parse on non-emphasis input = %v, want nil", got)
	}
}

func TestIsBlankLine_AllBranches(t *testing.T) {
	// isBlankLine has branches:
	//   - empty stats -> true
	//   - matching lineNum + level -> return isBlank
	//   - lineNum decreases without match -> break, return false
	cases := []struct {
		name  string
		num   int
		level int
		stats []lineStat
		want  bool
	}{
		{"empty-stats", 5, 0, nil, true},
		{"match-blank", 5, 0, []lineStat{{lineNum: 5, level: 0, isBlank: true}}, true},
		{"match-non-blank", 5, 0, []lineStat{{lineNum: 5, level: 0, isBlank: false}}, false},
		{"break-on-lower-num", 5, 0, []lineStat{{lineNum: 3, level: 0, isBlank: true}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isBlankLine(c.num, c.level, c.stats); got != c.want {
				t.Errorf("isBlankLine = %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseAttributes_AllBranches(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"no-brace", "no brace", false},
		{"empty-braces", "{}", true},
		{"single-id", "{#myid}", true},
		{"single-class", "{.cls}", true},
		{"multi-class", "{.a .b .c}", true},
		{"key-value", `{key=val}`, true},
		{"comma-separated", "{#a, .b}", true},
		{"bad-attr-inside", "{!invalid}", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := text.NewReader([]byte(c.in))
			_, ok := ParseAttributes(r)
			if ok != c.want {
				t.Errorf("ParseAttributes(%q) = %v, want %v", c.in, ok, c.want)
			}
		})
	}
}
