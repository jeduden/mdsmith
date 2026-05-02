package engine

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

// --- cachedGitignore tests ---

func TestCachedGitignore_ReturnsMatcher(t *testing.T) {
	dir := t.TempDir()
	// Write a .gitignore so the matcher has something to parse.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644))

	runner := &Runner{
		Config: &config.Config{},
		Rules:  nil,
	}

	m := runner.cachedGitignore(dir)
	require.NotNil(t, m, "expected non-nil GitignoreMatcher")
}

func TestCachedGitignore_CacheHit(t *testing.T) {
	dir := t.TempDir()

	runner := &Runner{
		Config: &config.Config{},
		Rules:  nil,
	}

	m1 := runner.cachedGitignore(dir)
	m2 := runner.cachedGitignore(dir)
	// Same pointer means cache hit.
	assert.Same(t, m1, m2, "expected same matcher from cache")
}

// (dead test removed — filepath.Join(dir, ".") normalizes to dir, so both args are identical)

func TestCachedGitignore_InitializesNilCache(t *testing.T) {
	runner := &Runner{
		Config: &config.Config{},
		Rules:  nil,
	}
	// gitignoreCache starts nil.
	assert.Nil(t, runner.gitignoreCache)

	_ = runner.cachedGitignore(t.TempDir())
	assert.NotNil(t, runner.gitignoreCache, "expected cache map to be initialized")
}

// --- log tests ---

func TestLog_NilLogger(t *testing.T) {
	runner := &Runner{
		Config: &config.Config{},
		Rules:  nil,
		Logger: nil,
	}
	l := runner.log()
	require.NotNil(t, l, "expected non-nil logger even when Logger is nil")
	assert.False(t, l.Enabled, "expected disabled logger when Logger is nil")
}

func TestLog_EnabledLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := &vlog.Logger{Enabled: true, W: &buf}

	runner := &Runner{
		Config: &config.Config{},
		Rules:  nil,
		Logger: logger,
	}
	l := runner.log()
	assert.Same(t, logger, l, "expected same logger instance")
	l.Printf("test %s", "message")
	assert.Contains(t, buf.String(), "test message")
}

// --- logRules tests ---

func TestLogRules_DisabledLogger(t *testing.T) {
	runner := &Runner{
		Config: &config.Config{},
		Rules:  []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		Logger: nil,
	}

	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: true},
	}

	// Should not panic with nil logger.
	runner.logRules(runner.Rules, effective)
}

func TestLogRules_EnabledLoggerLogsRules(t *testing.T) {
	var buf bytes.Buffer
	logger := &vlog.Logger{Enabled: true, W: &buf}

	runner := &Runner{
		Config: &config.Config{},
		Rules: []rule.Rule{
			&mockRule{id: "MDS999", name: "mock-rule"},
			&silentRule{id: "MDS998", name: "silent-rule"},
		},
		Logger: logger,
	}

	effective := map[string]config.RuleCfg{
		"mock-rule":   {Enabled: true},
		"silent-rule": {Enabled: true},
	}

	runner.logRules(runner.Rules, effective)
	output := buf.String()
	assert.Contains(t, output, "MDS999")
	assert.Contains(t, output, "mock-rule")
	assert.Contains(t, output, "MDS998")
	assert.Contains(t, output, "silent-rule")
}

func TestLogRules_SkipsDisabledRules(t *testing.T) {
	var buf bytes.Buffer
	logger := &vlog.Logger{Enabled: true, W: &buf}

	runner := &Runner{
		Config: &config.Config{},
		Rules: []rule.Rule{
			&mockRule{id: "MDS999", name: "mock-rule"},
		},
		Logger: logger,
	}

	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: false},
	}

	runner.logRules(runner.Rules, effective)
	assert.NotContains(t, buf.String(), "MDS999")
}

func TestLogRules_SkipsRulesNotInEffective(t *testing.T) {
	var buf bytes.Buffer
	logger := &vlog.Logger{Enabled: true, W: &buf}

	runner := &Runner{
		Config: &config.Config{},
		Rules: []rule.Rule{
			&mockRule{id: "MDS999", name: "mock-rule"},
		},
		Logger: logger,
	}

	effective := map[string]config.RuleCfg{}

	runner.logRules(runner.Rules, effective)
	assert.Empty(t, buf.String())
}

// --- RunSource with RootDir ---

func TestRunSource_RootDirSetsRootFS(t *testing.T) {
	dir := t.TempDir()

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"silent-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config:  cfg,
		Rules:   []rule.Rule{&silentRule{id: "MDS998", name: "silent-rule"}},
		RootDir: dir,
	}

	result := runner.RunSource("<stdin>", []byte("# Hello\n"))
	require.Len(t, result.Errors, 0)
	assert.Equal(t, 1, result.FilesChecked)
}

// --- Run with RootDir ---

func TestRun_RootDirSetsGitignoreFunc(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello\n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"silent-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config:  cfg,
		Rules:   []rule.Rule{&silentRule{id: "MDS998", name: "silent-rule"}},
		RootDir: dir,
	}

	result := runner.Run([]string{mdFile})
	require.Len(t, result.Errors, 0)
	assert.Equal(t, 1, result.FilesChecked)
}

func TestDedupeDiagnostics_RemovesDuplicates(t *testing.T) {
	d := func(file string, line, col int, ruleID, msg string) lint.Diagnostic {
		return lint.Diagnostic{
			File: file, Line: line, Column: col,
			RuleID: ruleID, RuleName: ruleID, Severity: lint.Warning,
			Message: msg,
		}
	}
	in := []lint.Diagnostic{
		d(".gitattributes", 1, 1, "MDS048", "drift"),
		d(".gitattributes", 1, 1, "MDS048", "drift"),
		d("README.md", 5, 1, "MDS001", "long line"),
		d(".gitattributes", 1, 1, "MDS048", "drift"),
		d("README.md", 5, 1, "MDS001", "long line"),
	}
	got := DedupeDiagnostics(in)
	require.Len(t, got, 2, "duplicates collapse to one entry per (file, line, col, rule, message)")
	assert.Equal(t, "MDS048", got[0].RuleID)
	assert.Equal(t, "MDS001", got[1].RuleID)
}

func TestDedupeDiagnostics_PreservesDistinctMessages(t *testing.T) {
	d := func(msg string) lint.Diagnostic {
		return lint.Diagnostic{
			File: "f", Line: 1, Column: 1,
			RuleID: "X", Message: msg,
		}
	}
	in := []lint.Diagnostic{d("a"), d("b"), d("a")}
	got := DedupeDiagnostics(in)
	assert.Len(t, got, 2, "different messages at same coordinates remain distinct")
}

func TestDedupeDiagnostics_HandlesShortInput(t *testing.T) {
	assert.Nil(t, DedupeDiagnostics(nil))

	one := []lint.Diagnostic{{File: "f", RuleID: "X"}}
	got := DedupeDiagnostics(one)
	assert.Equal(t, one, got, "single-element input round-trips by content")

	// Result must be a freshly-allocated slice so mutating it does
	// not corrupt the caller's input. Mutate the result's first
	// element and confirm the input is unchanged.
	got[0].File = "mutated"
	assert.Equal(t, "f", one[0].File,
		"single-element result must not alias the input slice")
}
