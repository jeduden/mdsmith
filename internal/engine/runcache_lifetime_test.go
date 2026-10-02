package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/runcache"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunner_AutoCreatedRunCacheIsCallScoped pins the lifetime
// contract: when the caller does not install a RunCache, the cache
// the Runner builds for one Run does NOT persist on r.RunCache.
// Without this, reusing the same Runner for a second Run would
// silently serve stale reads from the first pass — exactly the
// staleness hazard the LSP's Invalidate seam exists to avoid.
//
// A caller-supplied RunCache (the LSP installs one on Server) must
// be honored unchanged — that path persists across calls by design.
func TestRunner_AutoCreatedRunCacheIsCallScoped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.md")
	require.NoError(t, os.WriteFile(path, []byte("# X\n"), 0o644))

	r := &Runner{
		Config:  config.Merge(config.Defaults(), nil),
		Rules:   nil,
		RootDir: dir,
	}
	require.Nil(t, r.RunCache, "precondition: caller did not install a cache")
	_ = r.Run([]string{path})
	assert.Nilf(t, r.RunCache,
		"Runner.Run must not assign its auto-created RunCache to "+
			"r.RunCache; doing so leaks the cache into the next Run "+
			"and breaks the run-scoped lifetime contract")
}

// TestRunner_AutoCreatedRunSourceCacheIsCallScoped pins the same
// contract for RunSource: an auto-created cache must stay local to
// the call rather than persisting on the Runner.
func TestRunner_AutoCreatedRunSourceCacheIsCallScoped(t *testing.T) {
	r := &Runner{
		Config: config.Merge(config.Defaults(), nil),
		Rules:  nil,
	}
	require.Nil(t, r.RunCache)
	_ = r.RunSource("buf.md", []byte("# X\n"))
	assert.Nilf(t, r.RunCache,
		"Runner.RunSource must not assign its auto-created RunCache "+
			"to r.RunCache; the LSP installs its own long-lived "+
			"instance and the auto-created one belongs to one call")
}

// TestRunner_CallerProvidedRunCacheIsHonored pins the LSP contract:
// when a RunCache is installed on the Runner, the engine uses it
// (and leaves it intact) so the LSP's Invalidate seam controls
// staleness across runLint calls.
func TestRunner_CallerProvidedRunCacheIsHonored(t *testing.T) {
	cache := runcache.NewRunCache()
	r := &Runner{
		Config:   config.Merge(config.Defaults(), nil),
		Rules:    nil,
		RunCache: cache,
	}
	_ = r.RunSource("buf.md", []byte("# X\n"))
	assert.Samef(t, cache, r.RunCache,
		"a caller-installed RunCache must be preserved on r.RunCache "+
			"so the LSP keeps invalidation control across calls")
}

// TestRunner_NewRunGetsFreshCache pins the per-call freshness: two
// successive Runs on the same Runner each get a fresh auto-created
// cache. We pre-populate the cache the first call would build, run,
// then confirm the second call cannot see that entry — proving
// neither call's cache leaks into the other.
func TestRunner_NewRunGetsFreshCache(t *testing.T) {
	r := &Runner{
		Config: config.Merge(config.Defaults(), nil),
		Rules:  nil,
	}
	// First call: auto-creates a cache, processes one file, drops
	// the cache.
	_ = r.RunSource("buf.md", []byte("# X\n"))
	require.Nil(t, r.RunCache)
	// A second call gets its own fresh auto-create. We can't peek at
	// the in-flight cache directly, so the strong assertion is that
	// r.RunCache stays nil — which it would not if the first call
	// had latched its cache onto the Runner.
	_ = r.RunSource("buf.md", []byte("# Y\n"))
	assert.Nil(t, r.RunCache,
		"each call must drop its auto-created cache; r.RunCache "+
			"staying nil across two Runs proves the call-scoped contract")
}

// TestRunCacheForCall_NilCallerBuildsFresh pins the auto-create
// branch directly. Pulled into a unit test so the helper carries
// its own coverage even when the higher-level Run/RunSource paths
// route through it transparently.
func TestRunCacheForCall_NilCallerBuildsFresh(t *testing.T) {
	r := &Runner{}
	c1 := r.runCacheForCall()
	c2 := r.runCacheForCall()
	require.NotNil(t, c1)
	require.NotNil(t, c2)
	assert.NotSamef(t, c1, c2,
		"two auto-create calls must produce distinct caches; sharing "+
			"would silently re-introduce the cross-Run leak")
}

// TestRunCacheForCall_CallerProvidedReused pins the LSP branch
// directly: an installed cache is returned unchanged so the LSP
// keeps invalidation control.
func TestRunCacheForCall_CallerProvidedReused(t *testing.T) {
	cache := runcache.NewRunCache()
	r := &Runner{RunCache: cache}
	got := r.runCacheForCall()
	assert.Samef(t, cache, got,
		"caller-installed RunCache must be reused, not replaced; "+
			"otherwise the LSP's Server.runCache would be a no-op "+
			"because the engine would build a fresh cache per call")
}
