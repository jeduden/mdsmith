package firstlineheading

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/require"
)

func TestCheck_FirstLineH1_NoViolation(t *testing.T) {
	src := []byte("# Title\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_SetextHeading_NoViolation(t *testing.T) {
	src := []byte("Title\n=====\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 0, "setext heading on line 1 should pass, got %d: %+v", len(diags), diags)
}

func TestCheck_EmphasisHeading_NoViolation(t *testing.T) {
	src := []byte("# *Title*\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 0, "heading with emphasis on line 1 should pass, got %d: %+v", len(diags), diags)
}

func TestCheck_LinkHeading_NoViolation(t *testing.T) {
	src := []byte("# [link](url)\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 0, "heading with link on line 1 should pass, got %d: %+v", len(diags), diags)
}

func TestCheck_BlankLineSetextHeading(t *testing.T) {
	src := []byte("\nTitle\n=====\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "setext heading after blank line should fail, got %d: %+v", len(diags), diags)
	require.Equal(t, "first line should be a level 1 heading, found blank line", diags[0].Message)
}

func TestCheck_EmptyFile(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].RuleID != "MDS004" {
		t.Errorf("expected rule ID MDS004, got %s", diags[0].RuleID)
	}
}

func TestCheck_StartsWithParagraph(t *testing.T) {
	src := []byte("Some text\n\n# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	require.Equal(t, "first line should be a level 1 heading", diags[0].Message)
}

func TestCheck_BlankLineThenHeading(t *testing.T) {
	src := []byte("\n# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic for heading not on line 1, got %d: %+v", len(diags), diags)
	require.Equal(t, "first line should be a level 1 heading, found blank line", diags[0].Message)
}

func TestCheck_MultipleBlankLinesThenHeading(t *testing.T) {
	src := []byte("\n\n\n# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	require.Equal(t, "first line should be a level 1 heading, found blank line", diags[0].Message)
}

func TestCheck_WhitespaceBlankLineThenEmptyHeading(t *testing.T) {
	src := []byte("   \n# \n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1,
		"whitespace-only blank line before empty heading should trigger, got %d: %+v",
		len(diags), diags)
	require.Equal(t, "first line should be a level 1 heading, found blank line", diags[0].Message)
}

func TestCheck_BlankLineThenEmptyHeading(t *testing.T) {
	src := []byte("\n# \n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic for empty heading after blank line, got %d: %+v", len(diags), diags)
	require.Equal(t, "first line should be a level 1 heading, found blank line", diags[0].Message)
}

func TestCheck_EmptyHeadingOnLine1(t *testing.T) {
	src := []byte("# \n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 0, "empty heading on line 1 should not trigger diagnostic, got %d: %+v", len(diags), diags)
}

func TestCheck_LevelZeroDefault(t *testing.T) {
	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 0}
	diags := r.Check(f)
	require.Len(t, diags, 0, "Level 0 should default to 1; expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_WrongLevel(t *testing.T) {
	src := []byte("## Title\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	require.Equal(t, "first heading should be level 1, got 2", diags[0].Message)
}

func TestCheck_Level2Config(t *testing.T) {
	src := []byte("## Title\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 2}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS004" {
		t.Errorf("expected MDS004, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "first-line-heading" {
		t.Errorf("expected first-line-heading, got %s", r.Name())
	}
}

// --- Configurable tests ---

func TestApplySettings_ValidLevel(t *testing.T) {
	r := &Rule{Level: 1}
	if err := r.ApplySettings(map[string]any{"level": 2}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Level != 2 {
		t.Errorf("expected Level=2, got %d", r.Level)
	}
}

func TestApplySettings_InvalidLevelType(t *testing.T) {
	r := &Rule{Level: 1}
	err := r.ApplySettings(map[string]any{"level": "not-a-number"})
	require.Error(t, err, "expected error for non-int level")
}

func TestApplySettings_LevelOutOfRange(t *testing.T) {
	r := &Rule{Level: 1}
	err := r.ApplySettings(map[string]any{"level": 7})
	require.Error(t, err, "expected error for level > 6")
}

func TestApplySettings_LevelZero(t *testing.T) {
	r := &Rule{Level: 1}
	err := r.ApplySettings(map[string]any{"level": 0})
	require.Error(t, err, "expected error for level 0")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{Level: 1}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err, "expected error for unknown key")
}

func TestDefaultSettings_FirstLineHeading(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["level"] != 1 {
		t.Errorf("expected level=1, got %v", ds["level"])
	}
}
