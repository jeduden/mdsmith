package fencedcodelanguage

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFenceLineHasInfo covers the info-string read the Layer-0 path uses,
// including the all-blank and non-fence fallbacks (a real span guarantees
// a fence, so those branches are reached only here).
func TestFenceLineHasInfo(t *testing.T) {
	cases := map[string]bool{
		"```go":       true,
		"~~~python":   true,
		"```":         false,
		"~~~   ":      false,
		"   ```js":    true,
		"        ":    false, // all spaces, no fence
		"not a fence": false, // first non-space is not a fence char
	}
	for line, want := range cases {
		assert.Equal(t, want, fenceLineHasInfo([]byte(line)), "line %q", line)
	}
}

// TestCheck_NilASTMatchesAST pins the Layer-0 migration: Check on a
// nil-AST File (the parse-skip path) must produce byte-identical
// diagnostics to the AST path, covering fences with and without an info
// string, tilde fences, and an info string that is only whitespace.
func TestCheck_NilASTMatchesAST(t *testing.T) {
	srcs := [][]byte{
		[]byte("# H\n\n```go\ncode\n```\n"),
		[]byte("# H\n\n```\ncode\n```\n"),
		[]byte("# H\n\n~~~\ncode\n~~~\n"),
		[]byte("# H\n\n~~~python\ncode\n~~~\n"),
		[]byte("# H\n\ntext\n\n```   \ncode\n```\n\nmore\n"),
		[]byte("# H\n\n   ```js\ncode\n   ```\n"),
	}
	for _, src := range srcs {
		astFile, err := lint.NewFile("f.md", src)
		require.NoError(t, err)
		astDiags := (&Rule{}).Check(astFile)
		l0Diags := (&Rule{}).Check(lint.NewFileLines("f.md", src))
		assert.Equal(t, astDiags, l0Diags,
			"nil-AST must match AST for src=%q", string(src))
	}
}

func TestCheck_MissingLanguage(t *testing.T) {
	src := []byte("```\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	d := diags[0]
	if d.RuleID != "MDS011" {
		t.Errorf("expected rule ID MDS011, got %s", d.RuleID)
	}
	if d.Line != 1 {
		t.Errorf("expected line 1, got %d", d.Line)
	}
	if d.Message != "fenced code block should have a language tag" {
		t.Errorf("unexpected message: %s", d.Message)
	}
}

func TestCheck_WithLanguage(t *testing.T) {
	src := []byte("```go\nfmt.Println()\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_TildeWithLanguage(t *testing.T) {
	src := []byte("~~~python\nprint()\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_TildeWithoutLanguage(t *testing.T) {
	src := []byte("~~~\ncode\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
}

func TestCheck_EmptyCodeBlock(t *testing.T) {
	src := []byte("```\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
}

func TestCheck_MultipleBlocks_OnlyMissingFlagged(t *testing.T) {
	src := []byte("```go\ncode1\n```\n\n```\ncode2\n```\n\n```python\ncode3\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 5 {
		t.Errorf("expected line 5, got %d", diags[0].Line)
	}
}

func TestCheck_DiagnosticPointsToOpeningFence(t *testing.T) {
	src := []byte("# Title\n\n```\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_ID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS011" {
		t.Errorf("expected ID MDS011, got %s", r.ID())
	}
}

func TestCheck_Name(t *testing.T) {
	r := &Rule{}
	if r.Name() != "fenced-code-language" {
		t.Errorf("expected name fenced-code-language, got %s", r.Name())
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() == "" {
		t.Error("expected non-empty category")
	}
}
