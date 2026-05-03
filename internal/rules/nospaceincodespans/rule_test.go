package nospaceincodespans

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/yuin/goldmark/ast"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func TestRuleMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS052", r.ID())
	assert.Equal(t, "no-space-in-code-spans", r.Name())
	assert.Equal(t, "whitespace", r.Category())
	assert.False(t, r.EnabledByDefault())
}

func TestCheck_NoSpaces_NoDiagnostic(t *testing.T) {
	f := newFile(t, "Use `x` here.\n")
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_BalancedSingleSpace_NoDiagnostic(t *testing.T) {
	// CommonMark trims one space from each side when both sides have one.
	f := newFile(t, "Use ` x ` here.\n")
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_LeadingSpace(t *testing.T) {
	f := newFile(t, "Use ` x` here.\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, msgLeading, diags[0].Message)
}

func TestCheck_LeadingSpaceLongContent(t *testing.T) {
	f := newFile(t, "Use ` abc` here.\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, msgLeading, diags[0].Message)
}

func TestCheck_AsymmetricDoubleLeading_OnlyLeading(t *testing.T) {
	// `  x ` — CommonMark strips one space from each side (both sides have one),
	// leaving ` x` in the segment. Only the leading space is visible; no trailing.
	f := newFile(t, "Use `  x ` here.\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, msgLeading, diags[0].Message)
}

func TestCheck_AsymmetricDoubleTrailing_OnlyTrailing(t *testing.T) {
	// ` x  ` — CommonMark strips one from each side, leaving `x ` in the segment.
	// Only the trailing space is visible; no leading.
	f := newFile(t, "Use ` x  ` here.\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, msgTrailing, diags[0].Message)
}

func TestCheck_TrailingSpace(t *testing.T) {
	f := newFile(t, "Use `x ` here.\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, msgTrailing, diags[0].Message)
}

func TestCheck_DoubleSpaceBothSides(t *testing.T) {
	f := newFile(t, "Use `  x  ` here.\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 2)
	assert.Equal(t, msgLeading, diags[0].Message)
	assert.Equal(t, msgTrailing, diags[1].Message)
}

func TestCheck_LeadingTab(t *testing.T) {
	f := newFile(t, "Use `\tx` here.\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, msgLeading, diags[0].Message)
}

func TestCheck_LeadingNewline(t *testing.T) {
	// Newlines inside code spans are valid CommonMark; flag the boundary ws.
	f := newFile(t, "Use `\nx` here.\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, msgLeading, diags[0].Message)
}

func TestCheck_TrailingNewline(t *testing.T) {
	f := newFile(t, "Use `x\n` here.\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, msgTrailing, diags[0].Message)
}

func TestCheck_EmptyAfterTrim_BothDiagnostics(t *testing.T) {
	// "   " — all whitespace; both leading and trailing fire.
	f := newFile(t, "Use `   ` here.\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 2)
	assert.Equal(t, msgLeading, diags[0].Message)
	assert.Equal(t, msgTrailing, diags[1].Message)
}

func TestCheck_Position(t *testing.T) {
	f := newFile(t, "Start ` x` end.\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 7, diags[0].Column) // backtick is at column 7
}

func TestFix_LeadingSpace(t *testing.T) {
	f := newFile(t, "Use ` x` here.\n")
	got := string((&Rule{}).Fix(f))
	assert.Equal(t, "Use `x` here.\n", got)
}

func TestFix_LeadingSpaceLongContent(t *testing.T) {
	f := newFile(t, "Use ` abc` here.\n")
	got := string((&Rule{}).Fix(f))
	assert.Equal(t, "Use `abc` here.\n", got)
}

func TestFix_TrailingSpace(t *testing.T) {
	f := newFile(t, "Use `x ` here.\n")
	got := string((&Rule{}).Fix(f))
	assert.Equal(t, "Use `x` here.\n", got)
}

func TestFix_DoubleSpaceBothSides(t *testing.T) {
	f := newFile(t, "Use `  x  ` here.\n")
	got := string((&Rule{}).Fix(f))
	assert.Equal(t, "Use `x` here.\n", got)
}

func TestFix_EmptyAfterTrim_NoChange(t *testing.T) {
	src := "Use `   ` here.\n"
	f := newFile(t, src)
	got := string((&Rule{}).Fix(f))
	assert.Equal(t, src, got)
}

func TestFix_BalancedSingleSpace_NoChange(t *testing.T) {
	src := "Use ` x ` here.\n"
	f := newFile(t, src)
	got := string((&Rule{}).Fix(f))
	assert.Equal(t, src, got)
}

func TestFix_Multiple(t *testing.T) {
	f := newFile(t, "See ` a` and `b ` and `c`.\n")
	got := string((&Rule{}).Fix(f))
	assert.Equal(t, "See `a` and `b` and `c`.\n", got)
}

func TestFix_DoubleBracketCodeSpan(t *testing.T) {
	// Double-backtick delimiter preserving.
	f := newFile(t, "Use `` x `` here.\n")
	got := string((&Rule{}).Fix(f))
	// balanced single space — no change
	assert.Equal(t, "Use `` x `` here.\n", got)
}

func TestFix_DoubleBracketLeadingSpace(t *testing.T) {
	// "  x " has double leading space and single trailing; trim produces "x".
	f := newFile(t, "Use ``  x `` here.\n")
	got := string((&Rule{}).Fix(f))
	assert.Equal(t, "Use ``x`` here.\n", got)
}

func TestFix_LeadingNewline(t *testing.T) {
	f := newFile(t, "Use `\nx` here.\n")
	got := string((&Rule{}).Fix(f))
	assert.Equal(t, "Use `x` here.\n", got)
}

func TestFix_TrailingNewline(t *testing.T) {
	f := newFile(t, "Use `x\n` here.\n")
	got := string((&Rule{}).Fix(f))
	assert.Equal(t, "Use `x` here.\n", got)
}

// TestSpanBounds_NoTextChildren exercises the defensive ok=false path
// in spanBounds when a CodeSpan has no *ast.Text children.
func TestSpanBounds_NoTextChildren(t *testing.T) {
	cs := ast.NewCodeSpan()
	_, _, ok := spanBounds(cs)
	assert.False(t, ok)
}

// TestSpanBounds_NonTextChild covers the !ok2 continue branch in spanBounds
// when a child node is present but is not *ast.Text.
func TestSpanBounds_NonTextChild(t *testing.T) {
	cs := ast.NewCodeSpan()
	cs.AppendChild(cs, ast.NewCodeSpan()) // non-Text child
	_, _, ok := spanBounds(cs)
	assert.False(t, ok)
}

// TestFix_LeadingSpaceBeforeBacktick covers the bytes.Equal guard in Fix: when
// the trimmed+protective-space result equals the raw content, Fix leaves the
// source unchanged (emitting the diagnostic but not auto-fixing).
// Input: “ `abc“ — the content " `abc" has a leading space but trimming gives
// "`abc" whose protective prepend " `abc" equals the original raw, so no cut.
func TestFix_LeadingSpaceBeforeBacktick(t *testing.T) {
	src := "Use `` `abc`` here.\n"
	f := newFile(t, src)
	got := string((&Rule{}).Fix(f))
	assert.Equal(t, src, got)
}

// TestCheck_LeadingSpaceBeforeBacktick verifies Check still emits a diagnostic
// for the same span that Fix cannot safely auto-fix.
func TestCheck_LeadingSpaceBeforeBacktick(t *testing.T) {
	f := newFile(t, "Use `` `abc`` here.\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, msgLeading, diags[0].Message)
}

// TestFix_ContentBacktick_ProtectiveSpace verifies that trimming a span whose
// content starts or ends with a backtick adds a protective space to prevent
// the content backtick from merging into the delimiter run.
func TestFix_ContentBacktick_ProtectiveSpace(t *testing.T) {
	// Content is " `x` " inside double-backtick delimiters.
	// Trimming naively would give `x` which merges with `` into ```x``` (wrong).
	// The fix must produce ` `x` ` (one protective space each side).
	f := newFile(t, "Use ``  `x`  `` here.\n")
	got := string((&Rule{}).Fix(f))
	assert.Equal(t, "Use `` `x` `` here.\n", got)
}
