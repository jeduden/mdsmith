package blanklinearoundheadings

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck_ProperBlankLines_NoViolation(t *testing.T) {
	src := []byte("# Title\n\nSome text\n\n## Section\n\nMore text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_NoBlankBefore(t *testing.T) {
	src := []byte("# Title\n\nSome text\n## Section\n\nMore text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if d.Message == "heading should have a blank line before" {
			found = true
		}
	}
	require.True(t, found, "expected 'blank line before' diagnostic, got: %+v", diags)
}

func TestCheck_NoBlankAfter(t *testing.T) {
	src := []byte("# Title\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if d.Message == "heading should have a blank line after" {
			found = true
		}
	}
	require.True(t, found, "expected 'blank line after' diagnostic, got: %+v", diags)
}

func TestCheck_FirstLine_NoBlankBefore_OK(t *testing.T) {
	src := []byte("# Title\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for heading on line 1, got %d: %+v", len(diags), diags)
}

func TestCheck_LastLine_NoBlankAfter_OK(t *testing.T) {
	src := []byte("Some text\n\n# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for heading on last line, got %d: %+v", len(diags), diags)
}

func TestFix_InsertsBlankLines(t *testing.T) {
	src := []byte("# Title\nSome text\n## Section\nMore text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "# Title\n\nSome text\n\n## Section\n\nMore text\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_AdjacentHeadings_NoDoubleBlanks(t *testing.T) {
	src := []byte("# Title\n## Section\n\nContent here.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "# Title\n\n## Section\n\nContent here.\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS013" {
		t.Errorf("expected MDS013, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "blank-line-around-headings" {
		t.Errorf("expected blank-line-around-headings, got %s", r.Name())
	}
}

// --- Code block awareness tests ---

func TestCheck_HeadingInsideCodeBlock_NoDiagnostics(t *testing.T) {
	// A fenced code block containing heading-like text should not
	// produce MDS013 diagnostics.
	src := []byte("# Real Heading\n\nSome text\n\n```markdown\n# Not a heading\nSome content\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestFix_HeadingInsideCodeBlock_NoCorruption(t *testing.T) {
	// Fix must not modify content inside fenced code blocks.
	src := []byte("# Real Heading\n\nSome text\n\n```markdown\n# Not a heading\nSome content\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("fix corrupted code block content:\nexpected: %q\ngot:      %q", string(src), string(result))
	}
}

// synthCodeAndHeadingFile builds a *lint.File whose AST has a
// FencedCodeBlock covering one source-line range AND a Heading
// whose Lines() segment falls inside that same range. Goldmark's
// parser will never produce this from real Markdown — headings
// and code blocks are mutually exclusive at the parse-line level
// — but the defensive `if codeLines[line] { return nil }` guard
// in CheckNode and collectHeadingBlankLineInsertions exists for
// AST shapes built directly (this test, or any future parser
// behaviour change). Drive both branches red/green here.
func synthCodeAndHeadingFile(t *testing.T) *lint.File {
	t.Helper()
	// Source layout (offsets):
	//   "```\n"        offsets [ 0,  4) line 1
	//   "code\n"       offsets [ 4,  9) line 2
	//   "```\n"        offsets [ 9, 13) line 3
	src := []byte("```\ncode\n```\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	doc := ast.NewDocument()
	fenced := ast.NewFencedCodeBlock(nil)
	fenced.Lines().Append(text.NewSegment(4, 8)) // "code" on line 2
	doc.AppendChild(doc, fenced)

	// Synthetic heading whose segment starts at offset 4 — line 2
	// — which lint.CollectCodeBlockLines will also report.
	heading := ast.NewHeading(1)
	heading.Lines().Append(text.NewSegment(4, 8))
	doc.AppendChild(doc, heading)

	f.AST = doc
	return f
}

// TestCheckNode_HeadingInCodeBlockRegion pins the `if
// codeLines[line] { return nil }` guard in CheckNode (rule.go
// line ~54).
func TestCheckNode_HeadingInCodeBlockRegion(t *testing.T) {
	f := synthCodeAndHeadingFile(t)
	var heading *ast.Heading
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		if h, ok := c.(*ast.Heading); ok {
			heading = h
			break
		}
	}
	require.NotNil(t, heading)
	diags := (&Rule{}).CheckNode(heading, true, f)
	assert.Nil(t, diags, "heading whose line falls inside a code-block region must be skipped")
}

// TestFix_NoTrailingNewline verifies that Fix preserves the absence of
// a trailing newline. bytes.Join round-trips a Split that produces no
// trailing empty element, so the output must not gain a spurious "\n".
func TestFix_NoTrailingNewline(t *testing.T) {
	src := []byte("# Title\nSome text")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "# Title\n\nSome text"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

// TestFix_HeadingInCodeBlockRegion pins the parallel guard
// inside collectHeadingBlankLineInsertions (rule.go line ~153):
// Fix must not insert blank lines around a synthetic heading
// whose line collides with a code block.
func TestFix_HeadingInCodeBlockRegion(t *testing.T) {
	f := synthCodeAndHeadingFile(t)
	got := (&Rule{}).Fix(f)
	assert.Equal(t, f.Source, got, "Fix must not touch a heading whose line is inside a code-block region")
}
