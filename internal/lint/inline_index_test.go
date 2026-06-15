package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// astCodeSpanRanges parses src normally and returns the AST-derived
// code-span content and literal ranges — the byte-identical target the
// inline index must reproduce on the parse-skipped path.
func astCodeSpanRanges(t *testing.T, src string) (content, literal []Range) {
	t.Helper()
	f, err := NewFile("doc.md", []byte(src))
	require.NoError(t, err)
	return f.CodeSpanContentRanges(), f.CodeSpanLiteralRanges()
}

// indexCodeSpanRanges builds a nil-AST File from src and returns its
// inline-index-derived code-span ranges.
func indexCodeSpanRanges(src string) (content, literal []Range) {
	f := NewFileLines("doc.md", []byte(src))
	return f.CodeSpanContentRanges(), f.CodeSpanLiteralRanges()
}

// TestInlineIndex_CodeSpanEquivalence pins the inline scanner byte-identical
// to goldmark's CodeSpan node bounds across the cases the corpus exercises:
// plain spans, multi-backtick fences, the single-space trim, all-space
// spans, unclosed runs, backticks inside fenced and indented code blocks,
// adjacent spans, and escaped backticks.
func TestInlineIndex_CodeSpanEquivalence(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"plain", "before `code` after\n"},
		{"double-backtick", "a ``code`` b\n"},
		{"backtick-in-content", "a ``a`b`` c\n"},
		{"single-space-trim", "a `  x ` b\n"},
		{"both-space-trim", "a ` x ` b\n"},
		{"all-spaces", "a `   ` b\n"},
		{"empty-span", "a `` b\n"},
		{"unclosed-run", "a `code without close\n"},
		{"mismatched-len", "a ``code` more\n"},
		{"adjacent", "`one``two`\n"},
		{"two-spans", "`one` and `two`\n"},
		{"escaped-backtick", "a \\`not a span\\` b\n"},
		{"escaped-then-real", "a \\` then `real` b\n"},
		{"span-in-list", "- item `code` here\n"},
		{"span-after-fence", "```\nx\n```\n\nthen `code`\n"},
		{"backtick-in-fence", "```\n`not a span`\n```\n\n`real`\n"},
		{"backtick-in-indented", "    `not a span`\n\n`real`\n"},
		{"multiline-span", "a `line one\nline two` b\n"},
		{"triple-backtick-span", "a ```code``` b\n"},
		{"no-spans", "plain paragraph with no code\n"},
		{"leading-space-only", "a ` x` b\n"},
		{"trailing-space-only", "a `x ` b\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantContent, wantLiteral := astCodeSpanRanges(t, tc.src)
			gotContent, gotLiteral := indexCodeSpanRanges(tc.src)
			assert.Equal(t, wantContent, gotContent, "content ranges diverge from AST")
			assert.Equal(t, wantLiteral, gotLiteral, "literal ranges diverge from AST")
		})
	}
}

// TestInlineIndex_NilSourceFile keeps a struct-literal File (no AST, no
// source) returning nil ranges, matching the AST guard.
func TestInlineIndex_NilSourceFile(t *testing.T) {
	f := &File{}
	assert.Nil(t, f.CodeSpanContentRanges())
	assert.Nil(t, f.CodeSpanLiteralRanges())
}

// TestInlineIndex_Memoized pins the projection to one scan per file.
func TestInlineIndex_Memoized(t *testing.T) {
	f := NewFileLines("doc.md", []byte("x `y` z\n"))
	first := InlineIndexProjection(f)
	second := InlineIndexProjection(f)
	assert.Same(t, first, second)
}
