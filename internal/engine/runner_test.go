package engine

import (
	"os"
	"path/filepath"
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

func (r *silentRule) ID() string                       { return r.id }
func (r *silentRule) Name() string                     { return r.name }
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
	// After stripping, "# BAD heading" is line 1 of the content.
	if result.Diagnostics[0].Line != 1 {
		t.Errorf("expected line 1 (after stripping), got %d",
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
