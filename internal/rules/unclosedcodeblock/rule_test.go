package unclosedcodeblock

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
