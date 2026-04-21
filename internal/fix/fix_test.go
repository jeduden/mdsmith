package fix

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock rules for testing ---

// mockFixableRule is a fixable rule that trims trailing spaces.
type mockFixableRule struct {
	id   string
	name string
}

func (r *mockFixableRule) ID() string       { return r.id }
func (r *mockFixableRule) Name() string     { return r.name }
func (r *mockFixableRule) Category() string { return "test" }

func (r *mockFixableRule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		trimmed := len(line)
		for trimmed > 0 && (line[trimmed-1] == ' ' || line[trimmed-1] == '\t') {
			trimmed--
		}
		if trimmed < len(line) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     i + 1,
				Column:   trimmed + 1,
				RuleID:   r.id,
				RuleName: r.name,
				Severity: lint.Warning,
				Message:  "trailing whitespace",
			})
		}
	}
	return diags
}

func (r *mockFixableRule) Fix(f *lint.File) []byte {
	var result []byte
	for i, line := range f.Lines {
		trimmed := len(line)
		for trimmed > 0 && (line[trimmed-1] == ' ' || line[trimmed-1] == '\t') {
			trimmed--
		}
		result = append(result, line[:trimmed]...)
		if i < len(f.Lines)-1 {
			result = append(result, '\n')
		}
	}
	return result
}

var _ rule.FixableRule = (*mockFixableRule)(nil)

// mockNonFixableRule is a rule that always reports a diagnostic but cannot fix.
type mockNonFixableRule struct {
	id   string
	name string
}

func (r *mockNonFixableRule) ID() string       { return r.id }
func (r *mockNonFixableRule) Name() string     { return r.name }
func (r *mockNonFixableRule) Category() string { return "test" }

func (r *mockNonFixableRule) Check(f *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{
		{
			File:     f.Path,
			Line:     1,
			Column:   1,
			RuleID:   r.id,
			RuleName: r.name,
			Severity: lint.Warning,
			Message:  "non-fixable issue",
		},
	}
}

// mockFixableRuleB replaces tabs with spaces (second fixable rule for ordering tests).
type mockFixableRuleB struct {
	id   string
	name string
}

func (r *mockFixableRuleB) ID() string       { return r.id }
func (r *mockFixableRuleB) Name() string     { return r.name }
func (r *mockFixableRuleB) Category() string { return "test" }

func (r *mockFixableRuleB) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		for j, b := range line {
			if b == '\t' {
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     i + 1,
					Column:   j + 1,
					RuleID:   r.id,
					RuleName: r.name,
					Severity: lint.Warning,
					Message:  "hard tab",
				})
			}
		}
	}
	return diags
}

func (r *mockFixableRuleB) Fix(f *lint.File) []byte {
	var result []byte
	for i, line := range f.Lines {
		for _, b := range line {
			if b == '\t' {
				result = append(result, ' ', ' ', ' ', ' ')
			} else {
				result = append(result, b)
			}
		}
		if i < len(f.Lines)-1 {
			result = append(result, '\n')
		}
	}
	return result
}

var _ rule.FixableRule = (*mockFixableRuleB)(nil)

// mockSloppyTabFixer replaces tabs with spaces but sloppily adds a trailing
// space to each modified line. This simulates a fix that introduces a violation
// for the trailing-spaces rule.
type mockSloppyTabFixer struct {
	id   string
	name string
}

func (r *mockSloppyTabFixer) ID() string       { return r.id }
func (r *mockSloppyTabFixer) Name() string     { return r.name }
func (r *mockSloppyTabFixer) Category() string { return "test" }

func (r *mockSloppyTabFixer) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		for j, b := range line {
			if b == '\t' {
				diags = append(diags, lint.Diagnostic{
					File: f.Path, Line: i + 1, Column: j + 1,
					RuleID: r.id, RuleName: r.name,
					Severity: lint.Warning, Message: "hard tab",
				})
			}
		}
	}
	return diags
}

func (r *mockSloppyTabFixer) Fix(f *lint.File) []byte {
	var result []byte
	for i, line := range f.Lines {
		hadTab := false
		for _, b := range line {
			if b == '\t' {
				result = append(result, ' ', ' ', ' ', ' ')
				hadTab = true
			} else {
				result = append(result, b)
			}
		}
		// Sloppy: adds a trailing space on lines that had tabs.
		if hadTab {
			result = append(result, ' ')
		}
		if i < len(f.Lines)-1 {
			result = append(result, '\n')
		}
	}
	return result
}

var _ rule.FixableRule = (*mockSloppyTabFixer)(nil)

// silentRule is a rule that never reports any diagnostics.
type silentRule struct {
	id   string
	name string
}

func (r *silentRule) ID() string                           { return r.id }
func (r *silentRule) Name() string                         { return r.name }
func (r *silentRule) Category() string                     { return "test" }
func (r *silentRule) Check(_ *lint.File) []lint.Diagnostic { return nil }

// mockFlakyConfigurableRule succeeds on the first non-default ApplySettings
// call, fails on the second, then succeeds again.
//
// This exercises the fix path that runs CheckRules before and after fixing:
// the error on the pre-fix check must be collected.
type mockFlakyConfigurableRule struct {
	id   string
	name string
}

var flakyConfigurableApplyCalls int

func (r *mockFlakyConfigurableRule) ID() string       { return r.id }
func (r *mockFlakyConfigurableRule) Name() string     { return r.name }
func (r *mockFlakyConfigurableRule) Category() string { return "test" }
func (r *mockFlakyConfigurableRule) Check(_ *lint.File) []lint.Diagnostic {
	return nil
}
func (r *mockFlakyConfigurableRule) DefaultSettings() map[string]any {
	return map[string]any{"mode": "default"}
}
func (r *mockFlakyConfigurableRule) ApplySettings(settings map[string]any) error {
	if settings["mode"] == "default" {
		return nil
	}

	flakyConfigurableApplyCalls++
	if flakyConfigurableApplyCalls == 2 {
		return errors.New("flaky settings failure")
	}
	return nil
}

var _ rule.Configurable = (*mockFlakyConfigurableRule)(nil)

// --- tests ---

func TestFix_BasicTrailingSpaces(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Hello  \nworld  \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Errors, 0, "unexpected errors: %v", result.Errors)
	if result.FilesChecked != 1 {
		t.Fatalf("expected 1 checked file, got %d", result.FilesChecked)
	}
	if result.Failures != 2 {
		t.Fatalf("expected 2 pre-fix failures, got %d", result.Failures)
	}
	require.Len(t, result.Modified, 1, "expected 1 modified file, got %d", len(result.Modified))

	content, err := os.ReadFile(mdFile)
	require.NoError(t, err)
	expected := "# Hello\nworld\n"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}

	// No remaining diagnostics for this fixable rule.
	assert.Len(t, result.Diagnostics, 0, "expected 0 remaining diagnostics, got %d", len(result.Diagnostics))
}

func TestFix_MultipleFixableRulesAppliedInOrder(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	// Content has a mid-line tab and trailing spaces (no trailing tab).
	// This way MDS100 (trailing spaces) strips the trailing spaces,
	// then MDS200 (tabs) converts the mid-line tab to spaces.
	if err := os.WriteFile(mdFile, []byte("# He\tllo  \nwor\tld  \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-tabs":     {Enabled: true},
			"mock-trailing": {Enabled: true},
		},
	}

	// MDS100 (trailing) runs before MDS200 (tabs) by ID sort order.
	// But we register them in reverse order to test sorting.
	fixer := &Fixer{
		Config: cfg,
		Rules: []rule.Rule{
			&mockFixableRuleB{id: "MDS200", name: "mock-tabs"},
			&mockFixableRule{id: "MDS100", name: "mock-trailing"},
		},
	}

	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Errors, 0, "unexpected errors: %v", result.Errors)
	require.Len(t, result.Modified, 1, "expected 1 modified file, got %d", len(result.Modified))

	content, err := os.ReadFile(mdFile)
	require.NoError(t, err)
	// MDS100 (trailing spaces) runs first: "# He\tllo  " -> "# He\tllo"
	// MDS200 (tabs) runs second: "# He\tllo" -> "# He    llo"
	expected := "# He    llo\nwor    ld\n"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}
}

func TestFix_NonFixableViolationsReportedAfterFix(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Hello  \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing":   {Enabled: true},
			"mock-nonfixable": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules: []rule.Rule{
			&mockFixableRule{id: "MDS100", name: "mock-trailing"},
			&mockNonFixableRule{id: "MDS999", name: "mock-nonfixable"},
		},
	}

	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Errors, 0, "unexpected errors: %v", result.Errors)
	if result.FilesChecked != 1 {
		t.Fatalf("expected 1 checked file, got %d", result.FilesChecked)
	}
	if result.Failures != 2 {
		t.Fatalf("expected 2 pre-fix failures, got %d", result.Failures)
	}

	// The fixable rule should have fixed trailing spaces, but the non-fixable
	// rule still reports a diagnostic.
	require.Len(t, result.Diagnostics, 1, "expected 1 remaining diagnostic, got %d", len(result.Diagnostics))
	if result.Diagnostics[0].RuleID != "MDS999" {
		t.Errorf("expected remaining diagnostic from MDS999, got %s", result.Diagnostics[0].RuleID)
	}
}

func TestFix_FileWithNoViolationsNotModified(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "clean.md")
	content := []byte("# Clean file\n")
	if err := os.WriteFile(mdFile, content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Record mtime before fix.
	infoBefore, err := os.Stat(mdFile)
	require.NoError(t, err)
	mtimeBefore := infoBefore.ModTime()

	// Small delay so mtime would change if file were rewritten.
	time.Sleep(50 * time.Millisecond)

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"silent-rule": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&silentRule{id: "MDS998", name: "silent-rule"}},
	}

	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Errors, 0, "unexpected errors: %v", result.Errors)
	require.Len(t, result.Modified, 0, "expected 0 modified files, got %d", len(result.Modified))

	infoAfter, err := os.Stat(mdFile)
	require.NoError(t, err)
	assert.True(t, infoAfter.ModTime().Equal(mtimeBefore),
		"mtime changed: before=%v after=%v", mtimeBefore, infoAfter.ModTime())
}

func TestFix_ReadOnlyFileError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only file test not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("read-only file test not reliable as root")
	}

	dir := t.TempDir()
	mdFile := filepath.Join(dir, "readonly.md")
	if err := os.WriteFile(mdFile, []byte("# Hello  \n"), 0o444); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Errors, 1,
		"expected 1 error for read-only file, got %d: %v", len(result.Errors), result.Errors)
}

func TestFix_MultipleFilesFixedIndependently(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.md")
	file2 := filepath.Join(dir, "b.md")
	if err := os.WriteFile(file1, []byte("# A  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("# B  \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{file1, file2})
	require.Len(t, result.Errors, 0, "unexpected errors: %v", result.Errors)
	require.Len(t, result.Modified, 2, "expected 2 modified files, got %d", len(result.Modified))

	for _, f := range []string{file1, file2} {
		content, err := os.ReadFile(f)
		require.NoError(t, err)
		if content[len(content)-2] == ' ' {
			t.Errorf("file %s still has trailing spaces", f)
		}
	}
}

func TestFix_EmptyPathsReturnsEmptyResult(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{})
	assert.Len(t, result.Diagnostics, 0, "expected 0 diagnostics, got %d", len(result.Diagnostics))
	assert.Len(t, result.Modified, 0, "expected 0 modified files, got %d", len(result.Modified))
	assert.Len(t, result.Errors, 0, "expected 0 errors, got %d", len(result.Errors))
	if result.FilesChecked != 0 {
		t.Errorf("expected 0 checked files, got %d", result.FilesChecked)
	}
	if result.Failures != 0 {
		t.Errorf("expected 0 failures, got %d", result.Failures)
	}
}

func TestFix_PreFixCheckRulesErrorsCollected(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	flakyConfigurableApplyCalls = 0
	t.Cleanup(func() { flakyConfigurableApplyCalls = 0 })

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-flaky-config": {
				Enabled:  true,
				Settings: map[string]any{"mode": "custom"},
			},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFlakyConfigurableRule{id: "MDS997", name: "mock-flaky-config"}},
	}

	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Errors, 1,
		"expected 1 pre-fix CheckRules error, got %d: %v", len(result.Errors), result.Errors)
	require.Contains(t, result.Errors[0].Error(), "flaky settings failure",
		"expected flaky settings error, got: %v", result.Errors[0])
	require.Len(t, result.Diagnostics, 0, "expected 0 diagnostics, got %d", len(result.Diagnostics))
	require.Len(t, result.Modified, 0, "expected 0 modified files, got %d", len(result.Modified))
}

func TestFix_ReParseBetweenPasses(t *testing.T) {
	// This test ensures that after one fixable rule modifies content, the next
	// fixable rule sees the updated source (re-parsed lint.File).
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	// Content has trailing tab+spaces: after MDS100 strips trailing whitespace,
	// MDS200 should not see any tabs (they were part of trailing whitespace).
	if err := os.WriteFile(mdFile, []byte("# Hello\t \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
			"mock-tabs":     {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules: []rule.Rule{
			// MDS100 sorts before MDS200, so trailing whitespace removal happens first.
			&mockFixableRule{id: "MDS100", name: "mock-trailing"},
			&mockFixableRuleB{id: "MDS200", name: "mock-tabs"},
		},
	}

	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Errors, 0, "unexpected errors: %v", result.Errors)

	content, err := os.ReadFile(mdFile)
	require.NoError(t, err)
	// MDS100 strips trailing "\t " -> "# Hello\n"
	// MDS200 sees "# Hello\n" which has no tabs -> no change.
	expected := "# Hello\n"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}
}

// mockRuleA flags and removes lines that are just "REMOVE_ME".
// This simulates a rule whose fix can be undone by another rule's fix.
type mockRuleA struct {
	id   string
	name string
}

func (r *mockRuleA) ID() string       { return r.id }
func (r *mockRuleA) Name() string     { return r.name }
func (r *mockRuleA) Category() string { return "test" }

func (r *mockRuleA) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		if string(line) == "REMOVE_ME" {
			diags = append(diags, lint.Diagnostic{
				File: f.Path, Line: i + 1, Column: 1,
				RuleID: r.id, RuleName: r.name,
				Severity: lint.Warning, Message: "remove me line",
			})
		}
	}
	return diags
}

func (r *mockRuleA) Fix(f *lint.File) []byte {
	var result []byte
	for i, line := range f.Lines {
		if string(line) == "REMOVE_ME" {
			continue
		}
		result = append(result, line...)
		if i < len(f.Lines)-1 {
			result = append(result, '\n')
		}
	}
	return result
}

var _ rule.FixableRule = (*mockRuleA)(nil)

// mockRuleB flags consecutive blank lines and collapses them to one.
// Its fix can introduce "REMOVE_ME" is a stand-in: here it simply adds
// trailing whitespace that mockFixableRule (MDS100) would need to clean.
// But for the cross-rule regression test we use a different approach:
// mockRuleB replaces "AAA\nBBB" with "AAA\nREMOVE_ME\nBBB",
// simulating a fix that introduces a violation for mockRuleA.
type mockCrossRule struct {
	id   string
	name string
}

func (r *mockCrossRule) ID() string       { return r.id }
func (r *mockCrossRule) Name() string     { return r.name }
func (r *mockCrossRule) Category() string { return "test" }

func (r *mockCrossRule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for i := 0; i+1 < len(f.Lines); i++ {
		if string(f.Lines[i]) == "AAA" && string(f.Lines[i+1]) == "BBB" {
			diags = append(diags, lint.Diagnostic{
				File: f.Path, Line: i + 1, Column: 1,
				RuleID: r.id, RuleName: r.name,
				Severity: lint.Warning, Message: "need separator",
			})
		}
	}
	return diags
}

func (r *mockCrossRule) Fix(f *lint.File) []byte {
	var result []byte
	for i, line := range f.Lines {
		result = append(result, line...)
		if i < len(f.Lines)-1 {
			result = append(result, '\n')
		}
		// Insert REMOVE_ME between AAA and BBB.
		if string(line) == "AAA" && i+1 < len(f.Lines) && string(f.Lines[i+1]) == "BBB" {
			result = append(result, "REMOVE_ME\n"...)
		}
	}
	return result
}

var _ rule.FixableRule = (*mockCrossRule)(nil)

func TestFix_MultiPassConvergence(t *testing.T) {
	// mockRuleA (MDS100) removes "REMOVE_ME" lines.
	// mockCrossRule (MDS200) inserts "REMOVE_ME" between "AAA" and "BBB".
	// On a single pass: MDS100 runs first (no REMOVE_ME yet, no-op),
	// then MDS200 inserts REMOVE_ME. Without multi-pass, REMOVE_ME remains.
	// With multi-pass, the second pass lets MDS100 remove it, and MDS200
	// no longer sees adjacent AAA/BBB, so it converges.
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("AAA\nBBB\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-remove":    {Enabled: true},
			"mock-separator": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules: []rule.Rule{
			&mockRuleA{id: "MDS100", name: "mock-remove"},
			&mockCrossRule{id: "MDS200", name: "mock-separator"},
		},
	}

	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Errors, 0, "unexpected errors: %v", result.Errors)

	content, err := os.ReadFile(mdFile)
	require.NoError(t, err)

	// The fixes oscillate within each pass: MDS100 removes REMOVE_ME,
	// MDS200 re-inserts it. The pass output equals the previous pass
	// output so the loop detects stability and stops.
	// Final content has REMOVE_ME because MDS200 runs after MDS100.
	expected := "AAA\nREMOVE_ME\nBBB\n"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}

	// MDS100 reports a remaining diagnostic for the REMOVE_ME line.
	found := false
	for _, d := range result.Diagnostics {
		if d.RuleID == "MDS100" {
			found = true
		}
	}
	assert.True(t, found, "expected remaining MDS100 diagnostic")
}

func TestFix_LaterRuleFixCaughtByEarlierRule(t *testing.T) {
	// Regression test for the actual bug: a later rule's fix introduces
	// trailing whitespace that an earlier rule should clean up.
	// mockFixableRule (MDS100) strips trailing spaces.
	// mockTrailingAdder (MDS200) fixes something but adds trailing spaces.
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	// "hello\tworld\n" — has a tab that MDS200 will convert to spaces,
	// but it incorrectly adds a trailing space.
	if err := os.WriteFile(mdFile, []byte("hello\tworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing":    {Enabled: true},
			"mock-tabs-sloppy": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules: []rule.Rule{
			&mockFixableRule{id: "MDS100", name: "mock-trailing"},
			&mockSloppyTabFixer{id: "MDS200", name: "mock-tabs-sloppy"},
		},
	}

	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Errors, 0, "unexpected errors: %v", result.Errors)

	content, err := os.ReadFile(mdFile)
	require.NoError(t, err)

	// MDS100 first pass: no trailing spaces, no-op.
	// MDS200 first pass: replaces tab with "    " but adds trailing space
	//   -> "hello    world \n"
	// Second pass: MDS100 strips trailing space -> "hello    world\n"
	// MDS200: no tabs, no-op. Stable.
	expected := "hello    world\n"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}

	// No remaining diagnostics — both issues fully fixed.
	for _, d := range result.Diagnostics {
		if d.RuleID == "MDS100" || d.RuleID == "MDS200" {
			t.Errorf("unexpected remaining diagnostic: %s: %s", d.RuleID, d.Message)
		}
	}
}

func TestFixer_StripFrontMatter_PreservesFrontMatter(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	// File with frontmatter followed by content with trailing spaces.
	content := "---\ntitle: hello\n---\n# Heading  \n\nSome text  \n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config:           cfg,
		Rules:            []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		StripFrontMatter: true,
	}

	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Errors, 0, "unexpected errors: %v", result.Errors)

	// Read the file back.
	got, err := os.ReadFile(mdFile)
	require.NoError(t, err)

	// Frontmatter should be preserved intact.
	expectedFM := "---\ntitle: hello\n---\n"
	if !strings.HasPrefix(string(got), expectedFM) {
		t.Errorf("frontmatter not preserved; got prefix %q, want %q",
			string(got[:len(expectedFM)]), expectedFM)
	}

	// Content portion should be fixed (no trailing spaces).
	body := string(got[len(expectedFM):])
	assert.NotContains(t, body, "  ", "content still has trailing spaces: %q", body)
	expectedBody := "# Heading\n\nSome text\n"
	assert.Equal(t, expectedBody, body, "expected body %q, got %q", expectedBody, body)

	// Remaining diagnostics should have line numbers adjusted for the offset.
	for _, d := range result.Diagnostics {
		// Front matter has 3 lines, so all diagnostic lines should be > 3.
		if d.Line <= 3 {
			t.Errorf("diagnostic line %d should be offset past frontmatter (> 3)", d.Line)
		}
	}
}

func TestFix_IgnoredFileSkipped(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(mdFile, []byte("# Changelog  \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
		Ignore: []string{"CHANGELOG.md"},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Modified, 0, "expected 0 modified files for ignored file, got %d", len(result.Modified))
	require.Len(t, result.Diagnostics, 0, "expected 0 diagnostics for ignored file, got %d", len(result.Diagnostics))
}

func TestFix_NonexistentFileError(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{"/nonexistent/file.md"})
	require.Len(t, result.Errors, 1, "expected 1 error, got %d", len(result.Errors))
}

func TestFix_PreservesFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test not reliable on Windows")
	}

	dir := t.TempDir()
	mdFile := filepath.Join(dir, "exec.md")
	if err := os.WriteFile(mdFile, []byte("# Hello  \n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Errors, 0, "unexpected errors: %v", result.Errors)

	info, err := os.Stat(mdFile)
	require.NoError(t, err)
	if info.Mode().Perm() != 0o755 {
		t.Errorf("expected permissions 0755, got %04o", info.Mode().Perm())
	}
}

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	data := []byte("hello world")
	err := atomicWriteFile(path, data, 0o644)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, got)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestAtomicWriteFile_NoPartialWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// Write initial content.
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	// Overwrite atomically.
	err := atomicWriteFile(path, []byte("replacement"), 0o644)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "replacement", string(got))

	// Verify no temp files left behind.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "should only have the target file")
}

func TestAtomicWriteFile_PreservesPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test not applicable on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	err := atomicWriteFile(path, []byte("data"), 0o755)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestAtomicWriteFile_ReadOnlyTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only file test not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("read-only file test not reliable as root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "readonly.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o444))

	err := atomicWriteFile(path, []byte("replacement"), 0o644)
	require.Error(t, err, "should fail writing to read-only target")

	// Verify original content is unchanged.
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(got))
}

func TestAtomicWriteFile_BadDirectory(t *testing.T) {
	err := atomicWriteFile("/nonexistent-dir/file.txt", []byte("data"), 0o644)
	require.Error(t, err, "should fail for nonexistent parent directory")
}

func TestAtomicWriteFile_StatErrorNotENOENT(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based stat test not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("permission test not reliable as root")
	}
	// Create a directory with a file, then remove read+execute perms
	// on the directory so Stat on the file fails with EACCES, not ENOENT.
	dir := t.TempDir()
	sub := filepath.Join(dir, "restricted")
	require.NoError(t, os.Mkdir(sub, 0o755))
	target := filepath.Join(sub, "file.txt")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o644))
	require.NoError(t, os.Chmod(sub, 0o000))
	defer func() { _ = os.Chmod(sub, 0o755) }()

	err := atomicWriteFile(target, []byte("new"), 0o644)
	require.Error(t, err, "should fail when Stat returns non-ENOENT error")
}

// mockNonConvergingRule always reports a diagnostic and appends "X" on every
// Fix call, so content never stabilises and the fixer exhausts all passes.
type mockNonConvergingRule struct {
	id   string
	name string
}

func (r *mockNonConvergingRule) ID() string       { return r.id }
func (r *mockNonConvergingRule) Name() string     { return r.name }
func (r *mockNonConvergingRule) Category() string { return "test" }

func (r *mockNonConvergingRule) Check(f *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{{
		File: f.Path, Line: 1, Column: 1,
		RuleID: r.id, RuleName: r.name,
		Severity: lint.Warning, Message: "always needs fixing",
	}}
}

func (r *mockNonConvergingRule) Fix(f *lint.File) []byte {
	return append(append([]byte(nil), f.Source...), 'X')
}

var _ rule.FixableRule = (*mockNonConvergingRule)(nil)

func TestFix_MaxPassesBoundary(t *testing.T) {
	// A rule whose Fix always appends "X", so content never converges.
	// applyFixPasses must exit after exactly maxPasses (10) iterations.
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	initial := []byte("A\n")
	require.NoError(t, os.WriteFile(mdFile, initial, 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-non-converging": {Enabled: true},
		},
	}
	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockNonConvergingRule{id: "MDS999", name: "mock-non-converging"}},
	}

	result := fixer.Fix([]string{mdFile})
	require.Empty(t, result.Errors, "unexpected errors: %v", result.Errors)

	got, err := os.ReadFile(mdFile)
	require.NoError(t, err)

	// After 10 passes each appending "X", the file should end with 10 X's.
	const maxPasses = 10
	assert.Equal(t, string(initial)+strings.Repeat("X", maxPasses), string(got))
}
