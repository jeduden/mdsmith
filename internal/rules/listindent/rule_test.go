package listindent

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/require"
)

func TestCheck_CorrectIndent2Spaces(t *testing.T) {
	src := []byte("- item 1\n  - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_WrongIndent4SpacesWhenExpecting2(t *testing.T) {
	src := []byte("- item 1\n    - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	if diags[0].RuleID != "MDS016" {
		t.Errorf("expected rule ID MDS016, got %s", diags[0].RuleID)
	}
}

func TestCheck_CorrectIndent4Spaces(t *testing.T) {
	src := []byte("- item 1\n    - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 4}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_DeeplyNested(t *testing.T) {
	src := []byte("- level 0\n  - level 1\n    - level 2\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	require.Len(t, diags, 0,
		"expected 0 diagnostics for correctly indented deep nesting, got %d: %+v", len(diags), diags)
}

func TestCheck_OrderedList(t *testing.T) {
	src := []byte("1. item 1\n   1. nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 3}
	diags := r.Check(f)
	require.Len(t, diags, 0,
		"expected 0 diagnostics for correctly indented ordered list, got %d: %+v", len(diags), diags)
}

func TestCheck_EmptyFile(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_FlatList(t *testing.T) {
	src := []byte("- item 1\n- item 2\n- item 3\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for flat list, got %d: %+v", len(diags), diags)
}

func TestFix_AdjustsIndentation(t *testing.T) {
	src := []byte("- item 1\n    - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	result := r.Fix(f)
	expected := "- item 1\n  - nested\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_NoChange(t *testing.T) {
	src := []byte("- item 1\n  - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("expected no change, got %q", string(result))
	}
}

// --- Configurable tests ---

func TestApplySettings_ValidSpaces(t *testing.T) {
	r := &Rule{Spaces: 2}
	if err := r.ApplySettings(map[string]any{"spaces": 4}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Spaces != 4 {
		t.Errorf("expected Spaces=4, got %d", r.Spaces)
	}
}

func TestApplySettings_InvalidSpacesType(t *testing.T) {
	r := &Rule{Spaces: 2}
	err := r.ApplySettings(map[string]any{"spaces": "not-a-number"})
	require.Error(t, err, "expected error for non-int spaces")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{Spaces: 2}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err, "expected error for unknown key")
}

func TestDefaultSettings_ListIndent(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["spaces"] != 2 {
		t.Errorf("expected spaces=2, got %v", ds["spaces"])
	}
}

func TestCheck_Spaces4_AllowsFourSpaceIndent(t *testing.T) {
	// With Spaces=4, four-space indent should be allowed.
	src := []byte("- item 1\n    - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 4}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics with Spaces=4 for 4-space indent, got %d", len(diags))
}

func TestCheck_Spaces4_FlagsTwoSpaceIndent(t *testing.T) {
	// With Spaces=4, two-space indent should be flagged.
	src := []byte("- item 1\n  - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 4}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic with Spaces=4 for 2-space indent, got %d", len(diags))
}
