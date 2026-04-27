package noemphasisasheading

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck_BoldParagraph_Violation(t *testing.T) {
	src := []byte("**Bold text**\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	if diags[0].RuleID != "MDS018" {
		t.Errorf("expected rule ID MDS018, got %s", diags[0].RuleID)
	}
	if diags[0].Message != "emphasis used instead of a heading" {
		t.Errorf("unexpected message: %s", diags[0].Message)
	}
}

func TestCheck_ItalicParagraph_Violation(t *testing.T) {
	src := []byte("*Italic text*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
}

func TestCheck_InlineEmphasis_NoViolation(t *testing.T) {
	src := []byte("Some **bold** text in a paragraph.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_NormalParagraph_NoViolation(t *testing.T) {
	src := []byte("Just normal text.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_Heading_NoViolation(t *testing.T) {
	src := []byte("# Real Heading\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_EmptyFile(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS018" {
		t.Errorf("expected MDS018, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "no-emphasis-as-heading" {
		t.Errorf("expected no-emphasis-as-heading, got %s", r.Name())
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() == "" {
		t.Error("expected non-empty category")
	}
}

// --- Placeholder tests ---

func TestCheck_Placeholder_VarTokenInEmphasis_Suppressed(t *testing.T) {
	// Emphasis wrapping a var-token placeholder should not be flagged
	// when var-token is configured.
	src := []byte("# Title\n\n*{title}*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Placeholders: []string{"var-token"}}
	diags := r.Check(f)
	require.Empty(t, diags, "var-token in emphasis should suppress diagnostic")
}

func TestCheck_Placeholder_VarTokenInEmphasis_EmptyList(t *testing.T) {
	// Without placeholders configured, emphasis with var-token is still flagged.
	src := []byte("# Title\n\n*{title}*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Placeholders: []string{}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "should flag emphasis-as-heading without placeholders configured")
}

func TestCheck_Placeholder_NoMatch_StillFlags(t *testing.T) {
	// Emphasis whose text does not match any configured placeholder is still flagged.
	// This also exercises the !entering branch of the inner AST walk.
	src := []byte("# Title\n\n*plain text*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Placeholders: []string{"var-token"}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "emphasis-as-heading with non-matching placeholder should still be flagged")
}

func TestApplySettings_Placeholders_NonList_NoEmphasisAsHeading(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"placeholders": "not-a-list"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "list of strings")
}

func TestApplySettings_UnknownKey_NoEmphasisAsHeading(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknownkey": true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown setting")
}

func TestApplySettings_Placeholders_NoEmphasisAsHeading(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"placeholders": []any{"var-token"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"var-token"}, r.Placeholders)
}

func TestApplySettings_Placeholders_UnknownToken_NoEmphasisAsHeading(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"placeholders": []any{"bad"}})
	require.Error(t, err)
}

func TestDefaultSettings_NoEmphasisAsHeading(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	require.Equal(t, []string{}, ds["placeholders"])
}

func TestSettingMergeMode_NoEmphasisAsHeading(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("placeholders"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("unknown"))
}
