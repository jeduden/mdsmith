package emphasisstyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func newRule(bold, italic string, forbid bool) *Rule {
	return &Rule{Bold: bold, Italic: italic, ForbidMixedNesting: forbid}
}

// --- disabled / unconfigured ------------------------------------------------

func TestNoSettings_NoCheck(t *testing.T) {
	r := &Rule{}
	f := parseFile(t, "# Heading\n\n__bold__ and _italic_\n")
	assert.Empty(t, r.Check(f))
}

// --- bold -------------------------------------------------------------------

func TestBoldAsterisk_Asterisk_OK(t *testing.T) {
	r := newRule("asterisk", "", false)
	f := parseFile(t, "# Heading\n\nSome **bold** text.\n")
	assert.Empty(t, r.Check(f))
}

func TestBoldAsterisk_Underscore_Diag(t *testing.T) {
	r := newRule("asterisk", "", false)
	f := parseFile(t, "# Heading\n\nSome __bold__ text.\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "bold uses underscore; configured style is asterisk", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
}

func TestBoldUnderscore_Underscore_OK(t *testing.T) {
	r := newRule("underscore", "", false)
	f := parseFile(t, "# Heading\n\n__bold__\n")
	assert.Empty(t, r.Check(f))
}

func TestBoldUnderscore_Asterisk_Diag(t *testing.T) {
	r := newRule("underscore", "", false)
	f := parseFile(t, "# Heading\n\n**bold**\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "bold uses asterisk; configured style is underscore", diags[0].Message)
}

// --- italic -----------------------------------------------------------------

func TestItalicUnderscore_Underscore_OK(t *testing.T) {
	r := newRule("", "underscore", false)
	f := parseFile(t, "# Heading\n\n_italic_\n")
	assert.Empty(t, r.Check(f))
}

func TestItalicUnderscore_Asterisk_Diag(t *testing.T) {
	r := newRule("", "underscore", false)
	f := parseFile(t, "# Heading\n\n*italic*\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "italic uses asterisk; configured style is underscore", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
}

func TestItalicAsterisk_Asterisk_OK(t *testing.T) {
	r := newRule("", "asterisk", false)
	f := parseFile(t, "# Heading\n\n*italic*\n")
	assert.Empty(t, r.Check(f))
}

func TestItalicAsterisk_Underscore_Diag(t *testing.T) {
	r := newRule("", "asterisk", false)
	f := parseFile(t, "# Heading\n\n_italic_\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "italic uses underscore; configured style is asterisk", diags[0].Message)
}

// --- mixed nesting ----------------------------------------------------------

func TestMixedNesting_UnderscoreWrapsAsterisk_Diag(t *testing.T) {
	r := newRule("", "", true)
	f := parseFile(t, "# Heading\n\n_*x*_\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "mixed emphasis delimiters: underscore wraps asterisk", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
}

func TestMixedNesting_AsteriskWrapsUnderscore_Diag(t *testing.T) {
	r := newRule("", "", true)
	f := parseFile(t, "# Heading\n\n*_x_*\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "mixed emphasis delimiters: asterisk wraps underscore", diags[0].Message)
}

func TestMixedNesting_SameDelimiter_NoDiag(t *testing.T) {
	r := newRule("", "", true)
	f := parseFile(t, "# Heading\n\n*italic* and _italic_\n")
	assert.Empty(t, r.Check(f))
}

func TestMixedNesting_Disabled_NoDiag(t *testing.T) {
	r := newRule("", "", false)
	f := parseFile(t, "# Heading\n\n_*x*_\n")
	assert.Empty(t, r.Check(f))
}

// --- triple-delimiter runs --------------------------------------------------

func TestTripleAsterisk_ItalicUnderscore_DiagNoFix(t *testing.T) {
	r := newRule("asterisk", "underscore", false)
	f := parseFile(t, "# Heading\n\n***x***\n")
	diags := r.Check(f)
	// outer italic(level=1) uses asterisk but expected underscore → diagnostic
	require.NotEmpty(t, diags)
	hasItalic := false
	for _, d := range diags {
		if d.Message == "italic uses asterisk; configured style is underscore" {
			hasItalic = true
		}
	}
	assert.True(t, hasItalic, "expected italic diagnostic for triple-asterisk run")
	// Fix must not change the source (triple-run is skipped)
	assert.Equal(t, string(f.Source), string(r.Fix(f)))
}

func TestTripleUnderscore_DiagNoFix(t *testing.T) {
	r := newRule("asterisk", "asterisk", false)
	f := parseFile(t, "# Heading\n\n___x___\n")
	// both levels use underscore but config expects asterisk
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	// Fix must not change the source
	assert.Equal(t, string(f.Source), string(r.Fix(f)))
}

// --- code span / fenced code (must not flag) --------------------------------

func TestCodeSpan_NoFlag(t *testing.T) {
	r := newRule("asterisk", "underscore", true)
	f := parseFile(t, "# Heading\n\n`*italic*` and `__bold__`\n")
	assert.Empty(t, r.Check(f))
}

func TestFencedCode_NoFlag(t *testing.T) {
	r := newRule("asterisk", "underscore", true)
	src := "# Heading\n\n```\n__bold__ *italic*\n```\n"
	f := parseFile(t, src)
	assert.Empty(t, r.Check(f))
}

// --- Fix -------------------------------------------------------------------

func TestFix_UnderscoreBold_ToAsterisk(t *testing.T) {
	r := newRule("asterisk", "", false)
	f := parseFile(t, "# Heading\n\nSome __bold__ text.\n")
	require.Len(t, r.Check(f), 1)
	got := string(r.Fix(f))
	assert.Equal(t, "# Heading\n\nSome **bold** text.\n", got)
}

func TestFix_AsteriskItalic_ToUnderscore(t *testing.T) {
	r := newRule("", "underscore", false)
	f := parseFile(t, "# Heading\n\nSome *italic* text.\n")
	require.Len(t, r.Check(f), 1)
	got := string(r.Fix(f))
	assert.Equal(t, "# Heading\n\nSome _italic_ text.\n", got)
}

func TestFix_Multiple(t *testing.T) {
	r := newRule("asterisk", "underscore", false)
	f := parseFile(t, "# Heading\n\n__bold__ and *italic*\n")
	require.Len(t, r.Check(f), 2)
	got := string(r.Fix(f))
	assert.Equal(t, "# Heading\n\n**bold** and _italic_\n", got)
}

func TestFix_NoViolations_Unchanged(t *testing.T) {
	r := newRule("asterisk", "underscore", false)
	f := parseFile(t, "# Heading\n\n**bold** and _italic_\n")
	assert.Empty(t, r.Check(f))
	assert.Equal(t, string(f.Source), string(r.Fix(f)))
}

// --- ApplySettings ----------------------------------------------------------

func TestApplySettings_Valid(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"bold":                 "asterisk",
		"italic":               "underscore",
		"forbid-mixed-nesting": true,
	}))
	assert.Equal(t, "asterisk", r.Bold)
	assert.Equal(t, "underscore", r.Italic)
	assert.True(t, r.ForbidMixedNesting)
}

func TestApplySettings_InvalidBold(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"bold": "wrong"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid bold")
}

func TestApplySettings_InvalidItalic(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"italic": 42})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "italic must be a string")
}

func TestApplySettings_InvalidForbid(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"forbid-mixed-nesting": "yes"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a bool")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	d := r.DefaultSettings()
	assert.Equal(t, "", d["bold"])
	assert.Equal(t, "", d["italic"])
	assert.Equal(t, false, d["forbid-mixed-nesting"])
}

func TestEnabledByDefault(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault())
}
