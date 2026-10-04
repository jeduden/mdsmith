package include

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdjustHeadings_ATX(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		parentLevel int
		want        string
	}{
		{
			name:        "shift up: parent=2, source h2 and h3 become h3 and h4",
			content:     "## Title\n\nSome text.\n\n### Sub\n",
			parentLevel: 2,
			want:        "### Title\n\nSome text.\n\n#### Sub\n",
		},
		{
			name:        "no headings in content returns unchanged",
			content:     "Just some text.\n\nAnother paragraph.\n",
			parentLevel: 3,
			want:        "Just some text.\n\nAnother paragraph.\n",
		},
		{
			name:        "parentLevel=0 returns unchanged",
			content:     "## Title\n\n### Sub\n",
			parentLevel: 0,
			want:        "## Title\n\n### Sub\n",
		},
		{
			name:        "cap at level 6",
			content:     "# One\n\n## Two\n\n### Three\n",
			parentLevel: 5,
			want:        "###### One\n\n###### Two\n\n###### Three\n",
		},
		{
			name:        "ATX headings with closing hashes",
			content:     "## Heading ##\n\n### Sub ###\n",
			parentLevel: 2,
			want:        "### Heading ##\n\n#### Sub ###\n",
		},
		{
			name:        "shift=0 returns unchanged: parent=1, source min=h2",
			content:     "## Title\n\n### Sub\n",
			parentLevel: 1,
			want:        "## Title\n\n### Sub\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustHeadings(tt.content, tt.parentLevel)
			assert.Equal(t, tt.want, got, "adjustHeadings() =\n%q\nwant:\n%q", got, tt.want)
		})
	}
}

func TestAdjustHeadings_Setext(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		parentLevel int
		want        string
	}{
		{
			name:        "setext h1 converted to ATX when shifted",
			content:     "Title\n=====\n\nBody text.\n",
			parentLevel: 2,
			want:        "### Title\n\nBody text.\n",
		},
		{
			name:        "setext h2 converted to ATX when shifted",
			content:     "Title\n-----\n\nBody text.\n",
			parentLevel: 2,
			want:        "### Title\n\nBody text.\n",
		},
		{
			name:        "mixed ATX and setext headings",
			content:     "Top\n===\n\n## Sub ATX\n\nAnother\n---\n",
			parentLevel: 2,
			want:        "### Top\n\n#### Sub ATX\n\n#### Another\n",
		},
		{
			name:        "multiple setext h1 and h2",
			content:     "First\n=====\n\nSecond\n-----\n",
			parentLevel: 1,
			want:        "## First\n\n### Second\n",
		},
		{
			name:        "setext h2 is min level, parent=3",
			content:     "Only H2\n-------\n",
			parentLevel: 3,
			want:        "#### Only H2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustHeadings(tt.content, tt.parentLevel)
			assert.Equal(t, tt.want, got, "adjustHeadings() =\n%q\nwant:\n%q", got, tt.want)
		})
	}
}

func TestAdjustHeadings_CodeBlocks(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		parentLevel int
		want        string
	}{
		{
			name:        "code blocks with hash lines not modified",
			content:     "## Real Heading\n\n```bash\n# this is a comment\n## another comment\n```\n",
			parentLevel: 2,
			want:        "### Real Heading\n\n```bash\n# this is a comment\n## another comment\n```\n",
		},
		{
			name:        "setext inside code block not modified",
			content:     "## Heading\n\n```\nFake Title\n=====\n```\n",
			parentLevel: 2,
			want:        "### Heading\n\n```\nFake Title\n=====\n```\n",
		},
		{
			name:        "indented code fence (up to 3 spaces) skipped",
			content:     "## Heading\n\n   ```\n## fake heading\n   ```\n",
			parentLevel: 2,
			want:        "### Heading\n\n   ```\n## fake heading\n   ```\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustHeadings(tt.content, tt.parentLevel)
			assert.Equal(t, tt.want, got, "adjustHeadings() =\n%q\nwant:\n%q", got, tt.want)
		})
	}
}

func TestAdjustHeadingsByOffset(t *testing.T) {
	tests := []struct {
		name    string
		content string
		offset  int
		want    string
	}{
		{
			name:    "positive offset demotes ATX headings",
			content: "# One\n\n## Two\n\nText.\n",
			offset:  1,
			want:    "## One\n\n### Two\n\nText.\n",
		},
		{
			name:    "negative offset promotes ATX headings",
			content: "## Two\n\n### Three\n",
			offset:  -1,
			want:    "# Two\n\n## Three\n",
		},
		{
			name:    "offset 0 returns unchanged",
			content: "## Two\n\n### Three\n",
			offset:  0,
			want:    "## Two\n\n### Three\n",
		},
		{
			name:    "negative offset clamps at level 1",
			content: "# One\n\n## Two\n",
			offset:  -1,
			want:    "# One\n\n# Two\n",
		},
		{
			name:    "positive offset clamps at level 6",
			content: "##### Five\n\n###### Six\n",
			offset:  2,
			want:    "###### Five\n\n###### Six\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustHeadingsByOffset(tt.content, tt.offset)
			assert.Equal(t, tt.want, got,
				"adjustHeadingsByOffset() =\n%q\nwant:\n%q", got, tt.want)
		})
	}
}

func TestAdjustHeadingsByOffset_SetextAndFences(t *testing.T) {
	tests := []struct {
		name    string
		content string
		offset  int
		want    string
	}{
		{
			name:    "setext h1 converted to ATX when shifted",
			content: "Title\n=====\n\nBody.\n",
			offset:  1,
			want:    "## Title\n\nBody.\n",
		},
		{
			name:    "hash lines inside code fence are not shifted",
			content: "# Real\n\n```bash\n# comment\n```\n",
			offset:  1,
			want:    "## Real\n\n```bash\n# comment\n```\n",
		},
		{
			name:    "no headings returns unchanged",
			content: "Just text.\n\nMore text.\n",
			offset:  2,
			want:    "Just text.\n\nMore text.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustHeadingsByOffset(tt.content, tt.offset)
			assert.Equal(t, tt.want, got,
				"adjustHeadingsByOffset() =\n%q\nwant:\n%q", got, tt.want)
		})
	}
}

func TestAdjustHeadingsToLevel(t *testing.T) {
	tests := []struct {
		name    string
		content string
		target  int
		want    string
	}{
		{
			name:    "pin source h1 to level 2",
			content: "# One\n\n## Two\n",
			target:  2,
			want:    "## One\n\n### Two\n",
		},
		{
			name:    "pin deeper source up to level 1 (promote)",
			content: "### Deep\n\n#### Deeper\n",
			target:  1,
			want:    "# Deep\n\n## Deeper\n",
		},
		{
			name:    "target equals min is a no-op",
			content: "## Two\n\n### Three\n",
			target:  2,
			want:    "## Two\n\n### Three\n",
		},
		{
			name:    "clamp at level 6",
			content: "## A\n\n##### B\n",
			target:  6,
			want:    "###### A\n\n###### B\n",
		},
		{
			name:    "no headings returns unchanged",
			content: "Just text.\n\nMore.\n",
			target:  2,
			want:    "Just text.\n\nMore.\n",
		},
		{
			name:    "setext h1 pinned to level 2 becomes ATX",
			content: "Title\n=====\n\nBody.\n",
			target:  2,
			want:    "## Title\n\nBody.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustHeadingsToLevel(tt.content, tt.target)
			assert.Equal(t, tt.want, got,
				"adjustHeadingsToLevel() =\n%q\nwant:\n%q", got, tt.want)
		})
	}
}

// --- headingScan ---

// TestHeadingScan_SetextText pins which previous lines may carry setext
// heading text. CommonMark reads an underline after a blank line, an
// ATX heading, a fence line, an HTML line, a thematic break, indented
// code, or a list item or block quote line as a thematic break (or
// paragraph text), never as a setext heading. A lone "===" is paragraph
// text, so an underline after it does make a heading.
func TestHeadingScan_SetextText(t *testing.T) {
	tests := []struct {
		name string
		prev string
		want bool
	}{
		{"paragraph text", "Title", true},
		{"inline code span", "```x``` is code", true},
		{"inline html", "<span>Title</span>", true},
		{"empty", "", false},
		{"whitespace only", "  \t", false},
		{"carriage return only", "\r", false},
		{"atx heading", "## A", false},
		{"backtick fence", "```", false},
		{"indented fence with info", "  ```go", false},
		{"tilde fence", "~~~", false},
		{"lone = run is paragraph text", "===", true},
		{"indented = run is paragraph text", "  ==", true},
		{"two dashes are paragraph text", "--", true},
		{"setext h2 underline", "---", false},
		{"html comment", "<!-- note -->", false},
		{"processing instruction", "<?toc?>", false},
		{"block tag", "<div>", false},
		{"thematic break", "***", false},
		{"spaced thematic break", "* * *", false},
		{"bullet item", "- item", false},
		{"ordered item", "1. item", false},
		{"ordered item not at 1", "2. item", false},
		{"block quote", "> quote", false},
		{"indented code", "    code", false},
		{"tab-indented code", "\tcode", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var scan headingScan
			scan.step(tt.prev)
			level, text := scan.step("---")
			assert.Equal(t, tt.want, text > 0)
			assert.Equal(t, tt.want, level == 2)
		})
	}
}

// TestHeadingScan_ContainerAndBlockLinesMatchParser checks the scan
// against the canonical parser for lines that are not document-level
// paragraph text: an underline after them is no setext heading, while a
// line that cannot interrupt an open paragraph still continues it.
func TestHeadingScan_ContainerAndBlockLinesMatchParser(t *testing.T) {
	for _, src := range []string{
		"- item\n---\n", "- item\n===\n", "1. item\n---\n", "> quote\n---\n",
		"***\n---\n", "* * *\n---\n", "\n    code\n---\n", "- item\nlazy\n---\n",
		"***\nTitle\n---\n", "* * *\nTitle\n---\n", "- a\n\nTitle\n---\n",
		"para\n2. item\n---\n", "para\n    more\n---\n", "***\n<span>\n---\n",
		"- item\n===\n---\n", "a\nb\n---\n", "===\n---\n", "Title\n   ---\n",
		"Title\n    ---\n", "1. a\n-\n<span>\n# H\n",
	} {
		want := astHasHeading(src)
		got := findMinHeadingLevel(strings.Split(src, "\n")) > 0
		assert.Equal(t, want, got, "%q", src)
	}
}

func TestNextPara(t *testing.T) {
	tests := []struct {
		line string
		prev paraKind
		want paraKind
	}{
		{"text", paraNone, paraRoot},
		{"text", paraRoot, paraRoot},
		{"text", paraContainer, paraContainer},
		{"***", paraRoot, paraNone},
		{"* * *", paraNone, paraNone},
		{"- item", paraNone, paraContainer},
		{"- item", paraRoot, paraContainer},
		{"2. item", paraNone, paraContainer},
		{"2. item", paraRoot, paraRoot},
		{"> quote", paraRoot, paraContainer},
		{"    code", paraNone, paraNone},
		{"\tcode", paraNone, paraNone},
		{"    more", paraRoot, paraRoot},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, nextPara([]byte(tt.line), tt.prev), "%q after %d", tt.line, tt.prev)
	}
}

func TestSetextText(t *testing.T) {
	assert.Equal(t, "  Title", setextText([]string{"  Title"}), "one line is kept as it is")
	assert.Equal(t, "a b c", setextText([]string{" a ", "\tb", "c  "}))
	assert.Equal(t, "a b\r", setextText([]string{"a\r", "b\r"}), "CRLF ending kept")
}

func TestApplyShift_MultiLineSetext(t *testing.T) {
	// A setext heading's text is its whole paragraph; an ATX heading
	// holds one line, so the lines are joined rather than the first
	// left behind as a paragraph.
	got := applyShift([]string{"intro", "First", "second", "---", "", "## B"}, 1)
	assert.Equal(t, []string{"### intro First second", "", "### B"}, got)
}

func TestAdjustHeadings_ListItemThenBreakIsNoHeading(t *testing.T) {
	in := "- item\n---\n\n## A\n"
	assert.Equal(t, "- item\n---\n\n### A\n", adjustHeadings(in, 2))
}

// TestPIStart pins the processing-instruction start rules mirrored from
// the canonical parser.
func TestPIStart(t *testing.T) {
	tests := []struct {
		line           string
		opened, closed bool
	}{
		{"<?toc?>", true, true},
		{"   <?toc ?>  ", true, true},
		{"<?catalog", true, false},
		{"<?catalog\r", true, false},
		{"    <?toc?>", false, false},
		{"<? x", false, false},
		{"<??>", false, false},
		{"<?", false, false},
		{"text", false, false},
	}
	for _, tt := range tests {
		opened, closed := piStart(tt.line)
		assert.Equal(t, tt.opened, opened, "%q opened", tt.line)
		assert.Equal(t, tt.closed, closed, "%q closed", tt.line)
	}
}

func TestSetextLevel(t *testing.T) {
	tests := []struct {
		line string
		want int
	}{
		{"===", 1},
		{"=", 1},
		{"===  \r", 1},
		{"---", 2},
		{"-", 2},
		{"--- \t", 2},
		{"   ---", 2},
		{"   =", 1},
		{"    ---", 0},
		{"", 0},
		{"Title", 0},
		{"=-=", 0},
		{"## A", 0},
		{"- item", 0},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, setextLevel(tt.line), "%q", tt.line)
	}
}

func TestApplyShift_ATXThenThematicBreak(t *testing.T) {
	// "---" after an ATX heading is a thematic break, not a setext
	// underline: the heading is shifted and the break kept.
	got := applyShift([]string{"## A", "---", "text"}, 1)
	assert.Equal(t, []string{"### A", "---", "text"}, got)
}

func TestApplyShift_EmptyATXHeading(t *testing.T) {
	assert.Equal(t, []string{"### "}, applyShift([]string{"##"}, 1))
}

func TestApplyShift_SetextThenThematicBreak(t *testing.T) {
	got := applyShift([]string{"A", "===", "---"}, 1)
	assert.Equal(t, []string{"## A", "---"}, got)
}

func TestFindMinHeadingLevel_ThematicBreakAfterFence(t *testing.T) {
	// "---" after a closing fence is a thematic break; it must not
	// count as a level-2 heading and pull the shift off by one.
	lines := []string{"```", "code", "```", "---", "", "### Real"}
	assert.Equal(t, 3, findMinHeadingLevel(lines))
}

func TestFindMinHeadingLevel_ThematicBreakAfterATX(t *testing.T) {
	assert.Equal(t, 3, findMinHeadingLevel([]string{"### A", "---"}))
}

func TestAdjustHeadings_CRLFFenceCloses(t *testing.T) {
	// A CRLF closing fence must close the block so headings after
	// it are still shifted.
	in := "```\r\ncode\r\n```\r\n\r\n## Real\r\n"
	want := "```\r\ncode\r\n```\r\n\r\n### Real\r\n"
	assert.Equal(t, want, adjustHeadings(in, 2))
}

func TestApplyShift_BacktickInInfoIsNotAFence(t *testing.T) {
	// "```x```" is inline code, not a fence opener, so the heading
	// after it must still be shifted.
	got := applyShift([]string{"```x``` is code.", "", "## Head"}, 1)
	assert.Equal(t, []string{"```x``` is code.", "", "### Head"}, got)
}

// TestAdjustHeadings_SiblingItemFence pins that a fence opened on a
// sibling ordered item ("2. ```" after "1. a") is code: the comment
// inside it keeps its level, and the heading after it shifts.
func TestAdjustHeadings_SiblingItemFence(t *testing.T) {
	src := "1. a\n2. ```\n   # x\n   ```\n# y\n"
	assert.Equal(t, "1. a\n2. ```\n   # x\n   ```\n## y\n", adjustHeadingsByOffset(src, 1))
	assert.True(t, astHasHeading(src))
}
