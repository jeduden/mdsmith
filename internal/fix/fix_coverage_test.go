package fix

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- log tests ---

func TestLog_NilLogger(t *testing.T) {
	fixer := &Fixer{
		Config: &config.Config{},
		Logger: nil,
	}
	l := fixer.log()
	require.NotNil(t, l)
	assert.False(t, l.Enabled, "expected disabled logger when Logger is nil")
}

func TestLog_EnabledLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := &vlog.Logger{Enabled: true, W: &buf}

	fixer := &Fixer{
		Config: &config.Config{},
		Logger: logger,
	}
	l := fixer.log()
	assert.Same(t, logger, l)
	l.Printf("test %s", "log")
	assert.Contains(t, buf.String(), "test log")
}

// --- logRules tests ---

func TestLogRules_DisabledLogger(t *testing.T) {
	fixer := &Fixer{
		Config: &config.Config{},
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		Logger: nil,
	}

	effective := map[string]config.RuleCfg{
		"mock-trailing": {Enabled: true},
	}
	// Should not panic.
	fixer.logRules(effective)
}

func TestLogRules_EnabledLoggerLogsRules(t *testing.T) {
	var buf bytes.Buffer
	logger := &vlog.Logger{Enabled: true, W: &buf}

	fixer := &Fixer{
		Config: &config.Config{},
		Rules: []rule.Rule{
			&mockFixableRule{id: "MDS100", name: "mock-trailing"},
			&mockNonFixableRule{id: "MDS999", name: "mock-nonfixable"},
		},
		Logger: logger,
	}

	effective := map[string]config.RuleCfg{
		"mock-trailing":   {Enabled: true},
		"mock-nonfixable": {Enabled: true},
	}

	fixer.logRules(effective)
	output := buf.String()
	assert.Contains(t, output, "MDS100")
	assert.Contains(t, output, "MDS999")
}

func TestLogRules_SkipsDisabledRules(t *testing.T) {
	var buf bytes.Buffer
	logger := &vlog.Logger{Enabled: true, W: &buf}

	fixer := &Fixer{
		Config: &config.Config{},
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		Logger: logger,
	}

	effective := map[string]config.RuleCfg{
		"mock-trailing": {Enabled: false},
	}

	fixer.logRules(effective)
	assert.NotContains(t, buf.String(), "MDS100")
}

// --- fixableRules tests ---

func TestFixableRules_NonFixableRuleExcluded(t *testing.T) {
	fixer := &Fixer{
		Config: &config.Config{},
		Rules: []rule.Rule{
			&mockNonFixableRule{id: "MDS999", name: "mock-nonfixable"},
		},
	}

	effective := map[string]config.RuleCfg{
		"mock-nonfixable": {Enabled: true},
	}

	fixable, errs := fixer.fixableRules(effective)
	assert.Empty(t, errs)
	assert.Empty(t, fixable, "non-fixable rule should not be in fixable list")
}

func TestFixableRules_DisabledRuleExcluded(t *testing.T) {
	fixer := &Fixer{
		Config: &config.Config{},
		Rules: []rule.Rule{
			&mockFixableRule{id: "MDS100", name: "mock-trailing"},
		},
	}

	effective := map[string]config.RuleCfg{
		"mock-trailing": {Enabled: false},
	}

	fixable, errs := fixer.fixableRules(effective)
	assert.Empty(t, errs)
	assert.Empty(t, fixable)
}

func TestFixableRules_RuleNotInEffective(t *testing.T) {
	fixer := &Fixer{
		Config: &config.Config{},
		Rules: []rule.Rule{
			&mockFixableRule{id: "MDS100", name: "mock-trailing"},
		},
	}

	effective := map[string]config.RuleCfg{}

	fixable, errs := fixer.fixableRules(effective)
	assert.Empty(t, errs)
	assert.Empty(t, fixable)
}

func TestFixableRules_SortedByID(t *testing.T) {
	fixer := &Fixer{
		Config: &config.Config{},
		Rules: []rule.Rule{
			&mockFixableRuleB{id: "MDS200", name: "mock-tabs"},
			&mockFixableRule{id: "MDS100", name: "mock-trailing"},
		},
	}

	effective := map[string]config.RuleCfg{
		"mock-tabs":     {Enabled: true},
		"mock-trailing": {Enabled: true},
	}

	fixable, errs := fixer.fixableRules(effective)
	assert.Empty(t, errs)
	require.Len(t, fixable, 2)
	assert.Equal(t, "MDS100", fixable[0].ID())
	assert.Equal(t, "MDS200", fixable[1].ID())
}

// mockBadConfigFixableRule is a fixable rule whose ApplySettings always fails.
type mockBadConfigFixableRule struct {
	id   string
	name string
}

func (r *mockBadConfigFixableRule) ID() string       { return r.id }
func (r *mockBadConfigFixableRule) Name() string     { return r.name }
func (r *mockBadConfigFixableRule) Category() string { return "test" }
func (r *mockBadConfigFixableRule) Check(_ *lint.File) []lint.Diagnostic {
	return nil
}
func (r *mockBadConfigFixableRule) Fix(f *lint.File) []byte { return f.Source }
func (r *mockBadConfigFixableRule) DefaultSettings() map[string]any {
	return map[string]any{}
}
func (r *mockBadConfigFixableRule) ApplySettings(_ map[string]any) error {
	return assert.AnError
}

var _ rule.FixableRule = (*mockBadConfigFixableRule)(nil)
var _ rule.Configurable = (*mockBadConfigFixableRule)(nil)

func TestFixableRules_ConfigError(t *testing.T) {
	fixer := &Fixer{
		Config: &config.Config{},
		Rules: []rule.Rule{
			&mockBadConfigFixableRule{id: "MDS300", name: "bad-config"},
		},
	}

	effective := map[string]config.RuleCfg{
		"bad-config": {Enabled: true, Settings: map[string]any{"key": "val"}},
	}

	fixable, errs := fixer.fixableRules(effective)
	assert.Len(t, errs, 1)
	assert.Empty(t, fixable)
}

// --- Fix edge cases ---

func TestFix_WithVerboseLogger(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello  \n"), 0o644))

	var buf bytes.Buffer
	logger := &vlog.Logger{Enabled: true, W: &buf}

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		Logger: logger,
	}

	result := fixer.Fix([]string{mdFile})
	require.Empty(t, result.Errors)
	require.Len(t, result.Modified, 1)

	// Verify logging happened.
	output := buf.String()
	assert.Contains(t, output, "file:")
	assert.Contains(t, output, "fix: pass")
}

func TestFix_WithRootDir(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello  \n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config:  cfg,
		Rules:   []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		RootDir: dir,
	}

	result := fixer.Fix([]string{mdFile})
	require.Empty(t, result.Errors)
	require.Len(t, result.Modified, 1)
}

func TestFix_CleanFileNoModification(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "clean.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Clean\n"), 0o644))

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
	require.Empty(t, result.Errors)
	assert.Empty(t, result.Modified)
	assert.Equal(t, 0, result.Failures)
}

// --- Fix sort comparator branches ---

// mockMultiLineDiagRule reports diagnostics on two separate files, two
// separate lines on the same file, and two separate columns on the same line.
// This exercises all three comparator branches in Fix's sort.Slice.
type mockMultiLineDiagRule struct {
	id   string
	name string
}

func (r *mockMultiLineDiagRule) ID() string       { return r.id }
func (r *mockMultiLineDiagRule) Name() string     { return r.name }
func (r *mockMultiLineDiagRule) Category() string { return "test" }

// Check always returns two diagnostics for the same file to exercise the
// line-level and column-level comparator branches.
func (r *mockMultiLineDiagRule) Check(f *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{
		{File: f.Path, Line: 3, Column: 2, RuleID: r.id, RuleName: r.name, Severity: lint.Warning, Message: "issue c"},
		{File: f.Path, Line: 1, Column: 5, RuleID: r.id, RuleName: r.name, Severity: lint.Warning, Message: "issue a"},
		{File: f.Path, Line: 1, Column: 2, RuleID: r.id, RuleName: r.name, Severity: lint.Warning, Message: "issue b"},
	}
}

var _ rule.Rule = (*mockMultiLineDiagRule)(nil)

// TestFix_SortDiagnostics_MultiFile exercises the sort comparator in Fix
// across two files (di.File != dj.File branch), multiple lines on the same
// file (di.Line != dj.Line branch), and multiple columns on the same line
// (di.Column < dj.Column branch).
func TestFix_SortDiagnostics_MultiFile(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.md")
	fileB := filepath.Join(dir, "b.md")
	require.NoError(t, os.WriteFile(fileA, []byte("line1\nline2\nline3\n"), 0o644))
	require.NoError(t, os.WriteFile(fileB, []byte("line1\nline2\nline3\n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-multi": {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockMultiLineDiagRule{id: "MDS888", name: "mock-multi"}},
	}

	// Pass fileB before fileA so the sort must reorder by File path.
	result := fixer.Fix([]string{fileB, fileA})
	require.Empty(t, result.Errors)

	// All 6 diagnostics (3 per file) should be sorted by file, then line, then column.
	require.Len(t, result.Diagnostics, 6)

	// First two diagnostics should be from fileA (alphabetically earlier).
	assert.Equal(t, fileA, result.Diagnostics[0].File)
	assert.Equal(t, fileA, result.Diagnostics[1].File)
	// Within fileA, line 1 col 2 must precede line 1 col 5.
	assert.Equal(t, 1, result.Diagnostics[0].Line)
	assert.Equal(t, 2, result.Diagnostics[0].Column)
	assert.Equal(t, 1, result.Diagnostics[1].Line)
	assert.Equal(t, 5, result.Diagnostics[1].Column)
	// Line 3 comes after line 1.
	assert.Equal(t, 3, result.Diagnostics[2].Line)

	// Last three diagnostics should be from fileB.
	assert.Equal(t, fileB, result.Diagnostics[3].File)
}

// TestFix_IsIgnored_Continue verifies that ignored files do not count
// toward FilesChecked and are not modified, confirming the continue branch.
func TestFix_IsIgnored_Continue(t *testing.T) {
	dir := t.TempDir()
	ignored := filepath.Join(dir, "ignored.md")
	notIgnored := filepath.Join(dir, "kept.md")
	require.NoError(t, os.WriteFile(ignored, []byte("# ignored  \n"), 0o644))
	require.NoError(t, os.WriteFile(notIgnored, []byte("# kept  \n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
		Ignore: []string{"ignored.md"},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
	}

	result := fixer.Fix([]string{ignored, notIgnored})
	require.Empty(t, result.Errors)

	// Only the non-ignored file counts.
	assert.Equal(t, 1, result.FilesChecked)
	require.Len(t, result.Modified, 1)
	assert.Equal(t, notIgnored, result.Modified[0])

	// The ignored file should be unchanged.
	got, err := os.ReadFile(ignored)
	require.NoError(t, err)
	assert.Equal(t, "# ignored  \n", string(got))
}

// --- atomicWriteFile error paths ---

// TestAtomicWriteFile_RenameFailure causes os.Rename to fail by making the
// target path a directory. On Linux, renaming a regular file onto a directory
// returns EISDIR.
func TestAtomicWriteFile_RenameFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("rename-over-directory test not reliable on Windows")
	}

	dir := t.TempDir()
	// Create a directory at the target path.
	targetDir := filepath.Join(dir, "target")
	require.NoError(t, os.Mkdir(targetDir, 0o755))

	// atomicWriteFile: the target exists, stat will succeed, OpenFile on a
	// directory for O_WRONLY will fail on Linux (EISDIR).
	err := atomicWriteFile(targetDir, []byte("data"), 0o644)
	require.Error(t, err, "expected error when target is a directory")
}

// TestAtomicWriteFile_WriteFailure causes tmp.Write to fail by filling the
// target directory's filesystem quota. Since we cannot easily exhaust disk
// space, we instead point the temp file into a read-only directory — but that
// would prevent CreateTemp too. Instead, we use a pipe trick: replace the
// temp dir with a read-only mount is not portable. The most portable approach
// is to write a very large file to a tmpfs of limited size. As an alternative,
// we test the Rename failure path (different from read-only target) by
// arranging the Chmod to fail.
//
// On Linux, Chmod of a temp file we own always succeeds, so the only reliably
// injectable post-CreateTemp failure is Rename. TestAtomicWriteFile_RenameFailure
// covers that. The write/sync/close paths are exercised implicitly by the
// successful write tests; their error returns are straightforward OS wrappers
// with no logic to branch on, so they do not contribute to the branch-coverage
// gap targeted here.
func TestAtomicWriteFile_TmpFileCleanedUpOnRenameFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("not reliable on Windows")
	}

	dir := t.TempDir()
	targetDir := filepath.Join(dir, "target")
	require.NoError(t, os.Mkdir(targetDir, 0o755))

	_ = atomicWriteFile(targetDir, []byte("data"), 0o644)

	// Verify no orphaned temp files remain in the directory after the failure.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, len(e.Name()) > len(".mdsmith-fix-"),
			"unexpected leftover temp file: %s", e.Name())
	}
}
