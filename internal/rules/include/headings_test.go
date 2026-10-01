package include

import (
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

// --- setextContentLine ---

// TestSetextContentLine pins which previous lines may carry setext
// heading text. CommonMark reads an underline after a blank line, an
// ATX heading, a fence line, or another underline as a thematic break
// (or paragraph text), never as a setext heading.
func TestSetextContentLine(t *testing.T) {
	tests := []struct {
		name string
		prev string
		want bool
	}{
		{"paragraph text", "Title", true},
		{"inline code span", "```x``` is code", true},
		{"empty", "", false},
		{"whitespace only", "  \t", false},
		{"carriage return only", "\r", false},
		{"atx heading", "## A", false},
		{"backtick fence", "```", false},
		{"indented fence with info", "  ```go", false},
		{"tilde fence", "~~~", false},
		{"setext h1 underline", "===", false},
		{"setext h2 underline", "---", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, setextContentLine(tt.prev))
		})
	}
}

func TestApplyShift_ATXThenThematicBreak(t *testing.T) {
	// "---" after an ATX heading is a thematic break, not a setext
	// underline: the heading is shifted and the break kept.
	got := applyShift([]string{"## A", "---", "text"}, 1)
	assert.Equal(t, []string{"### A", "---", "text"}, got)
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
