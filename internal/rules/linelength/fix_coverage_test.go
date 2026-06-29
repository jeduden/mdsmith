package linelength

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
)

func TestFixTitle(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "Reflow long lines", r.FixTitle())
}

func TestFix_ZeroMaxDefaultsTo80(t *testing.T) {
	r := &Rule{Max: 0, Reflow: true}
	long := strings.Repeat("word ", 30) // ~150 chars, one paragraph
	got := fixSource(t, r, strings.TrimSpace(long))
	for _, line := range strings.Split(got, "\n") {
		assert.LessOrEqual(t, len(line), 80, "line exceeds default width: %q", line)
	}
}

func TestFix_MultiLineParagraphShortThenLong(t *testing.T) {
	r := &Rule{Max: 40, Reflow: true}
	// One paragraph whose first physical line is short and second is long.
	src := "short line\n" +
		"this is a much longer continuation line that exceeds the configured width"
	got := fixSource(t, r, src)
	for _, line := range strings.Split(got, "\n") {
		assert.LessOrEqual(t, len(line), 40, "line exceeds width: %q", line)
	}
	// All words are preserved and rejoined.
	assert.Contains(t, got, "short line this is")
}

func TestFix_BlockquoteNotReflowed(t *testing.T) {
	r := &Rule{Max: 40, Reflow: true}
	src := "> this is a long quoted line that exceeds the configured forty character width here\n"
	if got := fixSource(t, r, src); got != src {
		t.Errorf("block quote paragraph must not be reflowed (top-level only):\n%q", got)
	}
}

func TestFix_LooseListParagraphNotReflowed(t *testing.T) {
	r := &Rule{Max: 40, Reflow: true}
	// Blank line between items makes the list loose, so each item wraps a
	// Paragraph node whose parent is the list item, not the document.
	src := "- item one is long enough to exceed the configured forty character width limit now\n\n- two\n"
	if got := fixSource(t, r, src); got != src {
		t.Errorf("loose list item paragraph must not be reflowed:\n%q", got)
	}
}

func TestFix_RawHTMLParagraphSkipped(t *testing.T) {
	r := &Rule{Max: 40, Reflow: true}
	src := "This paragraph has <span>inline html</span> and runs past the forty character width here.\n"
	if got := fixSource(t, r, src); got != src {
		t.Errorf("paragraph with inline raw HTML must be skipped:\n%q", got)
	}
}

// TestReflowParagraph_EmptyParagraph drives the Lines().Len() == 0 guard
// with a hand-built paragraph node (the parser never emits one, but
// astutil treats it as possible, so the guard stays).
func TestReflowParagraph_EmptyParagraph(t *testing.T) {
	r := &Rule{Max: 80, Reflow: true}
	doc := ast.NewDocument()
	p := ast.NewParagraph()
	doc.AppendChild(doc, p)
	f := &lint.File{Source: []byte("x"), Lines: [][]byte{[]byte("x")}}
	_, _, _, reflowed := r.reflowParagraph(f, p, 80, nil)
	assert.False(t, reflowed, "empty paragraph must not reflow")
}

type flaggedLineCase struct {
	name  string
	rule  *Rule
	lines []string
	width int
	want  bool
}

var longURL = "https://example.com/a-very-long-url-path-here"

var flaggedLineCases = []flaggedLineCase{
	{"hard break short-circuits", &Rule{},
		[]string{"a line ending in a hard break\\"}, 10, false},
	{"short line then long line", &Rule{},
		[]string{"short", "this line is clearly longer than the width"}, 20, true},
	{"url-only excluded, no other long line", &Rule{Exclude: []string{"urls"}},
		[]string{longURL}, 20, false},
	{"url-only excluded but later prose flagged", &Rule{Exclude: []string{"urls"}},
		[]string{longURL, "plain prose that is also well over the width here"}, 20, true},
	{"stern skips long line with no space past limit", &Rule{Stern: true},
		[]string{strings.Repeat("a", 30)}, 20, false},
	{"stern flags long line with a space past limit", &Rule{Stern: true},
		[]string{strings.Repeat("a", 22) + " bb"}, 20, true},
	{"plain long line", &Rule{},
		[]string{"this is a plain line that is over the configured width"}, 20, true},
}

func TestParagraphHasFlaggedLine(t *testing.T) {
	for _, c := range flaggedLineCases {
		t.Run(c.name, func(t *testing.T) {
			lines := make([][]byte, len(c.lines))
			for i, l := range c.lines {
				lines[i] = []byte(l)
			}
			f := &lint.File{Lines: lines}
			got := c.rule.paragraphHasFlaggedLine(f, 1, len(lines), c.width)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestTokenizeParagraph_SpanClampedToEnd(t *testing.T) {
	// The code span literal range extends past the paragraph end; the
	// scan must clamp to end rather than read past it.
	src := []byte("a `bcdefg`")
	got := tokenizeParagraph(src, 0, 5, []lint.Range{{Start: 2, End: 10}})
	want := []string{"a", "`bc"}
	assert.Equal(t, want, got)
}
