package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParagraphContinuationLines covers each container a paragraph can
// sit in: every line after a paragraph's first is recorded, including
// lazy continuation lines of a list item or block quote and the content
// lines of a setext heading. First lines, headings, and code are not.
func TestParagraphContinuationLines(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []int
	}{
		{"single-line paragraphs", "# T\n\none\n\ntwo\n", nil},
		{"top-level paragraph", "# T\n\nfirst\nsecond\n  third\n", []int{4, 5}},
		{"list item lazy line", "- item\n#48 lazy\n", []int{2}},
		{"block quote lazy line", "> quote\n#48 lazy\n", []int{2}},
		{"setext heading content", "Head\n#48 more\n---\n", []int{2}},
		{"code is not paragraph", "```\na\nb\n```\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := NewFile("t.md", []byte(tc.src))
			require.NoError(t, err)
			assert.Equal(t, lineSet(tc.want), ParagraphContinuationLines(f))
		})
	}
}

// TestParagraphContinuationLines_NilAST pins the parse-skipped path: a
// File built without an AST parses its source once and returns the same
// set the AST walk does.
func TestParagraphContinuationLines_NilAST(t *testing.T) {
	src := []byte("# T\n\nfirst\n#48 second\n\n- item\n#9 lazy\n")
	parsed, err := NewFile("t.md", src)
	require.NoError(t, err)
	skipped := NewFileLines("t.md", src)
	require.Nil(t, skipped.AST)

	want := lineSet([]int{4, 7})
	assert.Equal(t, want, ParagraphContinuationLines(parsed))
	assert.Equal(t, want, ParagraphContinuationLines(skipped))
}

// TestParagraphContinuationLines_CachedPerFile pins the per-File memo:
// the second call returns the map the first call built.
func TestParagraphContinuationLines_CachedPerFile(t *testing.T) {
	f, err := NewFile("t.md", []byte("a\nb\n"))
	require.NoError(t, err)
	first := ParagraphContinuationLines(f)
	require.NotEmpty(t, first)
	assertSameMap(t, first, ParagraphContinuationLines(f))
}

// lineSet builds the map form ParagraphContinuationLines returns, nil
// for an empty list.
func lineSet(lines []int) map[int]struct{} {
	if len(lines) == 0 {
		return nil
	}
	m := make(map[int]struct{}, len(lines))
	for _, l := range lines {
		m[l] = struct{}{}
	}
	return m
}
