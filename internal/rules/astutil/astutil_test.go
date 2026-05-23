package astutil

import (
	"bytes"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// --- HeadingLine ---

func TestHeadingLine_SetextHeading(t *testing.T) {
	src := []byte("Title\n=====\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = HeadingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 1, line)
}

func TestHeadingLine_ATXHeading(t *testing.T) {
	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = HeadingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 1, line)
}

func TestHeadingLine_ATXOnLaterLine(t *testing.T) {
	src := []byte("Text\n\n## Heading\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = HeadingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 3, line)
}

func TestHeadingLine_ATXEmphasisOnLaterLine(t *testing.T) {
	// ATX heading on line 3 whose only child is emphasis (not a direct *ast.Text).
	// HeadingLine must descend into inline children to find the text segment.
	src := []byte("Text\n\n## *emph*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = HeadingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 3, line)
}

func TestHeadingLine_ATXLinkOnLaterLine(t *testing.T) {
	// ATX heading on line 3 whose only child is a link node.
	src := []byte("Text\n\n## [link](url)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = HeadingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 3, line)
}

func TestHeadingLine_Fallback_Returns1(t *testing.T) {
	heading := ast.NewHeading(1)
	f, err := lint.NewFile("test.md", []byte("# X\n"))
	require.NoError(t, err)
	assert.Equal(t, 1, HeadingLine(heading, f))
}

// --- ParagraphLine ---

func TestParagraphLine_FirstLine(t *testing.T) {
	src := []byte("Hello world.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if p, ok := n.(*ast.Paragraph); ok {
			line = ParagraphLine(p, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 1, line)
}

func TestParagraphLine_LaterLine(t *testing.T) {
	src := []byte("# Title\n\nParagraph here.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if p, ok := n.(*ast.Paragraph); ok {
			line = ParagraphLine(p, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 3, line)
}

func TestParagraphLine_Fallback_Returns1(t *testing.T) {
	para := ast.NewParagraph()
	f, err := lint.NewFile("test.md", []byte("text\n"))
	require.NoError(t, err)
	assert.Equal(t, 1, ParagraphLine(para, f))
}

// --- IsTable ---

func TestIsTable_TableParagraph(t *testing.T) {
	// goldmark without table extension parses a table as a paragraph
	src := []byte("| A | B |\n| - | - |\n| 1 | 2 |\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var found bool
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if p, ok := n.(*ast.Paragraph); ok {
			found = IsTable(p, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.True(t, found)
}

func TestIsTable_PlainParagraph(t *testing.T) {
	src := []byte("Just text.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var found bool
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if p, ok := n.(*ast.Paragraph); ok {
			found = IsTable(p, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.False(t, found)
}

func TestIsTable_EmptyParagraph_ReturnsFalse(t *testing.T) {
	para := ast.NewParagraph()
	f, err := lint.NewFile("test.md", []byte("text\n"))
	require.NoError(t, err)
	assert.False(t, IsTable(para, f))
}

// --- HeadingText and ExtractText ---

func TestHeadingText_PlainText(t *testing.T) {
	src := []byte("# Hello World\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			text := HeadingText(h, f.Source)
			assert.Equal(t, "Hello World", text)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestHeadingText_NestedEmphasis(t *testing.T) {
	src := []byte("# Hello *world*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			text := HeadingText(h, f.Source)
			assert.Equal(t, "Hello world", text)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestExtractText_DirectTextNode(t *testing.T) {
	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			var buf bytes.Buffer
			for c := h.FirstChild(); c != nil; c = c.NextSibling() {
				ExtractText(c, f.Source, &buf)
			}
			assert.Equal(t, "Title", buf.String())
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// TestHeadingLine_WalkDescendsIntoNonTextChild exercises the ast.Walk path in
// HeadingLine for headings where Lines() is empty (e.g. synthetic nodes).
// The walk must descend through a non-text child (Emphasis) to reach the Text.
func TestHeadingLine_WalkDescendsIntoNonTextChild(t *testing.T) {
	src := []byte("Text\n\n## end\n")
	// "end" starts at byte offset 9 (line 3).
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	heading := ast.NewHeading(2) // no Lines() set
	emph := ast.NewEmphasis(1)
	txt := ast.NewText()
	txt.Segment = text.NewSegment(9, 12)
	emph.AppendChild(emph, txt)
	heading.AppendChild(heading, emph)

	assert.Equal(t, 3, HeadingLine(heading, f))
}

// --- HeadingText and ExtractText additional cases ---

func TestHeadingText_LinkText(t *testing.T) {
	src := []byte("# [mdsmith](https://example.com)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	found := false
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			found = true
			assert.Equal(t, "mdsmith", HeadingText(h, f.Source))
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	require.True(t, found)
}

func TestExtractText_LinkNode(t *testing.T) {
	src := []byte("# [mdsmith](https://example.com)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	found := false
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			link, ok2 := h.FirstChild().(*ast.Link)
			require.True(t, ok2)
			var buf bytes.Buffer
			ExtractText(link, f.Source, &buf)
			assert.Equal(t, "mdsmith", buf.String())
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	require.True(t, found)
}

func TestHeadingText_AndExtractText_NoChildren(t *testing.T) {
	h := ast.NewHeading(1)
	assert.Equal(t, "", HeadingText(h, nil))

	var buf bytes.Buffer
	emptyLink := ast.NewLink()
	ExtractText(emptyLink, nil, &buf)
	assert.Equal(t, "", buf.String())
}

// --- CollectSectionHeadings ---

func TestCollectSectionHeadings_OrdersByLine(t *testing.T) {
	src := []byte("# H1\n\n## H2\n\n### H3\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	got := CollectSectionHeadings(f)
	require.Len(t, got, 3)
	assert.Equal(t, 1, got[0].Level)
	assert.Equal(t, 1, got[0].Line)
	assert.Equal(t, 2, got[1].Level)
	assert.Equal(t, 3, got[1].Line)
	assert.Equal(t, 3, got[2].Level)
	assert.Equal(t, 5, got[2].Line)
}

func TestCollectSectionHeadings_NoHeadings(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("just text\n"))
	require.NoError(t, err)
	assert.Empty(t, CollectSectionHeadings(f))
}

// --- CollectSectionParagraphs ---

func TestCollectSectionParagraphs_SkipsTables(t *testing.T) {
	src := []byte("# H1\n\nfirst.\n\n| a |\n| - |\n| b |\n\nsecond.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	got := CollectSectionParagraphs(f)
	require.Len(t, got, 2)
	// Text is now materialised lazily via ExtractText (plan 196): the
	// collector leaves Node set but Text empty, and callers reach the
	// string through ExtractText(source).
	assert.Equal(t, "first.", got[0].ExtractText(f.Source))
	assert.Equal(t, "second.", got[1].ExtractText(f.Source))
}

// --- SectionEnd ---

func TestSectionEnd_StopsAtSameOrShallowerLevel(t *testing.T) {
	heads := []SectionHeading{
		{Level: 1, Line: 1},
		{Level: 2, Line: 5},  // nested — does not end H1
		{Level: 3, Line: 10}, // nested — does not end H1
		{Level: 1, Line: 20}, // ends H1
	}
	assert.Equal(t, 20, SectionEnd(heads, 0, 100))
	// H2 at index 1 ends at the next heading of <=2: index 3 (H1).
	assert.Equal(t, 20, SectionEnd(heads, 1, 100))
}

func TestSectionEnd_RunsToEOFWhenNoFollowingHeading(t *testing.T) {
	heads := []SectionHeading{{Level: 1, Line: 1}}
	assert.Equal(t, 51, SectionEnd(heads, 0, 50))
}

// --- SectionParagraph.ExtractText ---

// TestSectionParagraph_ExtractText_NodeBacked pins that
// CollectSectionParagraphs leaves Text empty and Node set, and that
// ExtractText materialises the right plain text on demand (plan 196).
// A regression that re-introduces the eager ExtractPlainText call
// would set Text non-empty and the assertion below would catch it.
func TestSectionParagraph_ExtractText_NodeBacked(t *testing.T) {
	src := []byte("# H\n\nHello *world*.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	paras := CollectSectionParagraphs(f)
	require.Len(t, paras, 1)
	assert.Empty(t, paras[0].Text,
		"buildSectionParagraphs must not populate Text — plan 196 "+
			"defers materialisation to ExtractText")
	require.NotNil(t, paras[0].Node, "Node must be set by the collector")
	assert.Equal(t, "Hello world.", paras[0].ExtractText(f.Source))
}

// TestSectionParagraph_ExtractText_PrefersCachedText pins that a
// hand-constructed literal with Text set (and Node nil) still works
// through ExtractText — the Text shortcut keeps existing test
// literals compiling without forcing them to build an AST node.
func TestSectionParagraph_ExtractText_PrefersCachedText(t *testing.T) {
	p := SectionParagraph{Line: 1, Text: "pre-set"}
	assert.Equal(t, "pre-set", p.ExtractText(nil),
		"Text shortcut must win over the Node fallback so tests "+
			"can construct SectionParagraph literals without building "+
			"an AST")
}

// TestSectionParagraph_ExtractText_HasTextHonoursEmptyCache pins
// the reason HasText exists: a paragraph whose extracted plain
// text is legitimately empty (e.g. an image-only paragraph)
// still hits the cache. Without HasText the
// `p.Text != ""` shortcut would miss and fall back to
// ExtractPlainText on every call. With HasText set the empty
// string is returned without touching Node.
func TestSectionParagraph_ExtractText_HasTextHonoursEmptyCache(t *testing.T) {
	// Node is intentionally nil — the cache must fire, so the
	// nil-Node panic path inside ExtractPlainText is never
	// reached.
	p := SectionParagraph{Line: 1, Text: "", HasText: true}
	assert.Equal(t, "", p.ExtractText([]byte("anything")),
		"HasText must cause ExtractText to return the cached empty "+
			"string without descending into the Node branch")
}

// TestSectionParagraph_ExtractText_HasTextWinsOverShortcut pins
// the precedence: when both HasText and the legacy
// non-empty-Text shortcut would apply, HasText is checked first.
// In practice the two branches return the same thing, but the
// ordering is part of the contract for callers that explicitly
// set HasText to signal "this Text is authoritative."
func TestSectionParagraph_ExtractText_HasTextWinsOverShortcut(t *testing.T) {
	p := SectionParagraph{Line: 1, Text: "cached", HasText: true}
	assert.Equal(t, "cached", p.ExtractText(nil),
		"HasText branch returns Text verbatim")
}

// TestSectionParagraph_ExtractText_HasTextSkipsNodeExtraction
// pins that even with a valid Node set, the HasText cache is
// preferred — that is what makes CollectSectionParagraphsWithText
// an effective shared materialisation memo. A regression that
// re-extracts despite the cache would re-run ExtractPlainText
// per heading sweep through SectionBody and undo plan 196's
// per-paragraph-extract bound.
func TestSectionParagraph_ExtractText_HasTextSkipsNodeExtraction(t *testing.T) {
	src := []byte("# H\n\nHello world.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	paras := CollectSectionParagraphs(f)
	require.Len(t, paras, 1)
	// Pre-cache a divergent string to prove ExtractText does NOT
	// fall through to Node extraction when HasText is set.
	p := paras[0]
	p.Text = "from-cache"
	p.HasText = true
	assert.Equal(t, "from-cache", p.ExtractText(f.Source),
		"HasText cache must win even when Node would extract a "+
			"different string")
}

// TestSectionParagraph_ExtractText_AllThreeBranches is a table-
// driven sweep over the dispatch: HasText branch (cache wins
// regardless of Text content), Text-shortcut branch (HasText
// false but Text non-empty), Node fallback (HasText false, Text
// empty, Node present). One test pinning all three keeps the
// dispatch order legible.
func TestSectionParagraph_ExtractText_AllThreeBranches(t *testing.T) {
	src := []byte("# H\n\nbody.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	paras := CollectSectionParagraphs(f)
	require.Len(t, paras, 1)
	node := paras[0].Node

	cases := []struct {
		name string
		p    SectionParagraph
		want string
	}{
		{name: "HasText with non-empty Text returns Text",
			p:    SectionParagraph{Text: "hit", HasText: true, Node: node},
			want: "hit"},
		{name: "HasText with empty Text returns empty",
			p:    SectionParagraph{Text: "", HasText: true, Node: node},
			want: ""},
		{name: "no HasText, non-empty Text returns Text",
			p:    SectionParagraph{Text: "shortcut"},
			want: "shortcut"},
		{name: "no HasText, empty Text falls back to Node",
			p:    SectionParagraph{Node: node},
			want: "body."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.p.ExtractText(f.Source))
		})
	}
}

// --- CollectSectionParagraphsWithText ---

// TestCollectSectionParagraphsWithText_PopulatesText pins the
// contract MDS057/MDS058 rely on: every entry has Text filled in,
// matching what ExtractText would produce, so SectionBody's
// per-heading sweeps hit the cached field instead of re-extracting
// per containing section.
func TestCollectSectionParagraphsWithText_PopulatesText(t *testing.T) {
	src := []byte("# H\n\nFirst paragraph.\n\nSecond *here*.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	paras := CollectSectionParagraphsWithText(f)
	require.Len(t, paras, 2)
	assert.Equal(t, "First paragraph.", paras[0].Text)
	assert.Equal(t, "Second here.", paras[1].Text)
}

// TestCollectSectionParagraphsWithText_Memoized pins that repeated
// calls share the same backing slice — MDS057 and MDS058 both
// enabled should pay the materialisation cost once, not twice.
func TestCollectSectionParagraphsWithText_Memoized(t *testing.T) {
	src := []byte("# H\n\nOne.\n\nTwo.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	p1 := CollectSectionParagraphsWithText(f)
	p2 := CollectSectionParagraphsWithText(f)
	require.Len(t, p1, 2)
	assert.Same(t, &p1[0], &p2[0],
		"repeated calls must return the cached slice")
}

// TestCollectSectionParagraphsWithText_LeavesNodeMemoUntouched pins
// that the with-text variant builds a fresh slice and does NOT mutate
// the bare CollectSectionParagraphs memo. A caller that asks for the
// bare memo after the with-text memo was built must still see Text
// empty on every entry — Text is plan-196's lazy field, not a
// retroactively shared cache.
func TestCollectSectionParagraphsWithText_LeavesNodeMemoUntouched(t *testing.T) {
	src := []byte("# H\n\nOne.\n\nTwo.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	// Populate the with-text memo first.
	_ = CollectSectionParagraphsWithText(f)
	// The bare collector's slice must still carry empty Text.
	bare := CollectSectionParagraphs(f)
	require.Len(t, bare, 2)
	assert.Empty(t, bare[0].Text)
	assert.Empty(t, bare[1].Text)
}

// --- SectionBody ---

func TestSectionBody_JoinsWithSpace(t *testing.T) {
	// Text-only SectionParagraph literals (no Node) exercise
	// ExtractText's cache-hit branch: when the field is pre-populated
	// the AST is not touched and source can be nil.
	paras := []SectionParagraph{
		{Line: 3, Text: "alpha"},
		{Line: 5, Text: "beta"},
		{Line: 50, Text: "gamma"},
	}
	got := SectionBody(paras, nil, 2, 10)
	assert.Equal(t, "alpha beta", got)
}

func TestSectionBody_EmptyWhenNoParagraphsInRange(t *testing.T) {
	paras := []SectionParagraph{{Line: 100, Text: "out"}}
	assert.Equal(t, "", SectionBody(paras, nil, 1, 10))
}

// TestSectionBody_ExtractsFromNodeWhenTextEmpty pins the production
// path: paragraphs returned by CollectSectionParagraphs carry Node
// but no Text, so SectionBody must materialise text via
// ExtractText(source). Cache-only literals are already covered by
// TestSectionBody_JoinsWithSpace; this one exercises the AST
// extraction.
func TestSectionBody_ExtractsFromNodeWhenTextEmpty(t *testing.T) {
	src := []byte("# H\n\nfirst paragraph here.\n\nsecond paragraph too.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	paras := CollectSectionParagraphs(f)
	require.Len(t, paras, 2)
	// Heading is at line 1; first paragraph starts at line 3 in the
	// source, second at line 5. Range [2, 100) captures both.
	got := SectionBody(paras, f.Source, 2, 100)
	assert.Equal(t, "first paragraph here. second paragraph too.", got)
}

// TestCollectSectionParagraphs_MemoizedPerFile pins that the
// AST-walking collector runs once per File and serves a cached result
// thereafter. On prose-heavy corpora (the neutral Rust Book
// benchmark) the paragraph-walking rules — MDS023
// paragraph-readability (default-on) plus MDS024 paragraph-structure,
// MDS057 required-text-patterns, MDS058 required-mentions (opt-in) —
// each walk every paragraph; sharing one memoized walk removes the
// duplicates when more than one is enabled. Reference identity of
// the returned slice proves a later call did not re-walk.
func TestCollectSectionParagraphs_MemoizedPerFile(t *testing.T) {
	src := []byte("# H\n\nFirst paragraph here.\n\nSecond paragraph too.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	p1 := CollectSectionParagraphs(f)
	p2 := CollectSectionParagraphs(f)

	require.Len(t, p1, 2)
	require.Len(t, p2, 2)
	assert.Same(t, &p1[0], &p2[0],
		"repeated calls must return the cached slice, not a fresh walk")

	// A different File computes independently (the memo is per-File,
	// discarded with it — no cross-file or cross-run staleness).
	f2, err := lint.NewFile("other.md", []byte("Different.\n"))
	require.NoError(t, err)
	o := CollectSectionParagraphs(f2)
	require.Len(t, o, 1)
	assert.Equal(t, "Different.", o[0].ExtractText(f2.Source))
}
