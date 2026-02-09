package fix

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jeduden/tidymark/internal/config"
	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
)

// --- mock rules for testing ---

// mockFixableRule is a fixable rule that trims trailing spaces.
type mockFixableRule struct {
	id   string
	name string
}

func (r *mockFixableRule) ID() string   { return r.id }
func (r *mockFixableRule) Name() string { return r.name }

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

func (r *mockNonFixableRule) ID() string   { return r.id }
func (r *mockNonFixableRule) Name() string { return r.name }

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

func (r *mockFixableRuleB) ID() string   { return r.id }
func (r *mockFixableRuleB) Name() string { return r.name }

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

func (r *mockSloppyTabFixer) ID() string   { return r.id }
func (r *mockSloppyTabFixer) Name() string { return r.name }

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
func (r *silentRule) Check(_ *lint.File) []lint.Diagnostic { return nil }

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
		Rules:  []rule.Rule{&mockFixableRule{id: "TM100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Modified) != 1 {
		t.Fatalf("expected 1 modified file, got %d", len(result.Modified))
	}

	content, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatal(err)
	}
	expected := "# Hello\nworld\n"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}

	// No remaining diagnostics for this fixable rule.
	if len(result.Diagnostics) != 0 {
		t.Errorf("expected 0 remaining diagnostics, got %d", len(result.Diagnostics))
	}
}

func TestFix_MultipleFixableRulesAppliedInOrder(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	// Content has a mid-line tab and trailing spaces (no trailing tab).
	// This way TM100 (trailing spaces) strips the trailing spaces,
	// then TM200 (tabs) converts the mid-line tab to spaces.
	if err := os.WriteFile(mdFile, []byte("# He\tllo  \nwor\tld  \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-tabs":     {Enabled: true},
			"mock-trailing": {Enabled: true},
		},
	}

	// TM100 (trailing) runs before TM200 (tabs) by ID sort order.
	// But we register them in reverse order to test sorting.
	fixer := &Fixer{
		Config: cfg,
		Rules: []rule.Rule{
			&mockFixableRuleB{id: "TM200", name: "mock-tabs"},
			&mockFixableRule{id: "TM100", name: "mock-trailing"},
		},
	}

	result := fixer.Fix([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Modified) != 1 {
		t.Fatalf("expected 1 modified file, got %d", len(result.Modified))
	}

	content, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatal(err)
	}
	// TM100 (trailing spaces) runs first: "# He\tllo  " -> "# He\tllo"
	// TM200 (tabs) runs second: "# He\tllo" -> "# He    llo"
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
			"mock-trailing":  {Enabled: true},
			"mock-nonfixable": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules: []rule.Rule{
			&mockFixableRule{id: "TM100", name: "mock-trailing"},
			&mockNonFixableRule{id: "TM999", name: "mock-nonfixable"},
		},
	}

	result := fixer.Fix([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	// The fixable rule should have fixed trailing spaces, but the non-fixable
	// rule still reports a diagnostic.
	if len(result.Diagnostics) != 1 {
		t.Fatalf("expected 1 remaining diagnostic, got %d", len(result.Diagnostics))
	}
	if result.Diagnostics[0].RuleID != "TM999" {
		t.Errorf("expected remaining diagnostic from TM999, got %s", result.Diagnostics[0].RuleID)
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
	if err != nil {
		t.Fatal(err)
	}
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
		Rules:  []rule.Rule{&silentRule{id: "TM998", name: "silent-rule"}},
	}

	result := fixer.Fix([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Modified) != 0 {
		t.Fatalf("expected 0 modified files, got %d", len(result.Modified))
	}

	infoAfter, err := os.Stat(mdFile)
	if err != nil {
		t.Fatal(err)
	}
	if !infoAfter.ModTime().Equal(mtimeBefore) {
		t.Errorf("mtime changed: before=%v after=%v", mtimeBefore, infoAfter.ModTime())
	}
}

func TestFix_ReadOnlyFileError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only file test not reliable on Windows")
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
		Rules:  []rule.Rule{&mockFixableRule{id: "TM100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{mdFile})
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error for read-only file, got %d: %v", len(result.Errors), result.Errors)
	}
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
		Rules:  []rule.Rule{&mockFixableRule{id: "TM100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{file1, file2})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Modified) != 2 {
		t.Fatalf("expected 2 modified files, got %d", len(result.Modified))
	}

	for _, f := range []string{file1, file2} {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
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
		Rules:  []rule.Rule{&mockFixableRule{id: "TM100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{})
	if len(result.Diagnostics) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", len(result.Diagnostics))
	}
	if len(result.Modified) != 0 {
		t.Errorf("expected 0 modified files, got %d", len(result.Modified))
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(result.Errors))
	}
}

func TestFix_ReParseBetweenPasses(t *testing.T) {
	// This test ensures that after one fixable rule modifies content, the next
	// fixable rule sees the updated source (re-parsed lint.File).
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	// Content has trailing tab+spaces: after TM100 strips trailing whitespace,
	// TM200 should not see any tabs (they were part of trailing whitespace).
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
			// TM100 sorts before TM200, so trailing whitespace removal happens first.
			&mockFixableRule{id: "TM100", name: "mock-trailing"},
			&mockFixableRuleB{id: "TM200", name: "mock-tabs"},
		},
	}

	result := fixer.Fix([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	content, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatal(err)
	}
	// TM100 strips trailing "\t " -> "# Hello\n"
	// TM200 sees "# Hello\n" which has no tabs -> no change.
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

func (r *mockRuleA) ID() string   { return r.id }
func (r *mockRuleA) Name() string { return r.name }

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
// trailing whitespace that mockFixableRule (TM100) would need to clean.
// But for the cross-rule regression test we use a different approach:
// mockRuleB replaces "AAA\nBBB" with "AAA\nREMOVE_ME\nBBB",
// simulating a fix that introduces a violation for mockRuleA.
type mockCrossRule struct {
	id   string
	name string
}

func (r *mockCrossRule) ID() string   { return r.id }
func (r *mockCrossRule) Name() string { return r.name }

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
	// mockRuleA (TM100) removes "REMOVE_ME" lines.
	// mockCrossRule (TM200) inserts "REMOVE_ME" between "AAA" and "BBB".
	// On a single pass: TM100 runs first (no REMOVE_ME yet, no-op),
	// then TM200 inserts REMOVE_ME. Without multi-pass, REMOVE_ME remains.
	// With multi-pass, the second pass lets TM100 remove it, and TM200
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
			&mockRuleA{id: "TM100", name: "mock-remove"},
			&mockCrossRule{id: "TM200", name: "mock-separator"},
		},
	}

	result := fixer.Fix([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	content, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatal(err)
	}

	// The fixes oscillate within each pass: TM100 removes REMOVE_ME,
	// TM200 re-inserts it. The pass output equals the previous pass
	// output so the loop detects stability and stops.
	// Final content has REMOVE_ME because TM200 runs after TM100.
	expected := "AAA\nREMOVE_ME\nBBB\n"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}

	// TM100 reports a remaining diagnostic for the REMOVE_ME line.
	found := false
	for _, d := range result.Diagnostics {
		if d.RuleID == "TM100" {
			found = true
		}
	}
	if !found {
		t.Error("expected remaining TM100 diagnostic")
	}
}

func TestFix_LaterRuleFixCaughtByEarlierRule(t *testing.T) {
	// Regression test for the actual bug: a later rule's fix introduces
	// trailing whitespace that an earlier rule should clean up.
	// mockFixableRule (TM100) strips trailing spaces.
	// mockTrailingAdder (TM200) fixes something but adds trailing spaces.
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	// "hello\tworld\n" — has a tab that TM200 will convert to spaces,
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
			&mockFixableRule{id: "TM100", name: "mock-trailing"},
			&mockSloppyTabFixer{id: "TM200", name: "mock-tabs-sloppy"},
		},
	}

	result := fixer.Fix([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	content, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatal(err)
	}

	// TM100 first pass: no trailing spaces, no-op.
	// TM200 first pass: replaces tab with "    " but adds trailing space
	//   -> "hello    world \n"
	// Second pass: TM100 strips trailing space -> "hello    world\n"
	// TM200: no tabs, no-op. Stable.
	expected := "hello    world\n"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}

	// No remaining diagnostics — both issues fully fixed.
	for _, d := range result.Diagnostics {
		if d.RuleID == "TM100" || d.RuleID == "TM200" {
			t.Errorf("unexpected remaining diagnostic: %s: %s", d.RuleID, d.Message)
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
		Rules:  []rule.Rule{&mockFixableRule{id: "TM100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{mdFile})
	if len(result.Modified) != 0 {
		t.Fatalf("expected 0 modified files for ignored file, got %d", len(result.Modified))
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics for ignored file, got %d", len(result.Diagnostics))
	}
}

func TestFix_NonexistentFileError(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "TM100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{"/nonexistent/file.md"})
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
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
		Rules:  []rule.Rule{&mockFixableRule{id: "TM100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	info, err := os.Stat(mdFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("expected permissions 0755, got %04o", info.Mode().Perm())
	}
}
