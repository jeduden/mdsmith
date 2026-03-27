package lint

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

func TestNewFile_EmptyContent(t *testing.T) {
	f, err := NewFile("test.md", []byte(""))
	require.NoError(t, err)
	require.NotNil(t, f.AST, "expected non-nil AST for empty content")
	assert.Equal(t, ast.KindDocument, f.AST.Kind())
	assert.Equal(t, "test.md", f.Path)
}

func TestNewFile_WithMarkdownContent(t *testing.T) {
	source := []byte("# Heading\n\nSome text.\n\n- item 1\n- item 2\n\n```go\nfmt.Println()\n```\n")
	f, err := NewFile("doc.md", source)
	require.NoError(t, err)
	require.NotNil(t, f.AST, "expected non-nil AST")
	assert.Equal(t, ast.KindDocument, f.AST.Kind())
	// The document should have child nodes for heading, paragraph, list, code block.
	assert.True(t, f.AST.HasChildren(), "expected AST to have children for non-empty markdown")
}

func TestNewFile_LinesSplitCorrectly(t *testing.T) {
	source := []byte("line one\nline two\nline three")
	f, err := NewFile("lines.md", source)
	require.NoError(t, err)
	require.Len(t, f.Lines, 3)
	assert.Equal(t, "line one", string(f.Lines[0]))
	assert.Equal(t, "line two", string(f.Lines[1]))
	assert.Equal(t, "line three", string(f.Lines[2]))
}

func TestNewFile_TrailingNewline(t *testing.T) {
	source := []byte("line one\nline two\n")
	f, err := NewFile("trailing.md", source)
	require.NoError(t, err)
	// bytes.Split on trailing newline produces an empty last element.
	require.Len(t, f.Lines, 3, "expected 3 lines (including empty trailing)")
	assert.Equal(t, "", string(f.Lines[2]), "expected empty trailing line")
}

func TestNewFileFromSource_WithFrontMatter(t *testing.T) {
	source := []byte("---\ntitle: hello\n---\n# Heading\n")
	f, err := NewFileFromSource("test.md", source, true)
	require.NoError(t, err)

	// FrontMatter should contain the entire prefix including delimiters.
	assert.Equal(t, "---\ntitle: hello\n---\n", string(f.FrontMatter))

	// LineOffset should be 3 (three newlines in the front matter).
	assert.Equal(t, 3, f.LineOffset)

	// Source should be the stripped content only.
	assert.Equal(t, "# Heading\n", string(f.Source))
}

func TestNewFileFromSource_WithoutFrontMatter(t *testing.T) {
	source := []byte("# Heading\nSome text.\n")
	f, err := NewFileFromSource("test.md", source, true)
	require.NoError(t, err)

	assert.Nil(t, f.FrontMatter)
	assert.Equal(t, 0, f.LineOffset)
	assert.Equal(t, string(source), string(f.Source))
}

func TestNewFileFromSource_StripDisabled(t *testing.T) {
	source := []byte("---\ntitle: hello\n---\n# Heading\n")
	f, err := NewFileFromSource("test.md", source, false)
	require.NoError(t, err)

	assert.Nil(t, f.FrontMatter)
	assert.Equal(t, 0, f.LineOffset)
	// Source should be the full content (front matter not stripped).
	assert.Equal(t, string(source), string(f.Source))
}

func TestAdjustDiagnostics_ShiftsLineNumbers(t *testing.T) {
	f := &File{LineOffset: 5}
	diags := []Diagnostic{
		{Line: 1, Column: 3, RuleID: "MDS001"},
		{Line: 10, Column: 1, RuleID: "MDS002"},
	}
	f.AdjustDiagnostics(diags)

	assert.Equal(t, 6, diags[0].Line)
	assert.Equal(t, 15, diags[1].Line)
}

func TestAdjustDiagnostics_ZeroOffsetNoOp(t *testing.T) {
	f := &File{LineOffset: 0}
	diags := []Diagnostic{
		{Line: 1, Column: 1, RuleID: "MDS001"},
	}
	f.AdjustDiagnostics(diags)

	assert.Equal(t, 1, diags[0].Line)
}

func TestFullSource_PrependsFrontMatter(t *testing.T) {
	fm := []byte("---\ntitle: hello\n---\n")
	f := &File{FrontMatter: fm}
	body := []byte("# Heading\n")

	got := f.FullSource(body)
	want := append(fm, body...)
	assert.Equal(t, want, got)
}

func TestFullSource_NoFrontMatter(t *testing.T) {
	f := &File{}
	body := []byte("# Heading\n")

	got := f.FullSource(body)
	assert.Equal(t, body, got)
}

func TestFullSource_DoesNotMutateFrontMatter(t *testing.T) {
	fm := []byte("---\ntitle: hello\n---\n")
	origLen := len(fm)
	origContent := make([]byte, len(fm))
	copy(origContent, fm)

	f := &File{FrontMatter: fm}
	body := []byte("# Heading\n")

	// First call: check result is correct.
	got := f.FullSource(body)
	want := append(origContent, body...)
	assert.Equal(t, want, got)

	// Assert FrontMatter is unchanged.
	assert.Equal(t, origLen, len(f.FrontMatter))
	assert.Equal(t, origContent, f.FrontMatter)

	// Second call: idempotent result.
	got2 := f.FullSource(body)
	assert.Equal(t, want, got2)
}

func TestNewFileFromSource_EmptyFrontMatter(t *testing.T) {
	source := []byte("---\n---\n# Heading\n")
	f, err := NewFileFromSource("test.md", source, true)
	require.NoError(t, err)

	// LineOffset should be 2 (two newlines in "---\n---\n").
	assert.Equal(t, 2, f.LineOffset)

	// FrontMatter should be the delimiters only.
	assert.Equal(t, "---\n---\n", string(f.FrontMatter))

	// Source should be the content after front matter.
	assert.True(t, strings.HasPrefix(string(f.Source), "# Heading"), "expected Source to start with '# Heading'")
}

func TestNewFileFromSource_EmptySource(t *testing.T) {
	f, err := NewFileFromSource("test.md", []byte(""), true)
	require.NoError(t, err)

	assert.Equal(t, 0, f.LineOffset)
	assert.Nil(t, f.FrontMatter)
}
