package firstlineheading

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
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

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() == "" {
		t.Error("expected non-empty category")
	}
}

// =====================================================================
// Phase 5: additional branch coverage
// =====================================================================

// TestCheck_FirstChildNil exercises the `firstChild == nil` branch in Check.
// A source with only whitespace (no AST nodes) produces a nil first child.
func TestCheck_FirstChildNil(t *testing.T) {
	src := []byte(" ")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "whitespace-only source should trigger missing-heading diagnostic")
	assert.Equal(t, "first line should be a level 1 heading", diags[0].Message)
}

// TestHeadingLine_TextChild exercises the `for c := heading.FirstChild()` loop
// when Lines().Len() == 0 and the heading has a direct Text child.
func TestHeadingLine_TextChild(t *testing.T) {
	src := []byte("# Hello\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	// Build a heading without lines but with a Text child at offset 0.
	h := ast.NewHeading(1)
	textNode := ast.NewText()
	textNode.Segment = text.NewSegment(0, 7)
	h.AppendChild(h, textNode)

	line := headingLine(h, f)
	assert.Equal(t, 1, line)
}

// TestHeadingLine_FallbackReturn1 exercises the final `return 1` in headingLine
// when the heading has no lines and no Text children, and the source is empty.
func TestHeadingLine_FallbackReturn1(t *testing.T) {
	f := &lint.File{Path: "test.md", Source: []byte{}}
	h := ast.NewHeading(1)
	// No lines, no children, empty source → return 1
	line := headingLine(h, f)
	assert.Equal(t, 1, line)
}

// --- Placeholder tests ---

func TestCheck_PlaceholderHeadingQuestion_WrongLevel_Suppressed(t *testing.T) {
	// A heading-question placeholder at the wrong level should not be flagged
	// when heading-question is in the placeholders list.
	src := []byte("## ?\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1, Placeholders: []string{"heading-question"}}
	diags := r.Check(f)
	assert.Empty(t, diags, "heading-question placeholder should suppress level mismatch")
}

func TestCheck_PlaceholderHeadingQuestion_WrongLevel_EmptyPlaceholders(t *testing.T) {
	// Without placeholders configured, wrong-level heading is still flagged.
	src := []byte("## ?\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1, Placeholders: []string{}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "should flag wrong-level heading without placeholders configured")
	assert.Equal(t, "first heading should be level 1, got 2", diags[0].Message)
}

func TestCheck_PlaceholderVarToken_WrongLevel_Suppressed(t *testing.T) {
	// When var-token is configured and the heading text is a placeholder,
	// the level check is suppressed.
	src := []byte("## {title}\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1, Placeholders: []string{"var-token"}}
	diags := r.Check(f)
	assert.Empty(t, diags, "var-token placeholder heading should suppress level check")
}

func TestCheck_PlaceholderVarToken_WrongLevel_EmptyPlaceholders(t *testing.T) {
	// Without placeholders, var-token heading still fails the level check.
	src := []byte("## {title}\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1, Placeholders: []string{}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "should flag wrong-level heading without placeholders configured")
}

func TestApplySettings_Placeholders_Valid(t *testing.T) {
	r := &Rule{Level: 1}
	err := r.ApplySettings(map[string]any{
		"placeholders": []any{"heading-question", "var-token"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"heading-question", "var-token"}, r.Placeholders)
}

func TestApplySettings_Placeholders_UnknownToken(t *testing.T) {
	r := &Rule{Level: 1}
	err := r.ApplySettings(map[string]any{
		"placeholders": []any{"no-such-token"},
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "no-such-token")
}

func TestApplySettings_Placeholders_NonList_FirstLineHeading(t *testing.T) {
	r := &Rule{Level: 1}
	err := r.ApplySettings(map[string]any{"placeholders": "not-a-list"})
	require.Error(t, err)
	assert.ErrorContains(t, err, "list of strings")
}

func TestDefaultSettings_IncludesPlaceholders(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	assert.Equal(t, []string{}, ds["placeholders"])
}

func TestSettingMergeMode_FirstLineHeading(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("placeholders"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("level"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("unknown"))
}
