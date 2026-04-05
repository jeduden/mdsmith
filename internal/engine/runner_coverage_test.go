package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
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

func TestCachedGitignore_EquivalentPaths(t *testing.T) {
	dir := t.TempDir()

	runner := &Runner{
		Config: &config.Config{},
		Rules:  nil,
	}

	// Use "." relative and absolute to verify normalization.
	m1 := runner.cachedGitignore(dir)
	m2 := runner.cachedGitignore(filepath.Join(dir, "."))
	assert.Same(t, m1, m2, "equivalent paths should share cache entry")
}

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
	runner.logRules(effective)
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

	runner.logRules(effective)
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

	runner.logRules(effective)
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

	runner.logRules(effective)
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
