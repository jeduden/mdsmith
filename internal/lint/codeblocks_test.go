package lint

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCollectCodeBlockLines_CachedPerFile pins the memoization added in
// plan 175: a dozen rules call this per file, so it must walk the AST
// once and hand every caller the same map instance.
func TestCollectCodeBlockLines_CachedPerFile(t *testing.T) {
	f, err := NewFile("t.md", []byte("text\n\n```go\nx := 1\n```\n\nmore\n"))
	require.NoError(t, err)

	first := CollectCodeBlockLines(f)
	second := CollectCodeBlockLines(f)

	require.NotEmpty(t, first, "fenced block lines must be detected")
	assertSameMap(t, first, second)
}

// TestCollectPIBlockLines_CachedPerFile mirrors the code-block case for
// the processing-instruction line set.
func TestCollectPIBlockLines_CachedPerFile(t *testing.T) {
	f, err := NewFile("t.md", []byte("# H\n\n<?toc?>\n<?/toc?>\n\nbody\n"))
	require.NoError(t, err)

	first := CollectPIBlockLines(f)
	second := CollectPIBlockLines(f)

	assertSameMap(t, first, second)
}

// TestInCodeOrPI pins the short-circuit membership helper: a line in
// codeLines reports true without consulting piLines, a line only in
// piLines reports true, and a line in neither reports false. Nil maps
// are safe (membership reads on a nil map return the zero value).
func TestInCodeOrPI(t *testing.T) {
	code := map[int]struct{}{1: {}, 3: {}}
	pi := map[int]struct{}{2: {}}

	assert.True(t, InCodeOrPI(code, pi, 1), "line in codeLines")
	assert.True(t, InCodeOrPI(code, pi, 2), "line in piLines")
	assert.False(t, InCodeOrPI(code, pi, 4), "line in neither")
	assert.False(t, InCodeOrPI(nil, nil, 1), "nil maps are safe and report false")
	assert.True(t, InCodeOrPI(nil, pi, 2), "nil codeLines, hit in piLines")
}

// assertSameMap asserts the two maps are the identical backing map,
// which proves the walk ran once and the result was cached rather than
// recomputed (a fresh map each call would have a different address).
func assertSameMap(t *testing.T, a, b map[int]struct{}) {
	t.Helper()
	a[-1] = struct{}{}
	_, ok := b[-1]
	assert.True(t, ok, "second call must return the cached map, not a fresh walk")
	delete(a, -1)
}

func inSet(m map[int]struct{}, k int) bool {
	_, ok := m[k]
	return ok
}

func TestCollectPIBlockLines_MultiLine(t *testing.T) {
	// Lines:
	// 1: # Heading
	// 2: (blank)
	// 3: <?ignore
	// 4: content line
	// 5: ?>
	// 6: (blank)
	// 7: Some text.
	src := []byte("# Heading\n\n<?ignore\ncontent line\n?>\n\nSome text.\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectPIBlockLines(f)
	for _, ln := range []int{3, 4, 5} {
		assert.True(t, inSet(lines, ln), "expected line %d to be in PI block lines", ln)
	}
	for _, ln := range []int{1, 2, 6, 7} {
		assert.False(t, inSet(lines, ln), "expected line %d to NOT be in PI block lines", ln)
	}
}

func TestCollectPIBlockLines_NoPI(t *testing.T) {
	src := []byte("# Title\n\nJust a paragraph.\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectPIBlockLines(f)
	assert.Empty(t, lines)
}

func TestCollectCodeBlockLines_FencedCodeBlock(t *testing.T) {
	// Lines:
	// 1: # Heading
	// 2: (blank)
	// 3: ```
	// 4: code line
	// 5: ```
	// 6: (blank)
	src := []byte("# Heading\n\n```\ncode line\n```\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	// Lines 3 (open fence), 4 (content), 5 (close fence) should be in set.
	for _, ln := range []int{3, 4, 5} {
		assert.True(t, inSet(lines, ln), "expected line %d to be in code block lines", ln)
	}
	// Lines 1, 2 should NOT be in set.
	for _, ln := range []int{1, 2} {
		assert.False(t, inSet(lines, ln), "expected line %d to NOT be in code block lines", ln)
	}
}

func TestCollectCodeBlockLines_FencedWithInfoString(t *testing.T) {
	src := []byte("# Heading\n\n```go\nfmt.Println()\n```\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	for _, ln := range []int{3, 4, 5} {
		assert.True(t, inSet(lines, ln), "expected line %d to be in code block lines", ln)
	}
}

func TestCollectCodeBlockLines_IndentedCodeBlock(t *testing.T) {
	// Indented code block: 4 spaces of indentation, preceded by blank line.
	src := []byte("Some paragraph.\n\n    indented code\n    more code\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	for _, ln := range []int{3, 4} {
		assert.True(t, inSet(lines, ln), "expected line %d to be in code block lines", ln)
	}
	// Line 1 should not be in set.
	assert.False(t, inSet(lines, 1), "expected line 1 to NOT be in code block lines")
}

func TestCollectCodeBlockLines_NoCodeBlocks(t *testing.T) {
	src := []byte("# Title\n\nJust a paragraph.\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	assert.Empty(t, lines, "expected empty map for document with no code blocks")
}

func TestCollectCodeBlockLines_EmptyFencedCodeBlock(t *testing.T) {
	// An empty fenced code block with no info string: the parser records
	// the opener's offset as the node position, so both fence lines are
	// code like any other fence's.
	src := []byte("```\n```\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	for _, ln := range []int{1, 2} {
		assert.True(t, inSet(lines, ln), "expected line %d to be in code block lines", ln)
	}
}

// TestCollectCodeBlockLines_EmptyFenceAfterFence pins the empty,
// info-less block right after another fenced block: its fence lines
// (4 and 5) are code, as fencepos.OpenLine reports for the same block.
func TestCollectCodeBlockLines_EmptyFenceAfterFence(t *testing.T) {
	f, err := NewFile("test.md", []byte("```go\nx\n```\n```\n```\n"))
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	for _, ln := range []int{1, 2, 3, 4, 5} {
		assert.True(t, inSet(lines, ln), "expected line %d to be in code block lines", ln)
	}
}

func TestCollectCodeBlockLines_EmptyFencedCodeBlockWithInfo(t *testing.T) {
	// An empty fenced code block WITH an info string: the opening fence
	// can be located via the Info segment.
	src := []byte("```go\n```\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	// Line 1 (open fence) and line 2 (close fence) should be in the set.
	for _, ln := range []int{1, 2} {
		assert.True(t, inSet(lines, ln), "expected line %d to be in code block lines", ln)
	}
}

func TestCollectCodeBlockLines_MultipleFencedCodeBlocks(t *testing.T) {
	src := []byte("```\nfirst\n```\n\n```\nsecond\n```\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	// Lines 1,2,3 (first block) and 5,6,7 (second block).
	for _, ln := range []int{1, 2, 3, 5, 6, 7} {
		assert.True(t, inSet(lines, ln), "expected line %d to be in code block lines", ln)
	}
	// Line 4 (blank between blocks) should NOT be.
	assert.False(t, inSet(lines, 4), "expected line 4 to NOT be in code block lines")
}

func TestCollectCodeBlockLines_TildeFence(t *testing.T) {
	src := []byte("~~~\ncode\n~~~\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	for _, ln := range []int{1, 2, 3} {
		assert.True(t, inSet(lines, ln), "expected line %d to be in code block lines", ln)
	}
}

func TestCollectCodeBlockLines_FencedWithMultipleContentLines(t *testing.T) {
	src := []byte("```\nline1\nline2\nline3\n```\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	for _, ln := range []int{1, 2, 3, 4, 5} {
		assert.True(t, inSet(lines, ln), "expected line %d to be in code block lines", ln)
	}
}

func TestCollectCodeBlockLines_EmptyDocument(t *testing.T) {
	src := []byte("")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	assert.Empty(t, lines, "expected empty map for empty document")
}

func TestCollectCodeBlockLines_TabIndentedLine(t *testing.T) {
	// A tab-indented line at document start is parsed as an indented
	// code block by goldmark (tab equals 4+ spaces indentation).
	src := []byte("\thello\nworld\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	assert.True(t, inSet(lines, 1), "expected line 1 to be in code block lines (tab-indented = indented code block)")
	assert.False(t, inSet(lines, 2), "expected line 2 to NOT be in code block lines")
}

func TestCollectCodeBlockLines_FencedWithBlankLinesInside(t *testing.T) {
	// Fenced code block with blank lines inside should mark all lines.
	src := []byte("```\ncode\n\n\nmore code\n```\n")
	f, err := NewFile("test.md", src)
	require.NoError(t, err)
	lines := CollectCodeBlockLines(f)
	for _, ln := range []int{1, 2, 3, 4, 5, 6} {
		assert.True(t, inSet(lines, ln), "expected line %d to be in code block lines", ln)
	}
}

// TestCollectPIBlockLinesInto_NilNode pins the recursive descent's
// nil-node guard. The package-level recursion replaces the previous
// ast.Walk closure; the guard exists so unit-test struct-literal
// *File values with no AST stay safe to call.
func TestCollectPIBlockLinesInto_NilNode(t *testing.T) {
	lines := map[int]struct{}{}
	collectPIBlockLinesInto(nil, &File{}, lines)
	assert.Empty(t, lines)
}

// TestCollectCodeBlockLinesInto_NilNode pins the same nil-node
// guard for the code-block walker. Mirrors PIBlockLinesInto.
func TestCollectCodeBlockLinesInto_NilNode(t *testing.T) {
	lines := map[int]struct{}{}
	collectCodeBlockLinesInto(nil, &File{}, lines)
	assert.Empty(t, lines)
}

// TestFindFencedOpenLine_Positions pins the opener line read from the
// node position: an empty, info-less block (no Info, no content to
// infer from), one after another fenced block, one inside a list item
// and a block quote, and a hand-built node with no position (0).
func TestFindFencedOpenLine_Positions(t *testing.T) {
	cases := []struct {
		src  string
		want []int
	}{
		{"```\n```\n", []int{1}},
		{"```go\nx\n```\n```\n```\n", []int{1, 4}},
		{"# T\n\n- ```\n  x\n  ```\n", []int{3}},
		{"# T\n\n> ~~~\n> ~~~\n", []int{3}},
	}
	for _, tc := range cases {
		f, err := NewFile("test.md", []byte(tc.src))
		require.NoError(t, err)
		var got []int
		_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if fcb, ok := n.(*ast.FencedCodeBlock); ok && entering {
				got = append(got, FindFencedOpenLine(f, fcb))
			}
			return ast.WalkContinue, nil
		})
		assert.Equal(t, tc.want, got, tc.src)
	}

	f, err := NewFile("test.md", []byte("```\n```\n"))
	require.NoError(t, err)
	assert.Equal(t, 0, FindFencedOpenLine(f, ast.NewFencedCodeBlock(nil)))
}

// TestCollectCodeBlockLines_NilASTUsesLayer0 pins the parse-skipped path:
// a File built via NewFileLines has a nil AST, so collectCodeBlockLines
// falls back to the on-demand Layer 0 scan rather than walking the tree.
func TestCollectCodeBlockLines_NilASTUsesLayer0(t *testing.T) {
	f := NewFileLines("t.md", []byte("text\n\n```go\nx := 1\n```\n\nmore\n"))
	require.Nil(t, f.AST, "NewFileLines must leave AST nil")

	lines := CollectCodeBlockLines(f)
	assert.Equal(t, []int{3, 4, 5}, keysOf(lines))
}

// TestCollectCodeBlockLines_PreScannedLayer0 pins the fast path where the
// Layer 0 scan already ran (f.layer0Done is set), so collectCodeBlockLines
// serves the cached scan's set directly without re-scanning or walking the
// tree.
func TestCollectCodeBlockLines_PreScannedLayer0(t *testing.T) {
	f := NewFileLines("t.md", []byte("```go\nx := 1\n```\n"))
	_ = Layer0(f) // prime the cache so layer0Done is set
	require.True(t, f.layer0Done.Load())

	lines := CollectCodeBlockLines(f)
	assert.Equal(t, []int{1, 2, 3}, keysOf(lines))
}

// TestCollectPIBlockLines_NilASTUsesLayer0 mirrors the code-block nil-AST
// fallback for the processing-instruction line set.
func TestCollectPIBlockLines_NilASTUsesLayer0(t *testing.T) {
	f := NewFileLines("t.md", []byte("# H\n\n<?toc?>\n<?/toc?>\n\nbody\n"))
	require.Nil(t, f.AST, "NewFileLines must leave AST nil")

	lines := CollectPIBlockLines(f)
	assert.Equal(t, []int{3, 4}, keysOf(lines))
}

// TestCollectPIBlockLines_PreScannedLayer0 pins the fast path where the
// Layer 0 scan already ran, so collectPIBlockLines serves the cached set.
func TestCollectPIBlockLines_PreScannedLayer0(t *testing.T) {
	f := NewFileLines("t.md", []byte("<?toc?>\n"))
	_ = Layer0(f)
	require.True(t, f.layer0Done.Load())

	lines := CollectPIBlockLines(f)
	assert.Equal(t, []int{1}, keysOf(lines))
}
