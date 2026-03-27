package blanklinearoundheadings

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

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
