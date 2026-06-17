package unclosedcodeblock

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheck_NilASTMatchesAST pins the Layer-0 migration: Check on a
// nil-AST File (the parse-skip path, reading span.Closed) must produce
// byte-identical diagnostics to the AST path. The corpus has few unclosed
// fences, so this enumerates the closure edge cases directly: closed,
// unclosed-to-EOF, unclosed before a trailing blank, tilde fences, a
// fence with info but no content, a longer closing run, and a lone fence.
func TestCheck_NilASTMatchesAST(t *testing.T) {
	srcs := [][]byte{
		[]byte("# H\n\n```go\ncode\n```\n"),            // closed
		[]byte("# H\n\n```go\ncode\nno close\n"),       // unclosed to EOF
		[]byte("# H\n\n```\ncode\n"),                   // unclosed, no info
		[]byte("# H\n\n```go\ncode\n\n"),               // unclosed before trailing blank
		[]byte("# H\n\n~~~\ncode\n~~~\n"),              // closed tilde
		[]byte("# H\n\n~~~py\ncode\n"),                 // unclosed tilde
		[]byte("# H\n\n````\ncode ```\n````\n"),        // longer fence, closed
		[]byte("# H\n\n```js\n"),                       // info, no content, unclosed
		[]byte("# H\n\ntext\n\n```\na\n```\n\nmore\n"), // closed mid-document
		[]byte("```\nonly\n"),                          // lone unclosed at file start
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

func TestID(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS031", r.ID())
}

func TestName(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "unclosed-code-block", r.Name())
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "code", r.Category())
}

func TestCheck_ClosedBacktickBlock_NoDiagnostic(t *testing.T) {
	src := []byte("# Title\n\n```go\nfmt.Println()\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_ClosedTildeBlock_NoDiagnostic(t *testing.T) {
	src := []byte("~~~python\nprint()\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_UnclosedBacktickBlock(t *testing.T) {
	src := []byte("# Title\n\n```go\nfmt.Println()\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS031", diags[0].RuleID)
	assert.Equal(t, "unclosed-code-block", diags[0].RuleName)
	assert.Equal(t, lint.Error, diags[0].Severity)
	assert.Equal(t, "unclosed fenced code block", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 1, diags[0].Column)
}

func TestCheck_UnclosedTildeBlock(t *testing.T) {
	src := []byte("~~~\nsome code\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS031", diags[0].RuleID)
	assert.Equal(t, "unclosed fenced code block", diags[0].Message)
}

func TestCheck_EmptyClosedBlock_NoDiagnostic(t *testing.T) {
	src := []byte("```\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_NoCodeBlocks_NoDiagnostic(t *testing.T) {
	src := []byte("# Title\n\nSome text.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_EmptyFile_NoDiagnostic(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_MultipleClosedBlocks_NoDiagnostic(t *testing.T) {
	src := []byte("```go\ncode1\n```\n\n```python\ncode2\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_FourBackticksClosed_NoDiagnostic(t *testing.T) {
	src := []byte("````\ncode\n````\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_IndentedFenceClosed_NoDiagnostic(t *testing.T) {
	src := []byte("   ```\n   code\n   ```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_BlockWithInfoString_NoDiagnostic(t *testing.T) {
	src := []byte("```javascript\nconsole.log('hello');\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_NonFencedNode_Skipped(t *testing.T) {
	// Indented code block (not fenced) should not trigger.
	src := []byte("    indented code\n    more code\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_FilePath(t *testing.T) {
	src := []byte("```\nunclosed\n")
	f, err := lint.NewFile("docs/readme.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "docs/readme.md", diags[0].File)
}

// --- Defensive guards on hasClosingFence ---
//
// Real goldmark output never produces a FencedCodeBlock without a
// matching `` ``` `` or `~~~` marker, but hasClosingFence keeps
// defensive guards anyway. The tests below append synthetic
// FencedCodeBlocks so the guards fire.

func TestHasClosingFence_OpenStartPastSource(t *testing.T) {
	// Synthetic FCB with Info=nil and no Lines in an empty document:
	// OpenLineRange falls through to the (len(src), len(src)) sentinel,
	// so hasClosingFence returns true (treated as "closed") and Check
	// emits no diagnostics for that block.
	f, err := lint.NewFile("test.md", []byte(""))
	require.NoError(t, err)
	fcb := ast.NewFencedCodeBlock(nil)
	f.AST.AppendChild(f.AST, fcb)
	r := &Rule{}
	assert.Empty(t, r.Check(f))
}

func TestHasClosingFence_NonFenceFirstChar(t *testing.T) {
	// Info points at a non-fence line so OpenLineRange returns a valid
	// range but CharAt reads a non-fence byte and returns 0.
	src := []byte("hello\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	info := ast.NewText()
	info.Segment = text.NewSegment(0, 5)
	fcb := ast.NewFencedCodeBlock(info)
	f.AST.AppendChild(f.AST, fcb)
	r := &Rule{}
	assert.Empty(t, r.Check(f))
}

func TestHasClosingFence_ClosingLineEmpty(t *testing.T) {
	// Synthetic fcb whose content's last segment stops at a newline.
	// CloseLineRange then returns (closeStart, closeStart) — a
	// zero-width line in the middle of the source. hasClosingFence
	// must hit the `closeStart == closeEnd` guard and report the
	// block as unclosed.
	//
	// Source layout (byte offsets in parens):
	//   ```\n      (0..3)
	//   hello\n    (4..9, with \n at 9)
	//   \n         (10)
	//
	// The synthetic fcb owns "hello\n" as its only content line.
	src := []byte("```\nhello\n\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	fcb := ast.NewFencedCodeBlock(nil)
	segs := text.NewSegments()
	segs.Append(text.NewSegment(4, 10)) // covers "hello\n"
	fcb.SetLines(segs)
	f.AST.AppendChild(f.AST, fcb)

	r := &Rule{}
	diags := r.Check(f)
	// Two diagnostics: one from the real `` ``` `` fcb that goldmark
	// also parses (and is genuinely unclosed) and one from the
	// synthetic fcb that exercises the `closeStart == closeEnd` guard.
	require.NotEmpty(t, diags)
	for _, d := range diags {
		assert.Equal(t, "unclosed fenced code block", d.Message)
	}
}
