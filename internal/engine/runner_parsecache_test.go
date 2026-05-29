package engine

import (
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingRule reports one diagnostic per Check call and counts how
// many times Check fired. Used to prove a ParseCache hit still runs
// the rule pipeline against the cached *File — the cache must skip
// the parse, not the check.
type countingRule struct {
	id    string
	name  string
	calls int
}

func (r *countingRule) ID() string       { return r.id }
func (r *countingRule) Name() string     { return r.name }
func (r *countingRule) Category() string { return "test" }
func (r *countingRule) Check(f *lint.File) []lint.Diagnostic {
	r.calls++
	return []lint.Diagnostic{
		{File: f.Path, Line: 1, Column: 1, RuleID: r.id, RuleName: r.name,
			Severity: lint.Warning, Message: "mock"},
	}
}

// TestRunSourceWithVersion_CacheMissParsesAndStores pins the cold
// path: an empty cache + RunSourceWithVersion must parse the source
// and store a *File keyed at (path, version) for the next call.
func TestRunSourceWithVersion_CacheMissParsesAndStores(t *testing.T) {
	cache := lint.NewParseCache()
	cfg := &config.Config{Rules: map[string]config.RuleCfg{"mock-rule": {Enabled: true}}}
	r := &Runner{
		Config:     cfg,
		Rules:      []rule.Rule{&countingRule{id: "MDS999", name: "mock-rule"}},
		ParseCache: cache,
	}

	res := r.RunSourceWithVersion("docs/foo.md", []byte("# Title\n"), 1)
	require.Empty(t, res.Errors)
	require.Len(t, res.Diagnostics, 1)

	got, ok := cache.Get("docs/foo.md", 1)
	require.True(t, ok, "ParseCache must contain an entry after a miss")
	assert.NotNil(t, got)
	assert.Equal(t, "docs/foo.md", got.Path)
}

// TestRunSourceWithVersion_CacheHitSkipsParse pins the warm path:
// a second RunSourceWithVersion call at the same (path, version)
// must reuse the cached *File. The rule still runs, but the *File
// it sees is the same instance the first call produced.
func TestRunSourceWithVersion_CacheHitSkipsParse(t *testing.T) {
	cache := lint.NewParseCache()
	cfg := &config.Config{Rules: map[string]config.RuleCfg{"mock-rule": {Enabled: true}}}
	rl := &countingRule{id: "MDS999", name: "mock-rule"}
	r := &Runner{Config: cfg, Rules: []rule.Rule{rl}, ParseCache: cache}

	r.RunSourceWithVersion("docs/foo.md", []byte("# Title\n"), 1)
	first, _ := cache.Get("docs/foo.md", 1)
	require.NotNil(t, first)

	r.RunSourceWithVersion("docs/foo.md", []byte("# Title\n"), 1)
	second, ok := cache.Get("docs/foo.md", 1)
	require.True(t, ok)
	assert.Same(t, first, second, "warm hit must return the same *File the first parse stored")
	assert.Equal(t, 2, rl.calls, "rule must still run on each call; only the parse is skipped")
}

// TestRunSourceWithVersion_VersionBumpReparses pins the staleness
// boundary: a higher version forces a fresh parse and overwrites the
// stored entry. The cached *File from version 1 must not survive a
// call at version 2.
func TestRunSourceWithVersion_VersionBumpReparses(t *testing.T) {
	cache := lint.NewParseCache()
	cfg := &config.Config{Rules: map[string]config.RuleCfg{"mock-rule": {Enabled: true}}}
	r := &Runner{
		Config:     cfg,
		Rules:      []rule.Rule{&countingRule{id: "MDS999", name: "mock-rule"}},
		ParseCache: cache,
	}

	r.RunSourceWithVersion("docs/foo.md", []byte("# Old\n"), 1)
	oldFile, _ := cache.Get("docs/foo.md", 1)
	require.NotNil(t, oldFile)

	r.RunSourceWithVersion("docs/foo.md", []byte("# New\n"), 2)
	newFile, ok := cache.Get("docs/foo.md", 2)
	require.True(t, ok)
	assert.NotSame(t, oldFile, newFile, "version bump must produce a fresh *File")

	// The old version's lookup now misses — the newer Put evicted it.
	_, ok = cache.Get("docs/foo.md", 1)
	assert.False(t, ok)
}

// TestRunSource_NilCacheIsCold pins that callers who never set
// ParseCache (mdsmith check, embedded hosts) take the cold path
// unchanged. RunSource is the legacy entry; it parses every call.
func TestRunSource_NilCacheIsCold(t *testing.T) {
	cfg := &config.Config{Rules: map[string]config.RuleCfg{"mock-rule": {Enabled: true}}}
	r := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&countingRule{id: "MDS999", name: "mock-rule"}},
	}

	res := r.RunSource("docs/foo.md", []byte("# Title\n"))
	require.Empty(t, res.Errors)
	require.Len(t, res.Diagnostics, 1)
}

// TestRunSource_AbsPathWiresGitignoreFromFileDir pins the
// no-RootDir branch in populateFileFields: when the Runner has no
// RootDir and the caller hands in an absolute path, the gitignore
// hook anchors at filepath.Dir(path) so a stdin-style absolute
// caller still gets a gitignore matcher rooted alongside the file.
func TestRunSource_AbsPathWiresGitignoreFromFileDir(t *testing.T) {
	cfg := &config.Config{Rules: map[string]config.RuleCfg{"mock-rule": {Enabled: true}}}
	r := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&countingRule{id: "MDS999", name: "mock-rule"}},
	}

	absPath := filepath.Join(t.TempDir(), "foo.md")
	res := r.RunSource(absPath, []byte("# Title\n"))
	require.Empty(t, res.Errors)
	require.Len(t, res.Diagnostics, 1)
}
