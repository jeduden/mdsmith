package fix

import (
	"bytes"
	"os"
	"path/filepath"
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
