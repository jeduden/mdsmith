package include

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/markdown"
	"github.com/stretchr/testify/assert"
)

// indentCases are snippets whose headings depend on indentation: ATX
// headings indented up to three spaces, fence-like lines indented four
// or more columns (indented code, or continuation text after a
// paragraph), and fences opened on a list-marker line.
var indentCases = []string{
	"   # Indented\n",
	"  ## Two\n\n# One\n",
	"    # code\n",
	"\t# code\n",
	"para\n    ```\n# Heading\n",
	"para\n\t```\n# Heading\n",
	"\n    ```\n\n# Heading\n",
	"   ```\n# in fence\n   ```\n# out\n",
	"- ```\n  # in fence\n  ```\n# out\n",
	"1. ```\n   # in fence\n   ```\n\n## out\n",
	"- ```\n  # in fence\n\n  # still in fence\n  ```\n",
	"- ```\n# item ended, fence with it\n",
	"- a\n\n  # in item\n",
	"para\n2. ```\n# lazy text above\n",
	"para\n- ```\n  # in fence\n",
	"-     ```\n# out\n",
	"#\r\n",
	"#\ttab\n",
	"#x\n",
	"   Title\n   ===\n",
}

// astHeadingLevels returns the canonical parser's heading levels for
// src, in document order.
func astHeadingLevels(src string) []int {
	doc := markdown.Parse([]byte(src))
	var levels []int
	_ = ast.Walk(doc.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering {
			levels = append(levels, h.Level)
		}
		return ast.WalkContinue, nil
	})
	return levels
}

// scanHeadingLevels returns the heading levels the include scan reports
// for src, in line order.
func scanHeadingLevels(src string) []int {
	var levels []int
	var scan headingScan
	for _, line := range strings.Split(src, "\n") {
		if level, _ := scan.step(line); level > 0 {
			levels = append(levels, level)
		}
	}
	return levels
}

// TestHeadingScan_IndentMatchesParser checks the scan's heading levels
// against the canonical parser.
func TestHeadingScan_IndentMatchesParser(t *testing.T) {
	for _, src := range indentCases {
		assert.Equal(t, astHeadingLevels(src), scanHeadingLevels(src), "%q", src)
	}
}

// TestAdjustHeadingsByOffset_ShiftMatchesParser checks that the rewrite
// shifts exactly the headings the parser sees: applyShift and
// findMinHeadingLevel run the same scan over the original lines, so the
// shifted text parses to the same headings one level deeper.
func TestAdjustHeadingsByOffset_ShiftMatchesParser(t *testing.T) {
	for _, src := range indentCases {
		want := astHeadingLevels(src)
		for i := range want {
			want[i] = clampLevel(want[i] + 1)
		}
		got := astHeadingLevels(adjustHeadingsByOffset(src, 1))
		assert.Equal(t, want, got, "%q", src)
	}
}

func TestAdjustHeadings_IndentedATXKeepsIndent(t *testing.T) {
	assert.Equal(t, "   ## Indented\n\n### Sub\n", adjustHeadings("   # Indented\n\n## Sub\n", 1))
}

func TestAdjustHeadings_IndentedFenceLineIsNoFence(t *testing.T) {
	assert.Equal(t, "para\n    ```\n## Heading\n", adjustHeadings("para\n    ```\n# Heading\n", 1))
}

func TestAtxHeading(t *testing.T) {
	tests := []struct {
		line          string
		level, indent int
	}{
		{"# A", 1, 0},
		{"###### A", 6, 0},
		{"####### A", 0, 0},
		{"   ## A", 2, 3},
		{"    # A", 0, 0},
		{"\t# A", 0, 0},
		{"#", 1, 0},
		{"#\r", 1, 0},
		{"#\tA", 1, 0},
		{"#A", 0, 0},
		{"", 0, 0},
		{"A", 0, 0},
		{"   ", 0, 0},
	}
	for _, tt := range tests {
		level, indent := atxHeading(tt.line)
		assert.Equal(t, [2]int{tt.level, tt.indent}, [2]int{level, indent}, "%q", tt.line)
	}
}

func TestListContent(t *testing.T) {
	tests := []struct {
		line     string
		col, off int
		ok       bool
	}{
		{"- ```", 2, 2, true},
		{"  * x", 4, 4, true},
		{"1. ```", 3, 3, true},
		{"10) x", 4, 4, true},
		{"-   x", 4, 4, true},
		{"-\tx", 4, 2, true},
		{"-     x", 0, 0, false},
		{"-", 0, 0, false},
		{"- ", 0, 0, false},
		{"-x", 0, 0, false},
		{"    - x", 0, 0, false},
		{"1234567890. x", 0, 0, false},
		{"x", 0, 0, false},
	}
	for _, tt := range tests {
		col, off, ok := listContent([]byte(tt.line))
		assert.Equal(t, tt.ok, ok, "%q", tt.line)
		if ok {
			assert.Equal(t, [2]int{tt.col, tt.off}, [2]int{col, off}, "%q", tt.line)
		}
	}
}

// TestHeadingScan_StepZeroAllocs pins that the per-line scan allocates
// nothing: it runs over every line of every included file.
func TestHeadingScan_StepZeroAllocs(t *testing.T) {
	lines := strings.Split("## A\nTitle\n===\n   # B\n- ```\n  # c\n  ```\n<div>\n\npara\n    ```\n", "\n")
	allocs := testing.AllocsPerRun(100, func() {
		var scan headingScan
		for _, l := range lines {
			_, _ = scan.step(l)
		}
	})
	assert.Zero(t, allocs)
}
