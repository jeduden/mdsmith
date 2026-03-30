package directorystructure

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRule(t *testing.T, allowed []string) *Rule {
	t.Helper()
	r := &Rule{}
	settings := map[string]any{"allowed": allowed}
	require.NoError(t, r.ApplySettings(settings), "newRule")
	return r
}

func TestCheck_AllowedDirectory_NoViolation(t *testing.T) {
	r := newRule(t, []string{"docs/**"})
	src := []byte("# Title\n")
	f, err := lint.NewFile("docs/guide.md", src)
	require.NoError(t, err)
	diags := r.Check(f)
	assert.Empty(t, diags, "expected 0 diagnostics")
}

func TestCheck_DisallowedDirectory_Violation(t *testing.T) {
	r := newRule(t, []string{"docs/**"})
	src := []byte("# Title\n")
	f, err := lint.NewFile("src/notes.md", src)
	require.NoError(t, err)
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic")
	assert.Equal(t, "MDS033", diags[0].RuleID, "expected rule ID MDS033")
	assert.Equal(t, lint.Warning, diags[0].Severity, "expected warning severity")
}

func TestCheck_RootFile_WithDotPattern(t *testing.T) {
	r := newRule(t, []string{"."})
	src := []byte("# README\n")
	f, err := lint.NewFile("README.md", src)
	require.NoError(t, err)
	diags := r.Check(f)
	assert.Empty(t, diags, "expected 0 diagnostics for root file with '.' pattern")
}

func TestCheck_RootFile_Disallowed(t *testing.T) {
	r := newRule(t, []string{"docs/**"})
	src := []byte("# README\n")
	f, err := lint.NewFile("README.md", src)
	require.NoError(t, err)
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic")
}

func TestCheck_NestedGlob(t *testing.T) {
	r := newRule(t, []string{"internal/**/testdata/**"})
	src := []byte("# Test\n")
	f, err := lint.NewFile("internal/rules/foo/testdata/good/test.md", src)
	require.NoError(t, err)
	diags := r.Check(f)
	assert.Empty(t, diags, "expected 0 diagnostics")
}

func TestCheck_Unconfigured_NoOp(t *testing.T) {
	r := &Rule{}
	src := []byte("# Title\n")
	f, err := lint.NewFile("docs/guide.md", src)
	require.NoError(t, err)
	diags := r.Check(f)
	assert.Empty(t, diags, "expected 0 diagnostics (ApplySettings never called)")
}

func TestCheck_EmptyAllowed_WarnsOnEveryFile(t *testing.T) {
	r := newRule(t, []string{})
	src := []byte("# Title\n")
	f, err := lint.NewFile("docs/guide.md", src)
	require.NoError(t, err)
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic (config warning)")
	assert.Contains(t, diags[0].Message, "no \"allowed\" patterns configured")
}

func TestCheck_MultiplePatterns(t *testing.T) {
	r := newRule(t, []string{"docs/**", "plan/**", "."})
	tests := []struct {
		path  string
		wantN int
	}{
		{"docs/guide.md", 0},
		{"plan/roadmap.md", 0},
		{"README.md", 0},
		{"src/notes.md", 1},
	}
	for _, tt := range tests {
		f, err := lint.NewFile(tt.path, []byte("# Title\n"))
		require.NoError(t, err)
		diags := r.Check(f)
		assert.Len(t, diags, tt.wantN, "path %q: expected %d diagnostics", tt.path, tt.wantN)
	}
}

func TestEnabledByDefault(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault(), "directory-structure should be disabled by default")
}

func TestID(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS033", r.ID(), "expected MDS033")
}

func TestName(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "directory-structure", r.Name(), "expected directory-structure")
}

func TestApplySettings(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"allowed": []any{"docs/**", "plan/**"},
	})
	require.NoError(t, err)
	require.Len(t, r.Allowed, 2, "expected 2 allowed patterns")
	assert.Equal(t, "docs/**", r.Allowed[0], "expected docs/**")
}

func TestApplySettings_StringSlice(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"allowed": []string{"docs/**"},
	})
	require.NoError(t, err)
	require.Len(t, r.Allowed, 1, "expected 1 allowed pattern")
}

func TestApplySettings_InvalidGlob(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"allowed": []any{"[invalid"},
	})
	assert.Error(t, err, "expected error for invalid glob pattern")
	// Rule must remain unconfigured after error.
	assert.False(t, r.configured, "rule should remain unconfigured after invalid glob error")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"bogus": true})
	assert.Error(t, err, "expected error for unknown setting")
	// Rule must remain unconfigured after error.
	assert.False(t, r.configured, "rule should remain unconfigured after unknown key error")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	s := r.DefaultSettings()
	_, ok := s["allowed"]
	assert.False(t, ok, "default settings should not include 'allowed' key (rule stays unconfigured/no-op)")
}

func TestApplyDefaultSettings_EmitsConfigWarning(t *testing.T) {
	// Simulate the CloneRule/fixture-harness flow: configure the rule,
	// then restore defaults. After applying defaults, the rule is still
	// configured but has no "allowed" patterns and should emit a config
	// warning when checked.
	r := newRule(t, []string{"docs/**"})
	err := r.ApplySettings(r.DefaultSettings())
	require.NoError(t, err)
	src := []byte("# Title\n")
	f, err := lint.NewFile("anywhere/guide.md", src)
	require.NoError(t, err)
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic (config warning)")
	assert.Contains(t, diags[0].Message, "no \"allowed\" patterns configured")
}
