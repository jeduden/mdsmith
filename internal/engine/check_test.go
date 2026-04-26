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

// TestCheckRules_GeneratedSectionDiagsDropped verifies that diagnostics whose
// lines fall within a generated section (between <?include?> or <?catalog?>
// markers) are suppressed when linting the host file.
func TestCheckRules_GeneratedSectionDiagsDropped(t *testing.T) {
	// Host file with an include generated section. Lines 3-5 are the PI block,
	// line 6 is embedded content (ContentFrom=ContentTo=6), line 7 is end marker.
	src := "# Host\n\n<?include\nfile: fragment.md\n?>\nEmbedded content\n<?/include?>\n\nHost content\n"
	f, err := lint.NewFile("host.md", []byte(src))
	require.NoError(t, err)

	// Rule that reports a diagnostic on every line.
	r := &everyLineRule{id: "MDS990", name: "every-line"}
	effective := map[string]config.RuleCfg{
		"every-line": {Enabled: true},
	}

	diags, errs := CheckRules(f, []rule.Rule{r}, effective)
	require.Empty(t, errs)

	// Line 6 ("Embedded content") is inside the generated section; must be dropped.
	for _, d := range diags {
		assert.NotEqual(t, 6, d.Line,
			"diagnostic on line 6 (generated section content) should be suppressed")
	}
	// There should still be diagnostics for host-owned lines.
	assert.NotEmpty(t, diags, "host-owned line diagnostics should not be suppressed")
}

// TestCheckRules_GeneratedSectionCatalog verifies that diagnostics on lines
// within a <?catalog?> generated section are also suppressed.
func TestCheckRules_GeneratedSectionCatalog(t *testing.T) {
	// Host with a catalog generated section. Line 4 is the embedded content.
	src := "# Catalog\n\n<?catalog\nglob: \"*.md\"\n?>\nrow content\n<?/catalog?>\n"
	f, err := lint.NewFile("host.md", []byte(src))
	require.NoError(t, err)

	r := &everyLineRule{id: "MDS990", name: "every-line"}
	effective := map[string]config.RuleCfg{"every-line": {Enabled: true}}

	diags, errs := CheckRules(f, []rule.Rule{r}, effective)
	require.Empty(t, errs)

	// Line 6 ("row content") is the catalog generated section content; must be dropped.
	for _, d := range diags {
		assert.NotEqual(t, 6, d.Line,
			"diagnostic on line 6 (catalog generated section content) should be suppressed")
	}
	assert.NotEmpty(t, diags, "host-owned line diagnostics should not be suppressed")
}

// TestCheckRules_FragmentDirectDiagnosticsPreserved verifies that when a
// fragment is linted directly (not as part of a host), its diagnostics appear.
func TestCheckRules_FragmentDirectDiagnosticsPreserved(t *testing.T) {
	// Fragment file: no generated sections, just plain content.
	src := "Fragment content on line 1\nFragment content on line 2\n"
	f, err := lint.NewFile("fragment.md", []byte(src))
	require.NoError(t, err)

	r := &mockRuleAtLine{id: "MDS990", name: "every-line", line: 1, col: 1}
	effective := map[string]config.RuleCfg{"every-line": {Enabled: true}}

	diags, errs := CheckRules(f, []rule.Rule{r}, effective)
	require.Empty(t, errs)

	// Diagnostic on line 1 should appear since there are no generated sections.
	require.Len(t, diags, 1, "fragment linted directly should report its diagnostics")
	assert.Equal(t, 1, diags[0].Line)
}

// TestCheckRules_MultipleGeneratedSections verifies that multiple generated
// sections are all filtered, and host content between them is not filtered.
func TestCheckRules_MultipleGeneratedSections(t *testing.T) {
	// Lines: 1=heading, 2=blank, 3-5=PI1, 6=gen1, 7=end1,
	// 8=blank, 9=host-owned, 10=blank, 11-13=PI2, 14=gen2, 15=end2, 16=host
	src := strings.Join([]string{
		"# Host",
		"",
		"<?include",
		"file: a.md",
		"?>",
		"Generated A",
		"<?/include?>",
		"",
		"Host paragraph",
		"",
		"<?include",
		"file: b.md",
		"?>",
		"Generated B",
		"<?/include?>",
		"",
		"End",
		"",
	}, "\n")
	f, err := lint.NewFile("host.md", []byte(src))
	require.NoError(t, err)

	r := &everyLineRule{id: "MDS990", name: "every-line"}
	effective := map[string]config.RuleCfg{"every-line": {Enabled: true}}

	diags, errs := CheckRules(f, []rule.Rule{r}, effective)
	require.Empty(t, errs)

	lineSet := make(map[int]bool)
	for _, d := range diags {
		lineSet[d.Line] = true
	}

	// Lines 6 and 14 are in generated sections; must not appear.
	assert.False(t, lineSet[6], "line 6 (generated A) should be suppressed")
	assert.False(t, lineSet[14], "line 14 (generated B) should be suppressed")
	// Line 9 is host-owned; must appear.
	assert.True(t, lineSet[9], "line 9 (host paragraph) should not be suppressed")
}

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
