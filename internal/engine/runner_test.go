package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/tidymark/internal/config"
	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
)

// mockRule is a test rule that always reports a diagnostic on line 1.
type mockRule struct {
	id   string
	name string
}

func (r *mockRule) ID() string   { return r.id }
func (r *mockRule) Name() string { return r.name }
func (r *mockRule) Check(f *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{
		{
			File:     f.Path,
			Line:     1,
			Column:   1,
			RuleID:   r.id,
			RuleName: r.name,
			Severity: lint.Warning,
			Message:  "mock violation",
		},
	}
}

// silentRule is a test rule that never reports any diagnostics.
type silentRule struct {
	id   string
	name string
}

func (r *silentRule) ID() string                           { return r.id }
func (r *silentRule) Name() string                         { return r.name }
func (r *silentRule) Check(_ *lint.File) []lint.Diagnostic { return nil }

func TestRunner_MockRuleReportsDiagnostics(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}},
	}

	result := runner.Run([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(result.Diagnostics))
	}
	d := result.Diagnostics[0]
	if d.RuleID != "TM999" {
		t.Errorf("expected RuleID TM999, got %s", d.RuleID)
	}
	if d.Message != "mock violation" {
		t.Errorf("expected message %q, got %q", "mock violation", d.Message)
	}
}

func TestRunner_SilentRuleNoDiagnostics(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "clean.md")
	if err := os.WriteFile(mdFile, []byte("# Clean file\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"silent-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&silentRule{id: "TM998", name: "silent-rule"}},
	}

	result := runner.Run([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(result.Diagnostics))
	}
}

func TestRunner_DisabledRuleSkipped(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: false},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}},
	}

	result := runner.Run([]string{mdFile})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics for disabled rule, got %d", len(result.Diagnostics))
	}
}

func TestRunner_RuleNotInConfigSkipped(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}},
	}

	result := runner.Run([]string{mdFile})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics for unconfigured rule, got %d", len(result.Diagnostics))
	}
}

func TestRunner_NonexistentFileError(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}},
	}

	result := runner.Run([]string{"/nonexistent/file.md"})
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestRunner_IgnoredFileSkipped(t *testing.T) {
	dir := t.TempDir()
	vendorDir := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mdFile := filepath.Join(vendorDir, "lib.md")
	if err := os.WriteFile(mdFile, []byte("# Vendor\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
		Ignore: []string{"vendor/**"},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}},
	}

	result := runner.Run([]string{filepath.Join("vendor", "lib.md")})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics for ignored file, got %d", len(result.Diagnostics))
	}
}

func TestRunner_OverrideDisablesRuleForFile(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(changelog, []byte("# Changelog\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readme, []byte("# Readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
		Overrides: []config.Override{
			{
				Files: []string{"**/CHANGELOG.md"},
				Rules: map[string]config.RuleCfg{
					"mock-rule": {Enabled: false},
				},
			},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}},
	}

	result := runner.Run([]string{changelog, readme})

	// README.md should have a diagnostic, CHANGELOG.md should not.
	if len(result.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].File != readme {
		t.Errorf("expected diagnostic for %s, got %s", readme, result.Diagnostics[0].File)
	}
}

func TestRunner_DiagnosticsSortedByFileLineColumn(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.md")
	fileB := filepath.Join(dir, "b.md")
	if err := os.WriteFile(fileA, []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("# B\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mr := &multiDiagRuleImpl{
		id:   "TM997",
		name: "multi-diag",
		diags: map[string][]lint.Diagnostic{
			fileB: {
				{File: fileB, Line: 3, Column: 1, RuleID: "TM997", RuleName: "multi-diag", Severity: lint.Warning, Message: "b3"},
				{File: fileB, Line: 1, Column: 5, RuleID: "TM997", RuleName: "multi-diag", Severity: lint.Warning, Message: "b1c5"},
				{File: fileB, Line: 1, Column: 1, RuleID: "TM997", RuleName: "multi-diag", Severity: lint.Warning, Message: "b1c1"},
			},
			fileA: {
				{File: fileA, Line: 2, Column: 1, RuleID: "TM997", RuleName: "multi-diag", Severity: lint.Warning, Message: "a2"},
				{File: fileA, Line: 1, Column: 1, RuleID: "TM997", RuleName: "multi-diag", Severity: lint.Warning, Message: "a1"},
			},
		},
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"multi-diag": {Enabled: true},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{mr},
	}

	// Pass files in reverse order to ensure sorting works.
	result := runner.Run([]string{fileB, fileA})

	if len(result.Diagnostics) != 5 {
		t.Fatalf("expected 5 diagnostics, got %d", len(result.Diagnostics))
	}

	// Expected order: a.md:1:1, a.md:2:1, b.md:1:1, b.md:1:5, b.md:3:1
	expected := []struct {
		file    string
		line    int
		column  int
		message string
	}{
		{fileA, 1, 1, "a1"},
		{fileA, 2, 1, "a2"},
		{fileB, 1, 1, "b1c1"},
		{fileB, 1, 5, "b1c5"},
		{fileB, 3, 1, "b3"},
	}

	for i, exp := range expected {
		d := result.Diagnostics[i]
		if d.File != exp.file || d.Line != exp.line || d.Column != exp.column {
			t.Errorf("diagnostic[%d]: expected %s:%d:%d, got %s:%d:%d",
				i, exp.file, exp.line, exp.column, d.File, d.Line, d.Column)
		}
	}
}

// multiDiagRuleImpl returns pre-configured diagnostics based on the file path.
type multiDiagRuleImpl struct {
	id    string
	name  string
	diags map[string][]lint.Diagnostic
}

func (r *multiDiagRuleImpl) ID() string   { return r.id }
func (r *multiDiagRuleImpl) Name() string { return r.name }
func (r *multiDiagRuleImpl) Check(f *lint.File) []lint.Diagnostic {
	return r.diags[f.Path]
}

func TestRunner_MultipleFilesLinted(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		filepath.Join(dir, "one.md"),
		filepath.Join(dir, "two.md"),
		filepath.Join(dir, "three.md"),
	}
	for _, f := range files {
		if err := os.WriteFile(f, []byte("# Test\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}},
	}

	result := runner.Run(files)
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Diagnostics) != 3 {
		t.Fatalf("expected 3 diagnostics (one per file), got %d", len(result.Diagnostics))
	}
}

func TestRunner_EmptyPaths(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}},
	}

	result := runner.Run([]string{})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(result.Diagnostics))
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected 0 errors, got %d", len(result.Errors))
	}
}

// contentMockRule flags lines containing the string "BAD".
type contentMockRule struct {
	id   string
	name string
}

func (r *contentMockRule) ID() string   { return r.id }
func (r *contentMockRule) Name() string { return r.name }

func (r *contentMockRule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		s := string(line)
		for j := 0; j+3 <= len(s); j++ {
			if s[j:j+3] == "BAD" {
				diags = append(diags, lint.Diagnostic{
					File: f.Path, Line: i + 1, Column: j + 1,
					RuleID: r.id, RuleName: r.name,
					Severity: lint.Warning, Message: "found BAD",
				})
				break
			}
		}
	}
	return diags
}

func TestRunner_StripFrontMatter(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	// "BAD" appears in front matter only.
	content := "---\ntitle: BAD\n---\n# Heading\n\nGood content.\n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"content-mock": {Enabled: true},
		},
	}

	// With stripping — "BAD" in front matter is removed.
	runner := &Runner{
		Config:           cfg,
		Rules:            []rule.Rule{&contentMockRule{id: "TM999", name: "content-mock"}},
		StripFrontMatter: true,
	}
	result := runner.Run([]string{mdFile})
	if len(result.Diagnostics) != 0 {
		t.Errorf("with stripping: expected 0 diagnostics, got %d",
			len(result.Diagnostics))
	}

	// Without stripping — "BAD" in front matter IS flagged.
	runner.StripFrontMatter = false
	result = runner.Run([]string{mdFile})
	if len(result.Diagnostics) != 1 {
		t.Errorf("without stripping: expected 1 diagnostic, got %d",
			len(result.Diagnostics))
	}
}

func TestRunner_StripFrontMatter_ContentStillLinted(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	// "BAD" appears in content after front matter.
	content := "---\ntitle: ok\n---\n# BAD heading\n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"content-mock": {Enabled: true},
		},
	}

	runner := &Runner{
		Config:           cfg,
		Rules:            []rule.Rule{&contentMockRule{id: "TM999", name: "content-mock"}},
		StripFrontMatter: true,
	}
	result := runner.Run([]string{mdFile})
	if len(result.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic for BAD in content, got %d",
			len(result.Diagnostics))
	}
	// Line numbers must reflect the original file, not the stripped content.
	// "# BAD heading" is line 4 in the original file (after 3 front-matter lines).
	if result.Diagnostics[0].Line != 4 {
		t.Errorf("expected line 4 (original file), got %d",
			result.Diagnostics[0].Line)
	}
}

func TestRunner_InvalidIgnorePatternSkipped(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
		Ignore: []string{"[invalid"},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}},
	}

	result := runner.Run([]string{mdFile})
	// Invalid glob is silently skipped; file is still linted.
	if len(result.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(result.Diagnostics))
	}
}

func TestRunner_IgnoredFileByBasename(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(mdFile, []byte("# Changelog\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
		Ignore: []string{"CHANGELOG.md"},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}},
	}

	result := runner.Run([]string{mdFile})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics for ignored file, got %d", len(result.Diagnostics))
	}
}

// configurableLengthRule checks lines > Max chars. It implements Configurable.
type configurableLengthRule struct {
	Max int
}

func (r *configurableLengthRule) ID() string   { return "TM001" }
func (r *configurableLengthRule) Name() string { return "line-length" }
func (r *configurableLengthRule) Check(f *lint.File) []lint.Diagnostic {
	max := r.Max
	if max <= 0 {
		max = 80
	}
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		if len(line) > max {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     i + 1,
				Column:   max + 1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  fmt.Sprintf("line too long (%d > %d)", len(line), max),
			})
		}
	}
	return diags
}
func (r *configurableLengthRule) ApplySettings(settings map[string]any) error {
	if v, ok := settings["max"]; ok {
		switch n := v.(type) {
		case int:
			r.Max = n
		case float64:
			r.Max = int(n)
		default:
			return fmt.Errorf("max must be int, got %T", v)
		}
	}
	return nil
}
func (r *configurableLengthRule) DefaultSettings() map[string]any {
	return map[string]any{"max": 80}
}

var _ rule.Configurable = (*configurableLengthRule)(nil)

func TestRunner_AppliesSettingsFromConfig(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	// Create a line that is 100 chars wide.
	line := strings.Repeat("a", 100) + "\n"
	if err := os.WriteFile(mdFile, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	// Configure line-length with max=120 — 100-char line should NOT trigger.
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {
				Enabled:  true,
				Settings: map[string]any{"max": 120},
			},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&configurableLengthRule{Max: 80}},
	}

	result := runner.Run([]string{mdFile})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics with max=120 for 100-char line, got %d: %v",
			len(result.Diagnostics), result.Diagnostics)
	}
}

func TestRunner_DefaultMaxWithoutSettings(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	// Create a line that is 100 chars wide.
	line := strings.Repeat("a", 100) + "\n"
	if err := os.WriteFile(mdFile, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	// No settings — default max=80 should flag the 100-char line.
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&configurableLengthRule{Max: 80}},
	}

	result := runner.Run([]string{mdFile})
	if len(result.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic with default max=80, got %d", len(result.Diagnostics))
	}
}

func TestRunner_SettingsDoNotLeakBetweenFiles(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.md")
	fileB := filepath.Join(dir, "b.md")
	line := strings.Repeat("a", 100) + "\n"
	if err := os.WriteFile(fileA, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	// No per-file override; both files use same settings.
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {
				Enabled:  true,
				Settings: map[string]any{"max": 120},
			},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&configurableLengthRule{Max: 80}},
	}

	result := runner.Run([]string{fileA, fileB})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(result.Diagnostics))
	}
}

// --- RunSource tests ---

func TestRunSource_BasicDiagnostics(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}},
	}

	result := runner.RunSource("<stdin>", []byte("# Hello\n"))
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(result.Diagnostics))
	}
	if result.Diagnostics[0].File != "<stdin>" {
		t.Errorf("expected file <stdin>, got %s", result.Diagnostics[0].File)
	}
}

func TestRunSource_FrontMatterLineOffset(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config:           cfg,
		Rules:            []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}},
		StripFrontMatter: true,
	}

	source := []byte("---\ntitle: x\n---\n# Heading\n")
	result := runner.RunSource("<stdin>", source)
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(result.Diagnostics))
	}
	// mockRule reports line 1; front matter has 3 lines, so adjusted = 4.
	if result.Diagnostics[0].Line != 4 {
		t.Errorf("expected adjusted line 4, got %d", result.Diagnostics[0].Line)
	}
}

func TestRunSource_AppliesConfigurableSettings(t *testing.T) {
	// 100-char line with max=120 should not trigger.
	line := strings.Repeat("a", 100) + "\n"

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {
				Enabled:  true,
				Settings: map[string]any{"max": 120},
			},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&configurableLengthRule{Max: 80}},
	}

	result := runner.RunSource("<stdin>", []byte(line))
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics with max=120, got %d: %v",
			len(result.Diagnostics), result.Diagnostics)
	}
}

func TestRunSource_EmptyInput(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&silentRule{id: "TM998", name: "mock-rule"}},
	}

	result := runner.RunSource("<stdin>", []byte(""))
	if len(result.Errors) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(result.Errors), result.Errors)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %v", len(result.Diagnostics), result.Diagnostics)
	}
}
