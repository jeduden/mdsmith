package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckRules_BasicDiagnostics(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	require.NoError(t, err)

	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: true},
	}
	rules := []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	require.Len(t, errs, 0, "unexpected errors: %v", errs)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	assert.Equal(t, "MDS999", diags[0].RuleID)
}

func TestCheckRules_DisabledRuleSkipped(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	require.NoError(t, err)

	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: false},
	}
	rules := []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	require.Len(t, errs, 0, "unexpected errors: %v", errs)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheckRules_UnconfiguredRuleSkipped(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	require.NoError(t, err)

	effective := map[string]config.RuleCfg{}
	rules := []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	require.Len(t, errs, 0, "unexpected errors: %v", errs)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheckRules_AppliesSettings(t *testing.T) {
	// 100-char line with max=120 should not trigger.
	line := strings.Repeat("a", 100) + "\n"
	f, err := lint.NewFile("test.md", []byte(line))
	require.NoError(t, err)

	effective := map[string]config.RuleCfg{
		"line-length": {
			Enabled:  true,
			Settings: map[string]any{"max": 120},
		},
	}
	rules := []rule.Rule{&configurableLengthRule{Max: 80}}

	diags, errs := CheckRules(f, rules, effective)
	require.Len(t, errs, 0, "unexpected errors: %v", errs)
	require.Len(t, diags, 0, "expected 0 diagnostics with max=120, got %d", len(diags))
}

// mockConfigurableErrorRule implements both Rule and Configurable.
// Its ApplySettings always returns an error.
type mockConfigurableErrorRule struct {
	id   string
	name string
}

func (r *mockConfigurableErrorRule) ID() string       { return r.id }
func (r *mockConfigurableErrorRule) Name() string     { return r.name }
func (r *mockConfigurableErrorRule) Category() string { return "test" }
func (r *mockConfigurableErrorRule) Check(_ *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{
		{
			Line:     1,
			Column:   1,
			RuleID:   r.id,
			RuleName: r.name,
			Severity: lint.Warning,
			Message:  "should not appear",
		},
	}
}
func (r *mockConfigurableErrorRule) ApplySettings(_ map[string]any) error {
	return fmt.Errorf("bad settings")
}
func (r *mockConfigurableErrorRule) DefaultSettings() map[string]any {
	return map[string]any{}
}

var _ rule.Configurable = (*mockConfigurableErrorRule)(nil)

func TestCheckRules_ApplySettingsError(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	require.NoError(t, err)

	effective := map[string]config.RuleCfg{
		"bad-rule": {
			Enabled:  true,
			Settings: map[string]any{"key": "val"},
		},
	}
	rules := []rule.Rule{&mockConfigurableErrorRule{id: "MDS900", name: "bad-rule"}}

	diags, errs := CheckRules(f, rules, effective)

	// The rule should be skipped (no diagnostics from it).
	assert.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diags)

	// The error should be returned in the errors slice.
	require.Len(t, errs, 1, "expected 1 error, got %d", len(errs))
	assert.Contains(t, errs[0].Error(), "bad settings", "expected error to contain 'bad settings', got: %v", errs[0])
}

func TestCheckRules_AdjustsLineOffset(t *testing.T) {
	f, err := lint.NewFileFromSource("test.md", []byte("---\ntitle: x\n---\n# Heading\n"), true)
	require.NoError(t, err)

	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: true},
	}
	rules := []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	require.Len(t, errs, 0, "unexpected errors: %v", errs)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	assert.Equal(t, 4, diags[0].Line, "line should be adjusted for front matter")
}

func TestCheckRules_PopulatesSourceContext(t *testing.T) {
	source := "line one\nline two\nline three\nline four\nline five\n"
	f, err := lint.NewFile("test.md", []byte(source))
	require.NoError(t, err)

	// mockRule reports at line 1; we want to test with a rule that hits line 3.
	r := &mockRuleAtLine{id: "MDS998", name: "mid-rule", line: 3, col: 5}
	effective := map[string]config.RuleCfg{
		"mid-rule": {Enabled: true},
	}

	diags, errs := CheckRules(f, []rule.Rule{r}, effective)
	require.Len(t, errs, 0)
	require.Len(t, diags, 1)

	d := diags[0]
	assert.Equal(t, 3, d.Line)
	assert.Equal(t, 1, d.SourceStartLine, "context should start at line 1")
	require.Len(t, d.SourceLines, 5, "expected 5 context lines (±2)")
	assert.Equal(t, "line one", d.SourceLines[0])
	assert.Equal(t, "line three", d.SourceLines[2])
	assert.Equal(t, "line five", d.SourceLines[4])
}

func TestCheckRules_PopulatesSourceContextAtFileStart(t *testing.T) {
	source := "first\nsecond\nthird\n"
	f, err := lint.NewFile("test.md", []byte(source))
	require.NoError(t, err)

	r := &mockRuleAtLine{id: "MDS998", name: "mid-rule", line: 1, col: 1}
	effective := map[string]config.RuleCfg{
		"mid-rule": {Enabled: true},
	}

	diags, errs := CheckRules(f, []rule.Rule{r}, effective)
	require.Len(t, errs, 0)
	require.Len(t, diags, 1)

	d := diags[0]
	assert.Equal(t, 1, d.SourceStartLine)
	require.Len(t, d.SourceLines, 3, "expected 3 context lines (line 1 + 2 after)")
	assert.Equal(t, "first", d.SourceLines[0])
}

func TestCheckRules_PopulatesSourceContextWithFrontMatter(t *testing.T) {
	source := "---\ntitle: x\n---\nline one\nline two\nline three\n"
	f, err := lint.NewFileFromSource("test.md", []byte(source), true)
	require.NoError(t, err)

	// mockRule reports at line 1 (relative to body), adjusted to line 4.
	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: true},
	}
	rules := []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	require.Len(t, errs, 0)
	require.Len(t, diags, 1)

	d := diags[0]
	assert.Equal(t, 4, d.Line, "line should be adjusted for front matter")
	assert.Equal(t, 4, d.SourceStartLine)
	require.NotEmpty(t, d.SourceLines)
	assert.Equal(t, "line one", d.SourceLines[0])
}

func TestCheckRules_PopulatesSourceContextAtFileEnd(t *testing.T) {
	// File with trailing newline: will have empty element in Lines array
	source := "line one\nline two\nline three\nline four\nline five\n"
	f, err := lint.NewFile("test.md", []byte(source))
	require.NoError(t, err)

	// Diagnostic on the last actual line (line 5)
	r := &mockRuleAtLine{id: "MDS998", name: "end-rule", line: 5, col: 1}
	effective := map[string]config.RuleCfg{
		"end-rule": {Enabled: true},
	}

	diags, errs := CheckRules(f, []rule.Rule{r}, effective)
	require.Len(t, errs, 0)
	require.Len(t, diags, 1)

	d := diags[0]
	assert.Equal(t, 5, d.Line)
	// With context=2 from line 5: should include lines 3, 4, 5
	require.Len(t, d.SourceLines, 3, "expected 3 context lines (line 5 - 2)")

	// Check that no empty line is included
	for i, line := range d.SourceLines {
		assert.NotEmpty(t, line, "SourceLines[%d] should not be empty", i)
	}

	assert.Equal(t, "line three", d.SourceLines[0])
	assert.Equal(t, "line four", d.SourceLines[1])
	assert.Equal(t, "line five", d.SourceLines[2])
}

func TestCheckRules_PopulatesSourceContextSingleLine(t *testing.T) {
	source := "only line\n"
	f, err := lint.NewFile("test.md", []byte(source))
	require.NoError(t, err)

	r := &mockRuleAtLine{id: "MDS998", name: "single-rule", line: 1, col: 1}
	effective := map[string]config.RuleCfg{
		"single-rule": {Enabled: true},
	}

	diags, errs := CheckRules(f, []rule.Rule{r}, effective)
	require.Len(t, errs, 0)
	require.Len(t, diags, 1)

	d := diags[0]
	require.Len(t, d.SourceLines, 1, "single-line file should produce 1 context line")
	assert.Equal(t, "only line", d.SourceLines[0])
	assert.Equal(t, 1, d.SourceStartLine)
}

func TestCheckRules_DiagnosticBeyondFileEnd(t *testing.T) {
	source := "line one\nline two\n"
	f, err := lint.NewFile("test.md", []byte(source))
	require.NoError(t, err)

	// Diagnostic on line 10, but file only has 2 lines
	r := &mockRuleAtLine{id: "MDS998", name: "beyond-rule", line: 10, col: 1}
	effective := map[string]config.RuleCfg{
		"beyond-rule": {Enabled: true},
	}

	diags, errs := CheckRules(f, []rule.Rule{r}, effective)
	require.Len(t, errs, 0)
	require.Len(t, diags, 1)

	d := diags[0]
	assert.Empty(t, d.SourceLines, "diagnostic beyond file end should have no source context")
	assert.Equal(t, 0, d.SourceStartLine)
}

// mockRuleAtLine reports a diagnostic at a specific line.
type mockRuleAtLine struct {
	id   string
	name string
	line int
	col  int
}

func (r *mockRuleAtLine) ID() string       { return r.id }
func (r *mockRuleAtLine) Name() string     { return r.name }
func (r *mockRuleAtLine) Category() string { return "test" }
func (r *mockRuleAtLine) Check(f *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{
		{
			File:     f.Path,
			Line:     r.line,
			Column:   r.col,
			RuleID:   r.id,
			RuleName: r.name,
			Severity: lint.Warning,
			Message:  "mock violation",
		},
	}
}

// --- ConfigureRule tests ---

func TestConfigureRule_NoSettings(t *testing.T) {
	rl := &mockRule{id: "MDS999", name: "mock-rule"}
	cfg := config.RuleCfg{Enabled: true, Settings: nil}

	got, err := ConfigureRule(rl, cfg)
	require.NoError(t, err, "unexpected error: %v", err)
	assert.Same(t, rl, got, "expected same rule instance when no settings")
}

func TestConfigureRule_NonConfigurable(t *testing.T) {
	rl := &mockRule{id: "MDS999", name: "mock-rule"}
	cfg := config.RuleCfg{Enabled: true, Settings: map[string]any{"key": "val"}}

	got, err := ConfigureRule(rl, cfg)
	require.NoError(t, err, "unexpected error: %v", err)
	// mockRule does not implement Configurable, so the same instance is returned.
	assert.Same(t, rl, got, "expected same rule instance for non-configurable rule")
}

func TestConfigureRule_AppliesSettings(t *testing.T) {
	rl := &configurableLengthRule{Max: 80}
	cfg := config.RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"max": 120},
	}

	got, err := ConfigureRule(rl, cfg)
	require.NoError(t, err, "unexpected error: %v", err)
	// Should be a different instance (cloned).
	assert.NotSame(t, rl, got, "expected a cloned rule, got same instance")

	// The cloned rule should have max=120 applied.
	cloned, ok := got.(*configurableLengthRule)
	require.True(t, ok, "expected *configurableLengthRule, got %T", got)
	assert.Equal(t, 120, cloned.Max, "expected Max=120, got %d", cloned.Max)

	// Original should be unchanged.
	assert.Equal(t, 80, rl.Max, "original Max changed to %d, want 80", rl.Max)
}

func TestConfigureRule_ApplySettingsError(t *testing.T) {
	rl := &mockConfigurableErrorRule{id: "MDS900", name: "bad-rule"}
	cfg := config.RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"key": "val"},
	}

	got, err := ConfigureRule(rl, cfg)
	require.Error(t, err, "expected error, got nil")
	assert.Nil(t, got, "expected nil rule on error, got %v", got)
	assert.Contains(t, err.Error(), "bad settings", "expected error to contain 'bad settings', got: %v", err)
}

// --- filterGeneratedDiags tests ---

func TestFilterGeneratedDiags_EmptyRanges(t *testing.T) {
	diags := []lint.Diagnostic{
		{Line: 3, Message: "keep me"},
	}
	got := filterGeneratedDiags(diags, nil)
	assert.Len(t, got, 1, "no filtering with empty ranges")
}

func TestFilterGeneratedDiags_DropInRange(t *testing.T) {
	ranges := []lint.LineRange{{From: 5, To: 8}}
	diags := []lint.Diagnostic{
		{Line: 4, Message: "before"},
		{Line: 5, Message: "start of range"},
		{Line: 6, Message: "middle"},
		{Line: 8, Message: "end of range"},
		{Line: 9, Message: "after"},
	}
	got := filterGeneratedDiags(diags, ranges)
	require.Len(t, got, 2, "expected 2 diagnostics outside range")
	assert.Equal(t, "before", got[0].Message)
	assert.Equal(t, "after", got[1].Message)
}

func TestFilterGeneratedDiags_MultipleRanges(t *testing.T) {
	ranges := []lint.LineRange{{From: 3, To: 4}, {From: 8, To: 10}}
	diags := []lint.Diagnostic{
		{Line: 2, Message: "keep"},
		{Line: 3, Message: "drop"},
		{Line: 9, Message: "drop"},
		{Line: 11, Message: "keep"},
	}
	got := filterGeneratedDiags(diags, ranges)
	require.Len(t, got, 2)
	assert.Equal(t, "keep", got[0].Message)
	assert.Equal(t, "keep", got[1].Message)
}

func TestCheckRules_FiltersGeneratedRangeDiagnostics(t *testing.T) {
	// File: 5 lines. Lines 3-4 are in a generated range.
	source := "line1\nline2\nline3\nline4\nline5\n"
	f, err := lint.NewFile("host.md", []byte(source))
	require.NoError(t, err)
	f.GeneratedRanges = []lint.LineRange{{From: 3, To: 4}}

	// Rule reports on lines 2 and 3. Line 3 is in the generated range.
	r := &mockMultiLineRule{
		id:    "MDS999",
		name:  "multi-rule",
		lines: []int{2, 3},
	}
	effective := map[string]config.RuleCfg{"multi-rule": {Enabled: true}}

	diags, errs := CheckRules(f, []rule.Rule{r}, effective)
	require.Len(t, errs, 0)
	require.Len(t, diags, 1, "only line-2 diagnostic should survive")
	assert.Equal(t, 2, diags[0].Line)
}

func TestCheckRules_FiltersNoHostDiagnosticsUnaffected(t *testing.T) {
	// Generated range does not overlap the diagnostic line — no filtering.
	source := "line1\nline2\nline3\n"
	f, err := lint.NewFile("host.md", []byte(source))
	require.NoError(t, err)
	f.GeneratedRanges = []lint.LineRange{{From: 10, To: 12}}

	r := &mockRuleAtLine{id: "MDS999", name: "mock-rule", line: 1, col: 1}
	effective := map[string]config.RuleCfg{"mock-rule": {Enabled: true}}

	diags, errs := CheckRules(f, []rule.Rule{r}, effective)
	require.Len(t, errs, 0)
	require.Len(t, diags, 1, "diagnostic outside generated range must not be filtered")
}

// mockMultiLineRule reports a diagnostic at each of the given lines.
type mockMultiLineRule struct {
	id    string
	name  string
	lines []int
}

func (r *mockMultiLineRule) ID() string       { return r.id }
func (r *mockMultiLineRule) Name() string     { return r.name }
func (r *mockMultiLineRule) Category() string { return "test" }
func (r *mockMultiLineRule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for _, l := range r.lines {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     l,
			Column:   1,
			RuleID:   r.id,
			RuleName: r.name,
			Severity: lint.Warning,
			Message:  fmt.Sprintf("violation at line %d", l),
		})
	}
	return diags
}
