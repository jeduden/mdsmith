package blanklinearoundlists

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck_NoBlanksBeforeList(t *testing.T) {
	src := []byte("Some text\n- item 1\n- item 2\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	// Should report "list should be preceded by a blank line"
	found := false
	for _, d := range diags {
		if d.Message == "list should be preceded by a blank line" {
			found = true
			if d.RuleID != "MDS014" {
				t.Errorf("expected rule ID MDS014, got %s", d.RuleID)
			}
		}
	}
	assert.True(t, found, "expected diagnostic about missing blank before list, got %d diags: %+v", len(diags), diags)
}

func TestCheck_NoBlanksAfterList(t *testing.T) {
	// Use a heading after the list which creates a clear block boundary.
	// (Plain text after a list without blank line gets absorbed into the list item by goldmark.)
	src := []byte("- item 1\n- item 2\n# After\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if d.Message == "list should be followed by a blank line" {
			found = true
		}
	}
	assert.True(t, found, "expected diagnostic about missing blank after list, got %d diags: %+v", len(diags), diags)
}

func TestCheck_BlanksAroundList(t *testing.T) {
	src := []byte("Some text\n\n- item 1\n- item 2\n\nMore text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_ListAtStartOfFile(t *testing.T) {
	src := []byte("- item 1\n- item 2\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for list at start of file, got %d: %+v", len(diags), diags)
}

func TestCheck_ListAtEndOfFile(t *testing.T) {
	src := []byte("Some text\n\n- item 1\n- item 2\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for list at end of file, got %d: %+v", len(diags), diags)
}

func TestCheck_NestedListsNoFlag(t *testing.T) {
	src := []byte("Some text\n\n- item 1\n  - nested 1\n  - nested 2\n- item 2\n\nMore text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for nested lists, got %d: %+v", len(diags), diags)
}

func TestCheck_ListAfterHeading(t *testing.T) {
	src := []byte("# Heading\n- item 1\n- item 2\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if d.Message == "list should be preceded by a blank line" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic about missing blank before list after heading, got %d diags: %+v",
			len(diags), diags)
	}
}

func TestCheck_EmptyFile(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestFix_InsertsBlankBefore(t *testing.T) {
	src := []byte("Some text\n- item 1\n- item 2\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "Some text\n\n- item 1\n- item 2\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_InsertsBlankAfter(t *testing.T) {
	src := []byte("- item 1\n- item 2\n# After\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	expected := "- item 1\n- item 2\n\n# After\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_NoChange(t *testing.T) {
	src := []byte("Some text\n\n- item 1\n- item 2\n\nMore text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("expected no change, got %q", string(result))
	}
}

// --- Code block awareness tests ---

func TestCheck_FencedCodeBlockWithYAMLList_NoDiagnostics(t *testing.T) {
	// Fenced code block containing YAML list markers inside a numbered list item.
	// MDS014 must not report diagnostics for list-like content inside code blocks.
	src := []byte("1. Configure the template:\n\n   ```yaml\n   template:\n     - item-one\n     - item-two\n   ```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	for _, d := range diags {
		if d.RuleID == "MDS014" {
			t.Errorf("unexpected MDS014 diagnostic inside code block: %+v", d)
		}
	}
}

func TestFix_FencedCodeBlockWithYAMLList_NoCorruption(t *testing.T) {
	// Fix must not modify content inside fenced code blocks.
	src := []byte("1. Configure the template:\n\n   ```yaml\n   template:\n     - item-one\n     - item-two\n   ```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("fix corrupted code block content:\nexpected: %q\ngot:      %q", string(src), string(result))
	}
}

func TestCheck_ListBeforeCodeBlock_StillFires(t *testing.T) {
	// A real list immediately before a fenced code block should still get diagnostics.
	src := []byte("Some text\n- item 1\n- item 2\n\n```\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if d.Message == "list should be preceded by a blank line" {
			found = true
		}
	}
	assert.True(t, found, "expected diagnostic for list before code block, got %d diags: %+v", len(diags), diags)
}

func TestCheck_ListAfterCodeBlock_StillFires(t *testing.T) {
	// A real list immediately after a fenced code block should still get diagnostics.
	src := []byte("```\ncode\n```\n- item 1\n- item 2\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if d.Message == "list should be preceded by a blank line" {
			found = true
		}
	}
	assert.True(t, found, "expected diagnostic for list after code block, got %d diags: %+v", len(diags), diags)
}

func TestCheck_ListInsideIndentedCodeBlock_NoDiagnostics(t *testing.T) {
	// Indented code block (4+ spaces) containing list-like content.
	src := []byte("Paragraph\n\n    - not a real list\n    - also not a list\n\nMore text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	for _, d := range diags {
		if d.RuleID == "MDS014" {
			t.Errorf("unexpected MDS014 diagnostic inside indented code block: %+v", d)
		}
	}
}

func TestCheck_EmptyFencedCodeBlockAdjacentToList_NoDiagnostics(t *testing.T) {
	// Empty fenced code block adjacent to a list. The list should get
	// diagnostics but the code block lines must not be treated as list content.
	src := []byte("Some text\n\n- item 1\n\n```\n```\n\nMore text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

// TestFix_PreSizeAllocBudget verifies that Fix pre-sizes resultLines with
// make([][]byte, 0, cap) instead of starting from nil, so the backing array
// is allocated once rather than growing 4× for a file with several insertions.
// Budget is below the current 8-alloc baseline.
func TestFix_PreSizeAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	// 4 lines; 2 blank-line insertions required (before list and after list).
	src := []byte("text\n- item1\n- item2\nmore text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	_ = r.Fix(f) // warm up
	const (
		runs = 100
		// Current: 8 allocs (4 growth allocs from nil resultLines + 2-map setup +
		// bytes.Join). After make(0, cap): growth allocs replaced by 1 make alloc;
		// budget = 6 requires at least 2 allocs saved.
		budget = 6
	)
	allocs := testing.AllocsPerRun(runs, func() {
		_ = r.Fix(f)
	})
	require.LessOrEqualf(t, allocs, float64(budget),
		"Fix allocs/op = %.0f (budget=%d); pre-size resultLines with make([][]byte, 0, cap)",
		allocs, budget)
}

func TestCheck_BothViolations(t *testing.T) {
	// Heading immediately before list (no blank), heading immediately after (no blank).
	src := []byte("## Title\n- item 1\n- item 2\n## After\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 2, "expected 2 diagnostics (before + after), got %d", len(diags))
}

// TestCheck_NilASTMatchesAST pins the nil-AST path: Check on a parse-
// skipped File (f.AST nil) must produce byte-identical diagnostics to the
// AST path, including nested lists, a loose list, and a list holding a
// code fence.
func TestCheck_NilASTMatchesAST(t *testing.T) {
	srcs := [][]byte{
		[]byte("# Title\n\nContent here.\n- item one\n- item two\n"),
		[]byte("# Title\n\n- a\n- b\n\nAfter.\n"),
		[]byte("Para.\n- a\n- b\nNext para.\n"),
		[]byte("- a\n  - nested\n- b\n\ntext\n"),
		[]byte("- a\n\n- b\n\nafter\n"),
		[]byte("- item\n  ```\n  code\n  ```\n- two\nafter\n"),
		[]byte("text\n\n- a\n- b\n"),
		[]byte("```\n- not a list, in code\n```\ntext\n"),
		[]byte("# H\n\n- only item\n"),
		[]byte("1. one\n2. two\nimmediately after\n"),
		[]byte("text\n-\n-\ntext\n"),
	}
	for _, src := range srcs {
		astFile, err := lint.NewFile("f.md", src)
		require.NoError(t, err)
		astDiags := (&Rule{}).Check(astFile)
		l0Diags := (&Rule{}).Check(lint.NewFileLines("f.md", src))
		assert.Equal(t, astDiags, l0Diags,
			"nil-AST diagnostics must match AST for %q", string(src))
	}
}
