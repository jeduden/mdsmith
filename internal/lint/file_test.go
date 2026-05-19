package lint

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// TestLinkReferences_FromNewFileParse pins that LinkReferences reads the
// definitions goldmark already collected during NewFile's parse, so
// MDS053/MDS054 do not re-parse the document.
func TestLinkReferences_FromNewFileParse(t *testing.T) {
	src := []byte("See [a] and [b].\n\n[a]: https://example.com/a\n[B]: https://example.com/b\n")
	f, err := NewFile("t.md", src)
	require.NoError(t, err)

	refs := f.LinkReferences()
	require.Len(t, refs, 2)

	// Label() casing is goldmark's business; the rules normalize via
	// util.ToLinkReference. Assert presence case-insensitively so this
	// test pins "both defs were captured from the single parse", not
	// goldmark's internal label form.
	labels := map[string]bool{}
	for _, r := range refs {
		labels[strings.ToLower(strings.TrimSpace(string(r.Label())))] = true
	}
	assert.True(t, labels["a"], "definition [a] must be captured")
	assert.True(t, labels["b"], "definition [B] must be captured")

	// Second call returns the cached slice, not a fresh parse.
	again := f.LinkReferences()
	require.Len(t, again, 2)
	assert.Equal(t, refs[0].Label(), again[0].Label())
}

// TestLinkReferences_StructLiteralFallback covers a File built without
// NewFile: there is no captured parse context, so the first call must
// parse Source once on demand and still find the definitions.
func TestLinkReferences_StructLiteralFallback(t *testing.T) {
	f := &File{
		Path:   "t.md",
		Source: []byte("Use [x].\n\n[x]: https://example.com/x\n"),
	}
	refs := f.LinkReferences()
	require.Len(t, refs, 1)
	assert.Equal(t, "x", string(refs[0].Label()))
}

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

// TestFile_Memo pins the per-Check memo: build runs once per key, the
// cached value is served on every later call, and distinct keys are
// independent. This is the primitive the catalog rule uses so its
// generate / injection / case-mismatch passes stop recomputing the
// same directive's resolved entries three times per Check.
func TestFile_Memo(t *testing.T) {
	f := &File{Path: "t.md"}

	var calls int32
	build := func() any {
		atomic.AddInt32(&calls, 1)
		return 42
	}

	require.Equal(t, 42, f.Memo("k", build))
	require.Equal(t, 42, f.Memo("k", build))
	require.Equal(t, 42, f.Memo("k", build))
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"build must run exactly once per key")

	var otherCalls int32
	require.Equal(t, "v2", f.Memo("k2", func() any {
		atomic.AddInt32(&otherCalls, 1)
		return "v2"
	}))
	assert.Equal(t, int32(1), atomic.LoadInt32(&otherCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"a distinct key must not re-run the first key's build")
}

// TestFile_Memo_ConcurrentSingleBuild pins that build runs exactly
// once even under the concurrent readers the LSP can run against a
// single document.
func TestFile_Memo_ConcurrentSingleBuild(t *testing.T) {
	f := &File{Path: "t.md"}

	var calls int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := f.Memo("shared", func() any {
				atomic.AddInt32(&calls, 1)
				return "once"
			})
			assert.Equal(t, "once", v)
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"build must run exactly once under concurrent access")
}
