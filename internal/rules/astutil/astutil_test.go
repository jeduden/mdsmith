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
	assert.Equal(t, "first.", got[0].Text)
	assert.Equal(t, "second.", got[1].Text)
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

// --- SectionBody ---

func TestSectionBody_JoinsWithSpace(t *testing.T) {
	paras := []SectionParagraph{
		{Line: 3, Text: "alpha"},
		{Line: 5, Text: "beta"},
		{Line: 50, Text: "gamma"},
	}
	got := SectionBody(paras, 2, 10)
	assert.Equal(t, "alpha beta", got)
}

func TestSectionBody_EmptyWhenNoParagraphsInRange(t *testing.T) {
	paras := []SectionParagraph{{Line: 100, Text: "out"}}
	assert.Equal(t, "", SectionBody(paras, 1, 10))
}

// TestCollectSectionParagraphs_MemoizedPerFile pins that the
// AST-walking, ExtractPlainText-running collector runs once per File
// and serves a cached result thereafter. On prose-heavy corpora
// (the neutral Rust Book benchmark) the two hot default rules —
// MDS024 paragraph-structure and paragraph-readability — both walk
// every paragraph and extract its plain text; sharing one memoized
// walk removes the duplicate. Reference identity of the returned
// slice proves a later call did not re-walk.
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
	assert.Equal(t, "Different.", o[0].Text)
}
