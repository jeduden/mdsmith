package headingstyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/require"
)

func TestCheck_ATXStyle_NoViolation(t *testing.T) {
	src := []byte("# Heading 1\n\n## Heading 2\n\n### Heading 3\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "atx"}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_ATXStyle_SetextViolation(t *testing.T) {
	src := []byte("Heading 1\n=========\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "atx"}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	if diags[0].RuleID != "MDS002" {
		t.Errorf("expected rule ID MDS002, got %s", diags[0].RuleID)
	}
	if diags[0].Line != 1 {
		t.Errorf("expected line 1, got %d", diags[0].Line)
	}
}

func TestCheck_SetextStyle_ATXViolation(t *testing.T) {
	src := []byte("# Heading 1\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "setext"}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	if diags[0].Message != "heading style should be setext" {
		t.Errorf("unexpected message: %s", diags[0].Message)
	}
}

func TestCheck_SetextStyle_Level3ATX_NoViolation(t *testing.T) {
	// Setext only supports levels 1-2, so level 3+ ATX is fine
	src := []byte("### Heading 3\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "setext"}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_SetextStyle_NoViolation(t *testing.T) {
	src := []byte("Heading 1\n=========\n\nHeading 2\n---------\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "setext"}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestFix_SetextToATX(t *testing.T) {
	src := []byte("Heading 1\n=========\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "atx"}
	result := r.Fix(f)
	expected := "# Heading 1\n\nSome text\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_ATXToSetext(t *testing.T) {
	src := []byte("# Heading 1\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "setext"}
	result := r.Fix(f)
	expected := "Heading 1\n=========\n\nSome text\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS002" {
		t.Errorf("expected MDS002, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "heading-style" {
		t.Errorf("expected heading-style, got %s", r.Name())
	}
}

// --- Configurable tests ---

func TestApplySettings_ValidStyle(t *testing.T) {
	r := &Rule{Style: "atx"}
	if err := r.ApplySettings(map[string]any{"style": "setext"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Style != "setext" {
		t.Errorf("expected Style=setext, got %s", r.Style)
	}
}

func TestApplySettings_InvalidStyle(t *testing.T) {
	r := &Rule{Style: "atx"}
	err := r.ApplySettings(map[string]any{"style": "invalid"})
	require.Error(t, err, "expected error for invalid style")
}

func TestApplySettings_InvalidStyleType(t *testing.T) {
	r := &Rule{Style: "atx"}
	err := r.ApplySettings(map[string]any{"style": 42})
	require.Error(t, err, "expected error for non-string style")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{Style: "atx"}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err, "expected error for unknown key")
}

func TestDefaultSettings_HeadingStyle(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["style"] != "atx" {
		t.Errorf("expected style=atx, got %v", ds["style"])
	}
}
