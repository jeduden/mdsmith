package lint

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keysOf returns the sorted 1-based keys of a line set for assertions.
func keysOf(set map[int]struct{}) []int {
	out := make([]int, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func scan(src string) *Layer0Scan {
	return Layer0(NewFileLines("doc.md", []byte(src)))
}

func TestLayer0_FencedCodeLines(t *testing.T) {
	l0 := scan("# H\n\n```go\nx := 1\n```\n")
	assert.Equal(t, []int{3, 4, 5}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_TildeFencedCodeLines(t *testing.T) {
	l0 := scan("~~~\ncode\n~~~\n")
	assert.Equal(t, []int{1, 2, 3}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_BacktickInInfoStringIsNotAFence(t *testing.T) {
	// A backtick inside the info string disqualifies a backtick fence, so
	// the line is ordinary prose, not code.
	l0 := scan("```go `inline`\n")
	assert.Empty(t, l0.CodeBlockLines)
}

func TestLayer0_UnclosedFenceMarksPhantomClose(t *testing.T) {
	// An unclosed fence with content marks the opening fence, its content,
	// and a phantom closing-fence line after the last content line.
	l0 := scan("```go\nx := 1\n")
	assert.Equal(t, []int{1, 2, 3}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_EmptyUnclosedFenceMarksNothing(t *testing.T) {
	// An info-less, content-less fence has no source position in goldmark,
	// so the projection emits no code lines.
	l0 := scan("```\n")
	assert.Empty(t, l0.CodeBlockLines)
}

func TestLayer0_IndentedCodeAfterBlankIsCode(t *testing.T) {
	l0 := scan("para\n\n    indented code\n\nmore\n")
	assert.Equal(t, []int{3}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_IndentedLineInterruptingParagraphIsNotCode(t *testing.T) {
	// Indented code cannot interrupt a paragraph (lazy continuation).
	l0 := scan("para line\n    not code\n")
	assert.Empty(t, l0.CodeBlockLines)
}

func TestLayer0_IndentedCodeTrailingBlankTrimmed(t *testing.T) {
	// Trailing blank lines are excluded from an indented code block, just
	// as goldmark trims them on close.
	l0 := scan("\n    code\n\nnext\n")
	assert.Equal(t, []int{2}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_SingleLinePI(t *testing.T) {
	l0 := scan("<?toc?>\n")
	assert.Equal(t, []int{1}, keysOf(l0.PIBlockLines))
}

func TestLayer0_MultiLinePI(t *testing.T) {
	l0 := scan("<?catalog\nglob: docs\n?>\n")
	assert.Equal(t, []int{1, 2, 3}, keysOf(l0.PIBlockLines))
}

func TestLayer0_ClosingDirectivePIInterruptsParagraph(t *testing.T) {
	// A paired directive: the body line is prose, the <?/include?> marker
	// is its own single-line PI interrupting the paragraph.
	l0 := scan("<?include\nf: x\n?>\nbody\n<?/include?>\n")
	assert.Equal(t, []int{1, 2, 3, 5}, keysOf(l0.PIBlockLines))
}

func TestLayer0_NamelessPIIsNotPI(t *testing.T) {
	// "<? ?>" has no name; the PI parser rejects it.
	l0 := scan("<? ?>\n")
	assert.Empty(t, l0.PIBlockLines)
}

func TestLayer0_HTMLCommentSuppressesIndentedCode(t *testing.T) {
	// Indented lines inside an HTML comment are not indented code.
	l0 := scan("<!-- comment\n    indented inside comment\n-->\n")
	assert.Empty(t, l0.CodeBlockLines)
}

func TestLayer0_LeadingDelimiterPairIsNotFrontMatter(t *testing.T) {
	// The scan never strips front matter (the engine strips it before
	// building the File). A body that opens with a `---` thematic break and
	// contains a later `---` must still surface the fenced code between
	// them, matching goldmark.
	l0 := scan("---\n```\ncode\n```\n---\n")
	assert.Equal(t, []int{2, 3, 4}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_BlockquotedFencedCodeIsCode(t *testing.T) {
	l0 := scan("> ```\n> code\n> ```\n")
	assert.Equal(t, []int{1, 2, 3}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_NestedBlockquotedFencedCodeIsCode(t *testing.T) {
	// A fence two quote levels deep must still be found — the code-capable
	// guard descends through nested `>` markers.
	l0 := scan("> > ```\n> > code\n> > ```\n")
	assert.Equal(t, []int{1, 2, 3}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_UnclosedBlockquotedFenceMarksPhantomClose(t *testing.T) {
	// An unclosed fence inside a block quote records its phantom
	// closing-fence line on the parent line after the last quote line,
	// matching the AST.
	l0 := scan("> ```\n> code\n")
	assert.Equal(t, []int{1, 2, 3}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_IndentedLazyContinuationAfterQuoteIsNotCode(t *testing.T) {
	// An indented line that lazily continues a block quote paragraph is not
	// an indented code block (goldmark: lazy continuation, not code).
	l0 := scan("> para line\n    lazy continuation\n")
	assert.Empty(t, l0.CodeBlockLines)
}

func TestLayer0_IndentedCodeKeepsInteriorBlank(t *testing.T) {
	// A blank line interior to an indented code block stays code (goldmark
	// trims only trailing blanks).
	l0 := scan("x\n\n    a\n   \n    b\nend\n")
	assert.Equal(t, []int{3, 4, 5}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_BlockSpansClassifyHeadingsAndQuotes(t *testing.T) {
	l0 := scan("# Heading\n\n> a quote\n\n- item\n")
	kinds := map[BlockKind]int{}
	for _, sp := range l0.BlockSpans {
		kinds[sp.Kind]++
	}
	assert.Equal(t, 1, kinds[BlockATXHeading])
	assert.Equal(t, 1, kinds[BlockQuote])
	assert.Equal(t, 1, kinds[BlockList])
}

func TestLayer0_SetextHeadingSpan(t *testing.T) {
	l0 := scan("Title\n=====\n\nbody\n")
	var found bool
	for _, sp := range l0.BlockSpans {
		if sp.Kind == BlockSetextHeading {
			found = true
			assert.Equal(t, 1, sp.Start)
			assert.Equal(t, 2, sp.End)
		}
	}
	assert.True(t, found, "expected a setext heading span")
}

func TestLayer0_ThematicBreakSpan(t *testing.T) {
	l0 := scan("a\n\n---\n\nb\n")
	var found bool
	for _, sp := range l0.BlockSpans {
		if sp.Kind == BlockThematicBreak {
			found = true
		}
	}
	assert.True(t, found, "expected a thematic break span")
}

func TestLayer0_CachedAcrossCalls(t *testing.T) {
	f := NewFileLines("doc.md", []byte("```\ncode\n```\n"))
	a := Layer0(f)
	b := Layer0(f)
	require.Same(t, a, b, "Layer0 must memoize the same scan")
}

func TestLayer0_NestedBlockquoteDepth(t *testing.T) {
	l0 := scan("> > nested\n")
	require.Len(t, l0.BlockSpans, 1)
	assert.Equal(t, BlockQuote, l0.BlockSpans[0].Kind)
	assert.Equal(t, 2, l0.BlockSpans[0].Depth)
}

// spanKinds returns the set of block kinds present in the scan, counted.
func spanKinds(l0 *Layer0Scan) map[BlockKind]int {
	m := map[BlockKind]int{}
	for _, sp := range l0.BlockSpans {
		m[sp.Kind]++
	}
	return m
}

func TestLayer0_HTMLType1RawTextBlock(t *testing.T) {
	// A <script> opener is a type-1 HTML block; its indented interior must
	// not be mistaken for indented code, and it closes on </script>.
	l0 := scan("<script>\n    var x = 1;\n</script>\n\nafter\n")
	assert.Empty(t, l0.CodeBlockLines)
	assert.Equal(t, 1, spanKinds(l0)[BlockHTML])
}

func TestLayer0_HTMLType1ClosesOnOpeningLine(t *testing.T) {
	// A type-1 block whose terminator sits on the opening line is a
	// single-line HTML block.
	l0 := scan("<pre>code</pre>\n\nafter\n")
	var html *BlockSpan
	for i := range l0.BlockSpans {
		if l0.BlockSpans[i].Kind == BlockHTML {
			html = &l0.BlockSpans[i]
		}
	}
	require.NotNil(t, html, "expected an HTML block span")
	assert.Equal(t, html.Start, html.End, "single-line HTML block")
}

func TestLayer0_HTMLType2CommentBlock(t *testing.T) {
	// A multi-line HTML comment is a type-2 block closing on -->.
	l0 := scan("<!-- a\nb -->\n\nafter\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockHTML])
}

func TestLayer0_HTMLType3ProcessingInstruction(t *testing.T) {
	// A `<?` opener with no name is not a PI block, but it is a type-3 HTML
	// block closing on ?>.
	l0 := scan("<? raw\nmore\n?>\n\nafter\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockHTML])
}

func TestLayer0_HTMLType4Declaration(t *testing.T) {
	// `<!X` opens a type-4 declaration block closing on >.
	l0 := scan("<!DOCTYPE\nhtml>\n\nafter\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockHTML])
}

func TestLayer0_HTMLType5CDATA(t *testing.T) {
	// `<![CDATA[` opens a type-5 block closing on ]]>.
	l0 := scan("<![CDATA[\nraw <stuff>\n]]>\n\nafter\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockHTML])
}

func TestLayer0_HTMLType6BlockTagClosesOnBlank(t *testing.T) {
	// A block-level tag like <div> opens a type-6 block that closes before
	// the first blank line.
	l0 := scan("<div>\n    content\n\nafter\n")
	assert.Empty(t, l0.CodeBlockLines, "type-6 interior is not indented code")
	assert.Equal(t, 1, spanKinds(l0)[BlockHTML])
}

func TestLayer0_HTMLType6UnknownTagWithTextIsNotBlock(t *testing.T) {
	// A non-block-level tag with trailing text is neither a type-6 opener
	// (tag not in the allowed set) nor a type-7 opener (the line is not a
	// single standalone tag), so it is ordinary prose.
	l0 := scan("<madeuptag> trailing words here\nstuff\n")
	assert.Zero(t, spanKinds(l0)[BlockHTML])
}

func TestLayer0_HTMLType7StandaloneTag(t *testing.T) {
	// A standalone complete tag that does not interrupt a paragraph opens a
	// type-7 block; it closes before the first blank line.
	l0 := scan("<custom-element>\nbody\n\nafter\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockHTML])
}

func TestLayer0_HTMLType7CannotInterruptParagraph(t *testing.T) {
	// A type-7 opener cannot interrupt a paragraph: under a prose line, the
	// standalone tag stays part of the paragraph, not an HTML block.
	l0 := scan("para text\n<custom-element>\n")
	assert.Zero(t, spanKinds(l0)[BlockHTML])
}

func TestLayer0_HTMLType7RawTextTagIsType1(t *testing.T) {
	// A standalone <script ...> tag is raw-text, so it is classified as a
	// type-1 block (closing on </script>), not type 7. The single-line form
	// here has its terminator on the same line.
	l0 := scan("<script src=\"a.js\"></script>\n\nafter\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockHTML])
}

func TestLayer0_HTMLType6CanInterruptParagraph(t *testing.T) {
	// Type-6 HTML blocks can interrupt a paragraph: a <div> under a prose
	// line breaks the paragraph and opens an HTML block.
	l0 := scan("para text\n<div>\nx\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockHTML])
}

func TestLayer0_OrderedListItem(t *testing.T) {
	l0 := scan("1. first\n2. second\n")
	assert.Equal(t, 2, spanKinds(l0)[BlockList])
}

func TestLayer0_OrderedListParenMarker(t *testing.T) {
	l0 := scan("1) only\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockList])
}

func TestLayer0_ThematicBreakStar(t *testing.T) {
	l0 := scan("a\n\n***\n\nb\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockThematicBreak])
}

func TestLayer0_ThematicBreakUnderscore(t *testing.T) {
	l0 := scan("a\n\n___\n\nb\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockThematicBreak])
}

func TestLayer0_ThematicBreakInterruptsParagraph(t *testing.T) {
	// A `***` directly under a prose line is a thematic break, breaking the
	// paragraph.
	l0 := scan("para\n***\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockThematicBreak])
}

func TestLayer0_StarNotThematicBreakIsList(t *testing.T) {
	// A single `* ` is a bullet list, not a thematic break.
	l0 := scan("* item\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockList])
	assert.Zero(t, spanKinds(l0)[BlockThematicBreak])
}

func TestLayer0_StarRunNotBreakNotListIsParagraph(t *testing.T) {
	// `**bold**` opens with `*` but is neither a thematic break (text after
	// the run) nor a bullet (no following space), so it is a paragraph.
	l0 := scan("**bold**\n")
	require.Len(t, l0.BlockSpans, 1)
	assert.Equal(t, BlockParagraph, l0.BlockSpans[0].Kind)
}

func TestLayer0_UnderscoreRunNotBreakIsParagraph(t *testing.T) {
	// `_emphasis_` opens with `_` but is not a thematic break, so it is a
	// paragraph (the `_` branch of paragraphLeadKind that falls through).
	l0 := scan("_emphasis_\n")
	require.Len(t, l0.BlockSpans, 1)
	assert.Equal(t, BlockParagraph, l0.BlockSpans[0].Kind)
}

func TestLayer0_LazyContinuationInBlockquote(t *testing.T) {
	// A non-marker prose line lazily continues an open block-quote
	// paragraph: the whole run is a single quote span and yields no code.
	l0 := scan("> para line\nlazy continuation line\n")
	assert.Empty(t, l0.CodeBlockLines)
	require.Len(t, l0.BlockSpans, 1)
	assert.Equal(t, BlockQuote, l0.BlockSpans[0].Kind)
	assert.Equal(t, 2, l0.BlockSpans[0].End)
}

func TestLayer0_BlockquoteEndsWhenFenceOpenAndNonMarkerLine(t *testing.T) {
	// A fence open inside a quote forbids lazy continuation: a non-marker
	// line ends the quote (advanceFenceState keeps the fence open until a
	// matching close).
	l0 := scan("> ```\n> code\nplain line\n")
	require.GreaterOrEqual(t, len(l0.BlockSpans), 1)
	assert.Equal(t, BlockQuote, l0.BlockSpans[0].Kind)
	// The quote covers only its two marker lines; the plain line is outside.
	assert.Equal(t, 2, l0.BlockSpans[0].End)
}

func TestLayer0_BlockquoteFenceClosedMidQuote(t *testing.T) {
	// A fence opened and then closed inside a quote re-enables lazy
	// continuation: advanceFenceState clears the open fence on the close.
	l0 := scan("> ```\n> code\n> ```\nlazy after close\n")
	require.GreaterOrEqual(t, len(l0.BlockSpans), 1)
	assert.Equal(t, BlockQuote, l0.BlockSpans[0].Kind)
	assert.Equal(t, []int{1, 2, 3}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_BlockquoteLazyContinuationSuppressedByOpenFence(t *testing.T) {
	// While a fence is open in the quote, a plain non-marker line is not a
	// lazy continuation, so the quote ends. Pairs with isLazyContinuation
	// being suppressed by openFence != nil.
	l0 := scan("> ```go\n> code\nnot continuation\n")
	assert.Equal(t, BlockQuote, l0.BlockSpans[0].Kind)
}

func TestLayer0_NestedQuoteWithIndentedCode(t *testing.T) {
	// lineHasNonFenceCode descends a nested quote whose deeper level holds
	// an indented (4-column) code line.
	l0 := scan(">     deep indented code\n")
	// A >=4-column indent inside the stripped quote body triggers the
	// recursive scan; whether it yields code depends on goldmark, but the
	// scan must not panic and the quote span must be present.
	assert.Equal(t, BlockQuote, l0.BlockSpans[0].Kind)
}

func TestLayer0_LazyContinuationRejectsFenceOpener(t *testing.T) {
	// A fence opener under a quote is not a lazy continuation: it ends the
	// quote (isLazyContinuation returns false for a fence).
	l0 := scan("> para\n```\ncode\n```\n")
	assert.Equal(t, BlockQuote, l0.BlockSpans[0].Kind)
	assert.Equal(t, []int{2, 3, 4}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_LazyContinuationRejectsPIAndHTML(t *testing.T) {
	// A PI opener under a quote ends the quote rather than continuing it.
	l0 := scan("> para\n<?toc?>\n")
	assert.Equal(t, BlockQuote, l0.BlockSpans[0].Kind)
	assert.Equal(t, []int{2}, keysOf(l0.PIBlockLines))
}

func TestLayer0_ATXHeadingLevels(t *testing.T) {
	// Each ATX level 1–6 is a single-line heading; a 7-hash run is not.
	l0 := scan("# h1\n## h2\n### h3\n#### h4\n##### h5\n###### h6\n####### not\n")
	assert.Equal(t, 6, spanKinds(l0)[BlockATXHeading])
}

func TestLayer0_ATXHeadingNoSpaceIsParagraph(t *testing.T) {
	// `#text` with no space after the hash run is not an ATX heading.
	l0 := scan("#notheading\n")
	require.Len(t, l0.BlockSpans, 1)
	assert.Equal(t, BlockParagraph, l0.BlockSpans[0].Kind)
}

func TestLayer0_ATXHeadingIndentedFourIsCode(t *testing.T) {
	// A hash run indented 4+ columns is indented code, not an ATX heading.
	l0 := scan("\n    # not a heading\n")
	assert.Zero(t, spanKinds(l0)[BlockATXHeading])
}

func TestLayer0_SetextHeadingDashUnderline(t *testing.T) {
	// A `---` directly under a paragraph line promotes it to a setext
	// heading (rather than a thematic break).
	l0 := scan("Title\n---\n\nbody\n")
	assert.Equal(t, 1, spanKinds(l0)[BlockSetextHeading])
}

func TestLowerInto_TruncatesLongInput(t *testing.T) {
	// A name longer than the stack buffer is truncated at capacity; the
	// fold still lowercases the bytes that fit.
	var tn tagName
	long := make([]byte, 40)
	for i := range long {
		long[i] = 'A'
	}
	got := tn.lowerInto(long)
	require.Len(t, got, len(tn.buf), "output is capped at buffer capacity")
	for _, c := range got {
		assert.Equal(t, byte('a'), c, "uppercase folded to lowercase")
	}
}

func TestType7TagIsRawText_AllRawTags(t *testing.T) {
	// Each raw-text tag (case-insensitive) is recognised; a non-raw tag is
	// not.
	for _, tag := range []string{"<script>", "<STYLE>", "<pre>", "<TextArea>"} {
		assert.True(t, type7TagIsRawText([]byte(tag)), tag)
	}
	assert.False(t, type7TagIsRawText([]byte("<custom-element>")))
}

func TestType7TagBytes_CloseTagSlash(t *testing.T) {
	// The tag-byte extractor skips a leading close-tag slash and spaces.
	assert.Equal(t, "div", string(type7TagBytes([]byte("</ div>"))))
}

func TestIsOrderedMarker_TooManyDigits(t *testing.T) {
	// More than 9 leading digits is not a valid ordered-list marker.
	assert.False(t, isOrderedMarker([]byte("1234567890. x"), 0))
}

func TestIsOrderedMarker_NoDigits(t *testing.T) {
	assert.False(t, isOrderedMarker([]byte("x. item"), 0))
}

func TestIsOrderedMarker_BareNumberAtEOL(t *testing.T) {
	// A digit run plus delimiter at end of line (no trailing space) is a
	// valid marker.
	assert.True(t, isOrderedMarker([]byte("1."), 0))
}

func TestIsThematicBreak_BlankAfterIndentIsFalse(t *testing.T) {
	// A line that is all indent (indent >= len) is not a thematic break.
	assert.False(t, isThematicBreak([]byte("   ")))
}

func TestIsThematicBreak_TooFewIsFalse(t *testing.T) {
	// Fewer than three markers is not a thematic break.
	assert.False(t, isThematicBreak([]byte("--")))
}

func TestIsThematicBreak_WithSpacesBetween(t *testing.T) {
	// Three markers with interspersed spaces still form a break.
	assert.True(t, isThematicBreak([]byte("- - -")))
}

func TestIndentWidth_TabExpandsToColumnStop(t *testing.T) {
	// A tab advances to the next 4-column stop; a space after it adds one.
	assert.Equal(t, 4, indentWidth([]byte("\tx")))
	assert.Equal(t, 4, indentWidth([]byte("  \tx")), "two spaces then tab fills to col 4")
}

func TestIndentWidth_AllWhitespaceLine(t *testing.T) {
	// A line of only whitespace counts the full width (no non-space byte to
	// stop at).
	assert.Equal(t, 2, indentWidth([]byte("  ")))
}

func TestStripQuoteMarker_NoMarkerReturnsUnchanged(t *testing.T) {
	// A line with no `>` marker (a lazy continuation) is returned verbatim.
	line := []byte("plain text")
	assert.Equal(t, "plain text", string(stripQuoteMarker(line)))
}

func TestStripQuoteMarker_MarkerWithoutSpace(t *testing.T) {
	// A `>` not followed by a space strips just the marker.
	assert.Equal(t, "x", string(stripQuoteMarker([]byte(">x"))))
}

func TestOpeningFence_IndentOnlyLineIsNotFence(t *testing.T) {
	// A line of only spaces (indent >= len) does not open a fence.
	_, ok := openingFence([]byte("   "))
	assert.False(t, ok)
}

func TestClosingFence_IndentOnlyLineIsNotClose(t *testing.T) {
	fi := fenceInfo{char: '`', length: 3}
	// A blank/indent-only line does not close a fence.
	assert.False(t, closingFence([]byte("   "), fi))
	// An over-indented (>=4) line does not close a fence.
	assert.False(t, closingFence([]byte("    ```"), fi))
}

func TestHTMLBlockCloses_EachType(t *testing.T) {
	assert.True(t, htmlBlockCloses([]byte("</script>"), htmlType1))
	assert.True(t, htmlBlockCloses([]byte("x -->"), htmlType2))
	assert.True(t, htmlBlockCloses([]byte("x ?>"), htmlType3))
	assert.True(t, htmlBlockCloses([]byte("x>"), htmlType4))
	assert.True(t, htmlBlockCloses([]byte("x ]]>"), htmlType5))
	// Types 6 and 7 never close on a terminator (the caller handles blanks).
	assert.False(t, htmlBlockCloses([]byte("</div>"), htmlType6))
}

func TestIsOrderedMarker_DigitsThenNonDelimiter(t *testing.T) {
	// A digit run not followed by `.` or `)` is not an ordered marker.
	assert.False(t, isOrderedMarker([]byte("12 item"), 0))
}

func TestIsThematicBreak_NonMarkerLeadIsFalse(t *testing.T) {
	// A line whose first non-space byte is not `-`, `*`, or `_` is not a
	// thematic break.
	assert.False(t, isThematicBreak([]byte("abc")))
}

func TestOpeningFence_IndentOnlyLineReturnsFalse(t *testing.T) {
	// A line that is all spaces (indent >= len(line)) is not a fence opener.
	_, ok := openingFence([]byte("   "))
	assert.False(t, ok)
}

func TestTryFence_InfoFenceImmediateClose(t *testing.T) {
	// An info-string fence closed on the very next line with no content must
	// mark both the opening and closing fence lines (lastContent==0 path).
	l0 := scan("```go\n```\n")
	assert.Equal(t, []int{1, 2}, keysOf(l0.CodeBlockLines))
}

func TestTryPI_UnclosedAtEOF(t *testing.T) {
	// A PI block with no closing ?> runs to the trailing-empty-line guard;
	// all body lines are marked as PI.
	l0 := scan("<?include\nf: x\n")
	assert.Equal(t, []int{1, 2}, keysOf(l0.PIBlockLines))
}

func TestScanParagraph_HTMLInterruptsParagraph(t *testing.T) {
	// An HTML block (types 1–6) interrupts a paragraph without a blank line.
	// The paragraph span ends before the HTML line; the HTML is its own span.
	l0 := scan("para text\n<div>\nmore\n</div>\n")
	kinds := make([]BlockKind, 0, len(l0.BlockSpans))
	for _, sp := range l0.BlockSpans {
		kinds = append(kinds, sp.Kind)
	}
	assert.Contains(t, kinds, BlockParagraph)
	assert.Contains(t, kinds, BlockHTML)
}

func TestOpeningFence_TwoCharRunNotAFence(t *testing.T) {
	// A run of only 2 fence characters (length < 3) is not a fence opener.
	_, ok := openingFence([]byte("``code"))
	assert.False(t, ok)
}

func TestScanParagraph_FenceInterruptsParagraph(t *testing.T) {
	// A fenced code block interrupts an open paragraph without a blank line.
	// The paragraph span ends before the fence; the fence is its own span.
	l0 := scan("para\n```\ncode\n```\n")
	kinds := make([]BlockKind, 0, len(l0.BlockSpans))
	for _, sp := range l0.BlockSpans {
		kinds = append(kinds, sp.Kind)
	}
	assert.Contains(t, kinds, BlockParagraph)
	assert.Contains(t, kinds, BlockFencedCode)
	assert.Equal(t, []int{2, 3, 4}, keysOf(l0.CodeBlockLines))
}

func TestSourceMayHaveCodeBlock(t *testing.T) {
	// Code-free sources: no fence run, tab, or four-space indent.
	for _, src := range []string{
		"# Title\n\nplain prose only\n",
		"- a list\n- with items\n",
		"> a quote\n\ntrailing space \n",
		"",
		"one  two   three\n",        // up to 3 spaces is not a four-space run
		"text with `inline` span\n", // a single-backtick span is not a code block
	} {
		assert.False(t, SourceMayHaveCodeBlock([]byte(src)),
			"expected code-free: %q", src)
	}
	// Sources that may hold a code block — each trips a distinct marker.
	for _, src := range []string{
		"```\ncode\n```\n",     // backtick fence
		"~~~\ncode\n~~~\n",     // tilde fence
		"a\tb\n",               // tab
		"    indented code\n",  // four-space indent
		"- ```\n  c\n  ```\n",  // fence on a list-marker line (the divergence class)
		"-     deep in item\n", // five spaces after a marker → indented code
	} {
		assert.True(t, SourceMayHaveCodeBlock([]byte(src)),
			"expected may-have-code: %q", src)
	}
}
