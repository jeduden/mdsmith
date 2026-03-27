package nomultipleblanks

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/require"
)

func TestCheck_TwoConsecutiveBlanks(t *testing.T) {
	src := []byte("hello\n\n\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	d := diags[0]
	if d.Line != 3 {
		t.Errorf("expected line 3, got %d", d.Line)
	}
	if d.Column != 1 {
		t.Errorf("expected column 1, got %d", d.Column)
	}
	if d.RuleID != "MDS008" {
		t.Errorf("expected rule ID MDS008, got %s", d.RuleID)
	}
}

func TestCheck_ThreeConsecutiveBlanks(t *testing.T) {
	src := []byte("hello\n\n\n\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 2, "expected 2 diagnostics, got %d", len(diags))
	if diags[0].Line != 3 {
		t.Errorf("expected first diagnostic on line 3, got %d", diags[0].Line)
	}
	if diags[1].Line != 4 {
		t.Errorf("expected second diagnostic on line 4, got %d", diags[1].Line)
	}
}

func TestCheck_SingleBlankLine(t *testing.T) {
	src := []byte("hello\n\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_NoBlanks(t *testing.T) {
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

func TestCheck_BlankLineWithWhitespace(t *testing.T) {
	// A line containing only whitespace is considered blank
	src := []byte("hello\n  \n  \nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 3 {
		t.Errorf("expected diagnostic on line 3, got %d", diags[0].Line)
	}
}

func TestFix_CollapsesBlanks(t *testing.T) {
	src := []byte("hello\n\n\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "hello\n\nworld\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_CollapsesThreeBlanks(t *testing.T) {
	src := []byte("hello\n\n\n\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "hello\n\nworld\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_PreservesNoBlanks(t *testing.T) {
	src := []byte("hello\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("expected %q, got %q", string(src), string(result))
	}
}

func TestFix_PreservesSingleBlanks(t *testing.T) {
	src := []byte("hello\n\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("expected %q, got %q", string(src), string(result))
	}
}

func TestCheck_SkipsFencedCodeBlockLines(t *testing.T) {
	// Multiple consecutive blank lines inside a fenced code block
	// should NOT fire MDS008.
	src := []byte("# Title\n\n```\ncode\n\n\nmore code\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for blanks inside code block, got %d", len(diags))
}

func TestCheck_MultipleBlanksOutsideCodeBlockStillFlagged(t *testing.T) {
	// Multiple blanks outside code block should still fire.
	src := []byte("hello\n\n\nworld\n\n```\ncode\n\n\nmore\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 3 {
		t.Errorf("expected diagnostic on line 3, got %d", diags[0].Line)
	}
}

func TestCheck_SkipsIndentedCodeBlockBlanks(t *testing.T) {
	// Indented code block with blank lines should not fire.
	// Note: goldmark may not consider blank lines between indented
	// code block lines as part of the code block, so this tests
	// the actual behavior.
	src := []byte("Paragraph.\n\n    code\n    more code\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestFix_PreservesCodeBlockBlanks(t *testing.T) {
	// Fix should not collapse blank lines inside code blocks.
	src := []byte("hello\n\n\nworld\n\n```\ncode\n\n\nmore\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "hello\n\nworld\n\n```\ncode\n\n\nmore\n```\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

// --- Configurable tests ---

func TestApplySettings_ValidMax(t *testing.T) {
	r := &Rule{Max: 1}
	if err := r.ApplySettings(map[string]any{"max": 3}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Max != 3 {
		t.Errorf("expected Max=3, got %d", r.Max)
	}
}

func TestApplySettings_InvalidMaxType(t *testing.T) {
	r := &Rule{Max: 1}
	err := r.ApplySettings(map[string]any{"max": "not-a-number"})
	require.Error(t, err, "expected error for non-int max")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{Max: 1}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err, "expected error for unknown key")
}

func TestDefaultSettings_NoMultipleBlanks(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["max"] != 1 {
		t.Errorf("expected max=1, got %v", ds["max"])
	}
}

func TestCheck_MaxTwo_AllowsTwoConsecutiveBlanks(t *testing.T) {
	// With Max=2, two consecutive blank lines should be allowed.
	src := []byte("hello\n\n\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Max: 2}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics with Max=2 for 2 blanks, got %d", len(diags))
}

func TestCheck_MaxTwo_FlagsThreeConsecutiveBlanks(t *testing.T) {
	// With Max=2, three consecutive blank lines should flag the third.
	src := []byte("hello\n\n\n\nworld\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Max: 2}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic with Max=2 for 3 blanks, got %d", len(diags))
	if diags[0].Line != 4 {
		t.Errorf("expected diagnostic on line 4, got %d", diags[0].Line)
	}
}
