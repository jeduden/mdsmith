package nospaceincodespans

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func check(t *testing.T, src string) []lint.Diagnostic {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return (&Rule{}).Check(f)
}

func fix(t *testing.T, src string) string {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return string((&Rule{}).Fix(f))
}

func TestMetaInformation(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS049", r.ID())
	assert.Equal(t, "no-space-in-code-spans", r.Name())
	assert.Equal(t, "whitespace", r.Category())
	assert.False(t, r.EnabledByDefault())
}

func TestImplementsDefaultable(t *testing.T) {
	var _ rule.Defaultable = (*Rule)(nil)
}

func TestCheck_NoSpace_NoDiagnostic(t *testing.T) {
	diags := check(t, "Use `x` here.\n")
	assert.Empty(t, diags)
}

func TestCheck_BalancedSingleSpace_NoDiagnostic(t *testing.T) {
	diags := check(t, "Use ` x ` here.\n")
	assert.Empty(t, diags)
}

func TestCheck_LeadingSpace_OneDiagnostic(t *testing.T) {
	diags := check(t, "Use ` x` here.\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS049", diags[0].RuleID)
	assert.Equal(t, "no-space-in-code-spans", diags[0].RuleName)
	assert.Equal(t, "code span has leading whitespace", diags[0].Message)
}

func TestCheck_TrailingSpace_OneDiagnostic(t *testing.T) {
	diags := check(t, "Use `x ` here.\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "code span has trailing whitespace", diags[0].Message)
}

func TestCheck_DoubleSpaceBothSides_TwoDiagnostics(t *testing.T) {
	diags := check(t, "Use `  x  ` here.\n")
	require.Len(t, diags, 2)
	assert.Equal(t, "code span has leading whitespace", diags[0].Message)
	assert.Equal(t, "code span has trailing whitespace", diags[1].Message)
}

func TestCheck_LeadingTab_OneDiagnostic(t *testing.T) {
	diags := check(t, "Use `\tx` here.\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "code span has leading whitespace", diags[0].Message)
}

func TestCheck_EmptyAfterTrim_TwoDiagnostics(t *testing.T) {
	diags := check(t, "Use `   ` here.\n")
	require.Len(t, diags, 2)
	assert.Equal(t, "code span has leading whitespace", diags[0].Message)
	assert.Equal(t, "code span has trailing whitespace", diags[1].Message)
}

func TestFix_LeadingSpace(t *testing.T) {
	result := fix(t, "Use ` x` here.\n")
	assert.Equal(t, "Use `x` here.\n", result)
}

func TestFix_TrailingSpace(t *testing.T) {
	result := fix(t, "Use `x ` here.\n")
	assert.Equal(t, "Use `x` here.\n", result)
}

func TestFix_DoubleSpaceBothSides(t *testing.T) {
	result := fix(t, "Use `  x  ` here.\n")
	assert.Equal(t, "Use `x` here.\n", result)
}

func TestFix_EmptyAfterTrim_NoFix(t *testing.T) {
	src := "Use `   ` here.\n"
	result := fix(t, src)
	assert.Equal(t, src, result, "empty-after-trim span must not be auto-fixed")
}

func TestFix_NoChange_WhenClean(t *testing.T) {
	src := "Use `x` here.\n"
	result := fix(t, src)
	assert.Equal(t, src, result)
}

func TestFix_BalancedSingleSpace_NoChange(t *testing.T) {
	src := "Use ` x ` here.\n"
	result := fix(t, src)
	assert.Equal(t, src, result)
}

func TestFix_PreservesDelimiterCount(t *testing.T) {
	result := fix(t, "Use `` x `` here.\n")
	assert.Equal(t, "Use `` x `` here.\n", result, "balanced single space with double backticks should not change")
}

func TestFix_DoubleBacktick_LeadingSpace(t *testing.T) {
	result := fix(t, "Use ``  x`` here.\n")
	assert.Equal(t, "Use ``x`` here.\n", result)
}

func TestCheck_MultipleSpans_Independent(t *testing.T) {
	diags := check(t, "Use ` x` and `y ` here.\n")
	require.Len(t, diags, 2)
}

func TestCheck_LineNumber(t *testing.T) {
	src := "# Title\n\nUse ` x` here.\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
}
