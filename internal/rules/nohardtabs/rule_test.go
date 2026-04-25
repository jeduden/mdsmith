package nohardtabs

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/require"
)

func TestCheck_TabPresent(t *testing.T) {
	// Tab in a normal paragraph line (not code block).
	src := []byte("hel\tlo\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	d := diags[0]
	if d.Line != 1 {
		t.Errorf("expected line 1, got %d", d.Line)
	}
	if d.Column != 4 {
		t.Errorf("expected column 4, got %d", d.Column)
	}
	if d.RuleID != "MDS007" {
		t.Errorf("expected rule ID MDS007, got %s", d.RuleID)
	}
}

func TestCheck_TabMiddleOfLine(t *testing.T) {
	src := []byte("hel\tlo\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Column != 4 {
		t.Errorf("expected column 4, got %d", diags[0].Column)
	}
}

func TestCheck_MultipleLinesWithTabs(t *testing.T) {
	// Tabs in normal paragraph lines (not code blocks).
	src := []byte("fir\tst\nsec\tond\nthird\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 2, "expected 2 diagnostics, got %d", len(diags))
	if diags[0].Line != 1 {
		t.Errorf("expected first diagnostic on line 1, got %d", diags[0].Line)
	}
	if diags[1].Line != 2 {
		t.Errorf("expected second diagnostic on line 2, got %d", diags[1].Line)
	}
}

func TestCheck_NoTabs(t *testing.T) {
	src := []byte("hello\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_EmptyFile(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestFix_ReplacesTabsWithSpaces(t *testing.T) {
	// Tabs in normal paragraph lines should be replaced.
	src := []byte("hel\tlo\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "hel    lo\nworld\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_MultipleTabsOnOneLine(t *testing.T) {
	// Multiple tabs in a normal paragraph line should be replaced.
	src := []byte("a\t\thello\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "a        hello\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_PreservesNoTabLines(t *testing.T) {
	src := []byte("hello\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("expected %q, got %q", string(src), string(result))
	}
}

func TestCheck_SkipsFencedCodeBlockLines(t *testing.T) {
	// Tabs inside a fenced code block should NOT fire MDS007.
	src := []byte("# Title\n\n```\n\tcode\n\tmore\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for tabs inside code block, got %d", len(diags))
}

func TestCheck_TabsOutsideCodeBlockStillFlagged(t *testing.T) {
	// Tabs outside code block should still fire.
	src := []byte("hel\tlo\n\n```\n\tcode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 1 {
		t.Errorf("expected diagnostic on line 1, got %d", diags[0].Line)
	}
}

func TestCheck_SkipsIndentedCodeBlockLines(t *testing.T) {
	// Tabs inside an indented code block should NOT fire MDS007.
	// Note: indented code block lines start with 4 spaces; the tab is after that.
	src := []byte("Some paragraph.\n\n    \tcode\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for tabs inside indented code block, got %d", len(diags))
}

func TestFix_PreservesCodeBlockLines(t *testing.T) {
	// Fix should not replace tabs inside code blocks.
	src := []byte("hel\tlo\n\n```\n\tcode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "hel    lo\n\n```\n\tcode\n```\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() == "" {
		t.Error("expected non-empty category")
	}
}
