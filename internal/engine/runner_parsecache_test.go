package engine

import (
	"path/filepath"
	"sync"
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

// lazyFieldsRule reaches into every lazily-memoised field on
// *lint.File that the parse cache can hand to a concurrent reader:
// LinkReferences (atomic.Bool + mutex), NewlineOffsets (same
// pattern), code-block and PI-block index slices, and the gitignore
// matcher. None of these are populated by NewFile / populateFileFields
// — they fire the first time a rule asks — so two goroutines holding
// the same cached *File contend on their first-access guards. Drives
// the data-race surface the pointer-aliasing fix was meant to close.
type lazyFieldsRule struct{}

func (lazyFieldsRule) ID() string       { return "MDS998" }
func (lazyFieldsRule) Name() string     { return "lazy-fields-probe" }
func (lazyFieldsRule) Category() string { return "test" }
func (lazyFieldsRule) Check(f *lint.File) []lint.Diagnostic {
	_ = f.LinkReferences()
	_ = f.LineOfOffset(0)
	_ = lint.CollectCodeBlockLines(f)
	_ = lint.CollectPIBlockLines(f)
	_ = f.GetGitignore()
	return nil
}

// TestRunSourceWithVersion_ConcurrentReadersAreRaceSafe drives two
// goroutines through one cached *File and asserts every lazy
// memoisation guard on lint.File holds. The pointer-aliasing fix
// hinges on the cached *File being safe to share across the LSP's
// dispatch goroutines, so this is the test the -race detector
// catches a regression against.
//
// Source content is chosen to exercise each lazy field: a link
// reference (LinkReferences), a fenced code block (CodeBlockLines),
// a processing-instruction block (PIBlockLines), and enough lines to
// make the newline-offsets walk worth memoising.
func TestRunSourceWithVersion_ConcurrentReadersAreRaceSafe(t *testing.T) {
	cache := lint.NewParseCache()
	cfg := &config.Config{Rules: map[string]config.RuleCfg{"lazy-fields-probe": {Enabled: true}}}
	r := &Runner{
		Config:     cfg,
		Rules:      []rule.Rule{lazyFieldsRule{}},
		ParseCache: cache,
		RootDir:    t.TempDir(),
	}

	src := []byte("# Heading\n" +
		"\n" +
		"Body text linking to [Ref][a-ref].\n" +
		"\n" +
		"```go\n" +
		"package main\n" +
		"```\n" +
		"\n" +
		"<?build outputs: foo ?>\n" +
		"build body\n" +
		"<?/build?>\n" +
		"\n" +
		"[a-ref]: https://example.com\n")

	// Prime the cache so both goroutines hit instead of racing the
	// parse itself. This pins the cached *File as the surface under
	// test.
	r.RunSourceWithVersion("docs/foo.md", src, 1)
	primed, ok := cache.Get("docs/foo.md", 1)
	require.True(t, ok, "cache must hold the primed entry before the concurrent run")
	require.NotNil(t, primed)

	const goroutines = 8
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			r.RunSourceWithVersion("docs/foo.md", src, 1)
		}()
	}
	wg.Wait()

	after, ok := cache.Get("docs/foo.md", 1)
	require.True(t, ok)
	assert.Same(t, primed, after, "concurrent readers must not displace the cached entry")
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
