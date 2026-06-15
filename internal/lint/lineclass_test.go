package lint

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// equivCases are markdown snippets whose flat-classifier code-block line
// set must equal the AST-derived set. They cover the block shapes the
// corpus gate cannot guarantee are present: indented code, blockquote- and
// list-nested fences, HTML comments with indented interiors, setext
// headings, unclosed and empty fences. Each runs in CI (no corpus needed).
var equivCases = map[string]string{
	"plain fenced":            "# H\n\n```go\nx := 1\n```\n\ntext\n",
	"tilde fence":             "~~~\ncode\n~~~\n",
	"fence no info":           "```\ncode\n```\n",
	"fence indented 3":        "text\n\n   ```\n   code\n   ```\n",
	"backtick in info string": "```go `inline`\ncode\n```\n",
	"indented code":           "para\n\n    code line\n    more\n\nafter\n",
	"indented after para":     "a paragraph\n    not code, lazy cont\n",
	"list with fence":         "- item\n\n  ```go\n  code\n  ```\n",
	"ordered list fence":      "1. step\n\n   ```\n   code\n   ```\n",
	"blockquote fence":        "> ```go\n> code\n> ```\n",
	"nested list quote fence": "- a\n  1. b\n\n     > ```rust\n     > code\n     > ```\n",
	"html comment indented":   "text\n\n<!-- a\n\n    indented inside comment\n-->\n\nafter\n",
	"setext heading":          "Title\n=====\n\nbody\n",
	"setext dash":             "Title\n---\n\nbody\n",
	"unclosed fence empty":    "# h\n\n```\n",
	"unclosed fence content":  "# h\n\n```\ncode\n",
	"empty closed fence":      "```\n```\n",
	"empty closed fence info": "```go\n```\n",
	"multiple fences":         "```\nfirst\n```\n\n```\nsecond\n```\n",
	"fence blank inside":      "```\ncode\n\n\nmore\n```\n",
	"tab indented code":       "\thello\nworld\n",
	"cdata block":             "<![CDATA[\n    x\n]]>\n",
	"no code at all":          "# Title\n\nJust prose, nothing fenced.\n",
	// Regression guards for the code-review pass (recall): a fence nested
	// in a container whose interior or close drops the container prefix
	// must close at that boundary, not run on; an indented code block must
	// fold its interior blank; and CRLF lines must classify on `\r`-free
	// content.
	"bq fence drops prefix":    "> ```\n> a\nb\n> ```\n",
	"bq fence multi then drop": "> ```\n> a\n> b\nc\n",
	"list fence dedents":       "- x\n\n  ```\ncode_dedent\n  ```\n",
	"indented interior blank":  "para\n\n    code1\n\n    code2\n",
	"indented trailing blank":  "para\n\n    code\n\n",
	"crlf fenced block":        "```\r\nx\r\n```\r\nafter\r\n",
	"crlf indented code":       "para\r\n\r\n    code\r\n",
	// Second code-review pass: tab indentation and indented-code-in-list,
	// plus type-1 raw HTML blocks (script/pre/style), whose interiors a
	// fence-only or space-only scanner mishandles.
	"tab fence body in list":   "- ```\n\tcode\n",
	"tab after blockquote":     "> \tnot code, two cols\n",
	"tab after list indent":    "- a\n\n  \tnot code\n",
	"list item indented code":  "- a\n-     code\n",
	"ordered item code":        "1. a\n2.     code\n",
	"type1 script blank indt":  "<script>\n\n    x = 1\n</script>\n",
	"type1 pre with em":        "<pre><code>a <em>b</em>\n    c\n</code></pre>\n",
	"html comment in bq drop":  "> <!-- x\n> y\nz\n> -->\n",
	"single line html comment": "<!-- one line -->\n\ntext\n",
}

func sortedKeys(m map[int]struct{}) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// TestFlatClassifierEquivalence_Cases pins the flat classifier's
// code-block set against the AST set for every snippet in equivCases. It
// is the always-on (no-corpus) half of the equivalence gate.
func TestFlatClassifierEquivalence_Cases(t *testing.T) {
	names := make([]string, 0, len(equivCases))
	for n := range equivCases {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		src := []byte(equivCases[name])
		t.Run(name, func(t *testing.T) {
			f, err := NewFile("t.md", src)
			require.NoError(t, err)
			astSet := collectCodeBlockLines(f)
			flatSet := ClassifyLines(f.Lines).CodeBlockLines()
			assert.Equal(t, sortedKeys(astSet), sortedKeys(flatSet),
				"flat code-block set must match AST for:\n%s", src)
		})
	}
}

// TestClassifyLines_Classes pins the per-line LineClass values for a
// representative document so the class vocabulary (task 1) is exercised
// directly, not only through the code-block projection.
func TestClassifyLines_Classes(t *testing.T) {
	src := splitLines("# Heading\n\npara\n\n```go\ncode\n```\n")
	lc := ClassifyLines(src)
	want := []LineClass{
		LineATXHeading, // # Heading
		LineBlank,      // (blank)
		LineParagraph,  // para
		LineBlank,      // (blank)
		LineFenceOpen,  // ```go
		LineInCode,     // code
		LineFenceClose, // ```
		LineBlank,      // trailing "" from final newline
	}
	for i, w := range want {
		assert.Equalf(t, w, lc.Class(i+1), "line %d class", i+1)
	}
	assert.Equal(t, LineParagraph, lc.Class(0), "out-of-range low")
	assert.Equal(t, LineParagraph, lc.Class(99), "out-of-range high")
}

// TestClassifyLines_HeadingLines pins ATX and setext heading detection,
// including that a setext underline marks both its own line and the title
// line above it (matching the line-length rule's heading-line need).
func TestClassifyLines_HeadingLines(t *testing.T) {
	lc := ClassifyLines(splitLines("# ATX\n\nSetext Title\n======\n\nbody\n"))
	assert.Equal(t, []int{1, 3, 4}, sortedKeys(lc.HeadingLines()))
}

// TestClassifyLines_FrontMatter pins the leading front-matter bounds the
// standalone classifier records, and that a file without front matter
// reports none.
func TestClassifyLines_FrontMatter(t *testing.T) {
	lc := ClassifyLines(splitLines("---\ntitle: x\nkey: y\n---\n\n# Body\n"))
	from, to, ok := lc.FrontMatter()
	require.True(t, ok)
	assert.Equal(t, 1, from)
	assert.Equal(t, 4, to)
	assert.Equal(t, LineFrontMatter, lc.Class(2))

	lc2 := ClassifyLines(splitLines("# No front matter\n\nbody\n"))
	_, _, ok2 := lc2.FrontMatter()
	assert.False(t, ok2)
}

// TestClassifyLines_FrontMatterUnterminated pins that a leading `---` with
// no closing delimiter is not treated as front matter (the whole document
// is classified normally).
func TestClassifyLines_FrontMatterUnterminated(t *testing.T) {
	lc := ClassifyLines(splitLines("---\njust a thematic break and text\n"))
	_, _, ok := lc.FrontMatter()
	assert.False(t, ok)
}

// TestClassifyLines_Empty pins that an empty document classifies without
// panicking and yields no code or heading lines.
func TestClassifyLines_Empty(t *testing.T) {
	lc := ClassifyLines(nil)
	assert.Nil(t, lc.CodeBlockLines())
	assert.Nil(t, lc.HeadingLines())
	_, _, ok := lc.FrontMatter()
	assert.False(t, ok)
}

// TestFlatHeadingLines pins both arms of the flat heading-line accessor:
// an AST-backed File has no classifier and reports (nil, false), so the
// line-length rule takes its AST walk; a File built on the parse-skip path
// reports the classifier's heading set.
func TestFlatHeadingLines(t *testing.T) {
	astFile, err := NewFile("t.md", []byte("# H\n"))
	require.NoError(t, err)
	hl, ok := FlatHeadingLines(astFile)
	assert.Nil(t, hl)
	assert.False(t, ok)

	flat, release := NewFileFlatPooled("t.md", []byte("# H1\n\nSetext\n===\n"), false)
	defer release()
	hl, ok = FlatHeadingLines(flat)
	assert.True(t, ok)
	assert.Equal(t, []int{1, 3, 4}, sortedKeys(hl))
}

// TestClassifyLines_AllocBudget pins acceptance criterion 1: the flat
// classifier builds no node tree and stays under the per-rule allocation
// budget (≤10 allocs/op, CLAUDE.md) on representative input. It allocates
// only a flat class slice and the two lazily-built line sets — no ast.Node
// per line, which is the whole point of Layer 0.
func TestClassifyLines_AllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc budget skipped in -short mode")
	}
	lines := splitLines(allocBudgetClassifierFixture)
	allocs := testing.AllocsPerRun(100, func() {
		_ = ClassifyLines(lines)
	})
	assert.LessOrEqualf(t, allocs, float64(10),
		"ClassifyLines allocates %.1f/op, ceiling 10 (CLAUDE.md per-rule budget)", allocs)
}

// allocBudgetClassifierFixture mirrors the integration alloc-budget
// fixture's feature mix (heading, prose, fenced code, list, table) so the
// classifier's representative-input cost is measured against a realistic
// document.
const allocBudgetClassifierFixture = "# Document title\n" +
	"\n" +
	"A short prose paragraph for the classifier to scan.\n" +
	"\n" +
	"## Section\n" +
	"\n" +
	"```go\nfunc f() int { return 0 }\n```\n" +
	"\n" +
	"- one item\n" +
	"- two items\n"

// splitLines is the test helper mirroring how lint.File builds f.Lines.
func splitLines(s string) [][]byte {
	f, _ := NewFile("t.md", []byte(s))
	return f.Lines
}

// Example shows the classifier reporting which 1-based lines fall inside
// code blocks for a small document.
func ExampleClassifyLines() {
	lc := ClassifyLines(splitLines("text\n\n```\ncode\n```\n"))
	fmt.Println(sortedKeys(lc.CodeBlockLines()))
	// Output: [3 4 5]
}
