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

// --- Issue #320: emphasis inside a table cell is intentional inline styling ---

func TestCheck_EmphasisInsideTable_NotFlagged(t *testing.T) {
	// Bold text in a table cell — typically a row-label stub — is
	// intentional inline styling. MDS018 must defer to the table-format
	// rule rather than treat the cell as a stray heading.
	src := []byte("# Status\n\n" +
		"| Stub      | Value |\n" +
		"| --------- | ----- |\n" +
		"| **Bold**  | 1     |\n" +
		"| **Other** | 2     |\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Empty(t, diags, "emphasis in a table cell must not be flagged")
}

func TestCheck_TableShapedSingleEmphasisLine_NotFlagged(t *testing.T) {
	// A standalone "table-shaped" line (`|*x*|`) is parsed by the
	// default goldmark configuration as text-emphasis-text — three
	// children — which already escapes the lone-emphasis check. The
	// explicit IsTable guard documents the intent and stays defensive
	// against future GFM table parsing.
	src := []byte("# Heading\n\n|**Solo**|\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Empty(t, diags, "table-shaped emphasis line must not be flagged")
}

// TestInlineCapable_NoEmphasisAsHeading covers the InlineCapable method.
func TestInlineCapable_NoEmphasisAsHeading(t *testing.T) {
	r := &Rule{}
	assert.True(t, r.InlineCapable())
}

// TestSegmentsContainPlaceholder_Match covers the return-true branch when a
// segment accumulates to a known placeholder body token.
func TestSegmentsContainPlaceholder_Match(t *testing.T) {
	// "placeholder-section" matches strings.TrimSpace(text) == "..."
	got := segmentsContainPlaceholder([]string{"..."}, []string{"placeholder-section"})
	assert.True(t, got, "segmentsContainPlaceholder must return true for placeholder-section token")
}

// TestCheckFromInline_PlaceholderSuppressesAll covers the continue (line 69)
// and the len(diags)==0 early-return (line 81-83): a lone-emphasis paragraph
// whose content matches a placeholder token must produce zero diagnostics on
// the nil-AST path.
func TestCheckFromInline_PlaceholderSuppressesAll(t *testing.T) {
	// "..." with "placeholder-section" configured must be suppressed.
	src := []byte("*...*\n")
	lineFile := lint.NewFileLines("test.md", src)
	r := &Rule{Placeholders: []string{"placeholder-section"}}
	diags := r.Check(lineFile)
	assert.Empty(t, diags, "placeholder-suppressed inline emphasis must yield no diagnostics")
}

// TestCheck_NilASTEquivalence pins the parse-skipped path (f.AST nil,
// served from the Layer 1 whole-paragraph-emphasis index) byte-identical
// to the AST path across emphasis-as-heading shapes, including the
// placeholder suppression.
func TestCheck_NilASTEquivalence(t *testing.T) {
	cases := []struct {
		name         string
		src          string
		placeholders []string
	}{
		{"bold-heading", "**Bold text**\n", nil},
		{"italic-heading", "*Italic text*\n", nil},
		{"underscore-heading", "_Italic_\n", nil},
		{"inline-emphasis-ok", "Some **bold** text here.\n", nil},
		{"plain-ok", "just plain text\n", nil},
		{"two-paragraphs", "*one*\n\nplain\n\n**two**\n", nil},
		{"emphasis-then-text", "*emphasis* and more\n", nil},
		{"leading-spaces", "   *emphasis*\n", nil},
		{"multiline-emphasis", "*emphasis\ncontinued*\n", nil},
		{"placeholder-question", "*?*\n", []string{"?"}},
		{"placeholder-ellipsis", "*...*\n", []string{"..."}},
		{"placeholder-no-match", "*real heading*\n", []string{"?"}},
		{"blockquote-emph", "> *just emphasis*\n", nil},
		{"wrapped-emph-quote", "> *just\n> emph*\n", nil},
		{"multi-bq-emph", "> *a*\n>\n> *b*\n", nil},
		{"list-emph-not-flagged", "- *x*\n", nil},
		{"emph-then-html-block", "*emph*\n<div>x</div>\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			astFile, err := lint.NewFile("test.md", []byte(tc.src))
			require.NoError(t, err)
			lineFile := lint.NewFileLines("test.md", []byte(tc.src))
			r1 := &Rule{Placeholders: tc.placeholders}
			r2 := &Rule{Placeholders: tc.placeholders}
			assert.Equal(t, r1.Check(astFile), r2.Check(lineFile),
				"nil-AST diagnostics diverge from AST path")
		})
	}
}
