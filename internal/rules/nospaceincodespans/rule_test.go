package nospaceincodespans

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to build a lint.File from inline markdown source.
func mustFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

// TestCheck_NoSpan verifies a plain paragraph (no code spans) emits no diagnostics.
func TestCheck_NoSpan(t *testing.T) {
	f := mustFile(t, "# Title\n\nHello world.\n")
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

// TestCheck_CleanCodeSpan verifies `x` emits no diagnostic.
func TestCheck_CleanCodeSpan(t *testing.T) {
	f := mustFile(t, "# Title\n\nUse `x` here.\n")
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags, "clean code span must not emit diagnostics")
}

// TestCheck_BalancedSingleSpace verifies ` x ` (CommonMark trim case) emits no diagnostic.
func TestCheck_BalancedSingleSpace(t *testing.T) {
	f := mustFile(t, "# Title\n\nUse ` x ` here.\n")
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags, "balanced single-space code span must not emit diagnostics")
}

// TestCheck_LeadingSpaceOnly verifies ` x` emits a leading-whitespace diagnostic.
func TestCheck_LeadingSpaceOnly(t *testing.T) {
	f := mustFile(t, "# Title\n\nUse ` x` here.\n")
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS049", diags[0].RuleID)
	assert.Equal(t, "code span has leading whitespace", diags[0].Message)
}

// TestCheck_TrailingSpaceOnly verifies `x ` emits a trailing-whitespace diagnostic.
func TestCheck_TrailingSpaceOnly(t *testing.T) {
	f := mustFile(t, "# Title\n\nUse `x ` here.\n")
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS049", diags[0].RuleID)
	assert.Equal(t, "code span has trailing whitespace", diags[0].Message)
}

// TestCheck_BothSidesDoubleSpace verifies `  x  ` emits both diagnostics.
func TestCheck_BothSidesDoubleSpace(t *testing.T) {
	f := mustFile(t, "# Title\n\nUse `  x  ` here.\n")
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 2)
	messages := []string{diags[0].Message, diags[1].Message}
	assert.Contains(t, messages, "code span has leading whitespace")
	assert.Contains(t, messages, "code span has trailing whitespace")
}

// TestCheck_LeadingTab verifies `\tx` (leading tab) emits a leading-whitespace diagnostic.
func TestCheck_LeadingTab(t *testing.T) {
	f := mustFile(t, "# Title\n\nUse `\tx` here.\n")
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "code span has leading whitespace", diags[0].Message)
}

// TestCheck_EmptyAfterTrim verifies `   ` (all spaces) emits diagnostics but is not fixed.
func TestCheck_EmptyAfterTrimDiags(t *testing.T) {
	f := mustFile(t, "# Title\n\nUse `   ` here.\n")
	r := &Rule{}
	diags := r.Check(f)
	// Should emit both leading and trailing diagnostics.
	require.NotEmpty(t, diags, "all-space code span must emit diagnostics")
}

// TestCheck_RuleIDAndName verifies the rule has correct metadata.
func TestCheck_RuleIDAndName(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS049", r.ID())
	assert.Equal(t, "no-space-in-code-spans", r.Name())
	assert.Equal(t, "whitespace", r.Category())
}

// TestDefaultable verifies the rule is opt-in (disabled by default).
func TestDefaultable(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault())
}

// TestFix_LeadingSpace verifies ` x` fixes to `x`.
func TestFix_LeadingSpace(t *testing.T) {
	src := "# Title\n\nUse ` x` here.\n"
	f := mustFile(t, src)
	r := &Rule{}
	result := r.Fix(f)
	assert.Equal(t, "# Title\n\nUse `x` here.\n", string(result))
}

// TestFix_TrailingSpace verifies `x ` fixes to `x`.
func TestFix_TrailingSpace(t *testing.T) {
	src := "# Title\n\nUse `x ` here.\n"
	f := mustFile(t, src)
	r := &Rule{}
	result := r.Fix(f)
	assert.Equal(t, "# Title\n\nUse `x` here.\n", string(result))
}

// TestFix_BothSidesDoubleSpace verifies `  x  ` fixes to `x`.
func TestFix_BothSidesDoubleSpace(t *testing.T) {
	src := "# Title\n\nUse `  x  ` here.\n"
	f := mustFile(t, src)
	r := &Rule{}
	result := r.Fix(f)
	assert.Equal(t, "# Title\n\nUse `x` here.\n", string(result))
}

// TestFix_EmptyAfterTrim verifies `   ` (all spaces) is NOT fixed.
func TestFix_EmptyAfterTrim(t *testing.T) {
	src := "# Title\n\nUse `   ` here.\n"
	f := mustFile(t, src)
	r := &Rule{}
	result := r.Fix(f)
	// Source must be unchanged when trimmed body would be empty.
	assert.Equal(t, src, string(result))
}

// TestFix_NoChange verifies clean code spans are left unchanged.
func TestFix_NoChange(t *testing.T) {
	src := "# Title\n\nUse `x` here.\n"
	f := mustFile(t, src)
	r := &Rule{}
	result := r.Fix(f)
	assert.Equal(t, src, string(result))
}

// TestFix_BalancedSingleSpace verifies ` x ` (legal) is left unchanged.
func TestFix_BalancedSingleSpace(t *testing.T) {
	src := "# Title\n\nUse ` x ` here.\n"
	f := mustFile(t, src)
	r := &Rule{}
	result := r.Fix(f)
	assert.Equal(t, src, string(result))
}

// TestFix_PreservesDelimiterCount verifies that multi-backtick delimiters are preserved.
func TestFix_PreservesDelimiterCount(t *testing.T) {
	// Double-backtick delimiter: `` ` x `` ``  (one backtick, space, x, space, two backticks)
	// Source: "`` ` x ``" - double backtick delimiter, space before and after content
	// Actually: `` `  x  `` `` - has leading+trailing space inside double-backtick span
	src := "# Title\n\nUse ``  x  `` here.\n"
	f := mustFile(t, src)
	r := &Rule{}
	result := r.Fix(f)
	assert.Equal(t, "# Title\n\nUse ``x`` here.\n", string(result))
}
