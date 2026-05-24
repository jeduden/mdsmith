package lint

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunCache_FrontMatterBuildsOnce pins that the build closure runs
// exactly once per absPath: the run-scoped cache's whole purpose is to
// stop the catalog rule from re-reading the same target once per host
// file that globs it.
func TestRunCache_FrontMatterBuildsOnce(t *testing.T) {
	c := NewRunCache()

	var calls int32
	build := func() any {
		atomic.AddInt32(&calls, 1)
		return map[string]any{"title": "Alpha"}
	}

	for i := 0; i < 3; i++ {
		got := c.FrontMatter("/abs/x.md", build)
		require.Equal(t, map[string]any{"title": "Alpha"}, got)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"build must run exactly once per absPath")
}

// TestRunCache_IncludesBuildsOnce pins the same single-build guarantee
// for the include-adjacency cache.
func TestRunCache_IncludesBuildsOnce(t *testing.T) {
	c := NewRunCache()

	var calls int32
	build := func() []string {
		atomic.AddInt32(&calls, 1)
		return []string{"/abs/a.md", "/abs/b.md"}
	}

	for i := 0; i < 3; i++ {
		got := c.Includes("/abs/x.md", build)
		require.Equal(t, []string{"/abs/a.md", "/abs/b.md"}, got)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"include-adjacency build must run exactly once per absPath")
}

// TestRunCache_DistinctKeysDoNotShare pins that two absPaths are
// independent: caching one must not silently serve the other.
func TestRunCache_DistinctKeysDoNotShare(t *testing.T) {
	c := NewRunCache()

	c.FrontMatter("/abs/x.md", func() any { return "x-data" })
	c.FrontMatter("/abs/y.md", func() any { return "y-data" })

	assert.Equal(t, "x-data", c.FrontMatter("/abs/x.md",
		func() any { return "different" }))
	assert.Equal(t, "y-data", c.FrontMatter("/abs/y.md",
		func() any { return "different" }))
}

// TestRunCache_InvalidateForcesRebuild pins the LSP invalidation seam:
// after Invalidate(absPath) the next FrontMatter / Includes call for
// absPath must re-run build. Without this hook a long-lived server
// would serve a stale catalog body after the user edits a target.
func TestRunCache_InvalidateForcesRebuild(t *testing.T) {
	c := NewRunCache()

	var fmCalls, incCalls int32
	c.FrontMatter("/abs/x.md", func() any {
		atomic.AddInt32(&fmCalls, 1)
		return "v1"
	})
	c.Includes("/abs/x.md", func() []string {
		atomic.AddInt32(&incCalls, 1)
		return []string{"a"}
	})

	c.Invalidate("/abs/x.md")

	v := c.FrontMatter("/abs/x.md", func() any {
		atomic.AddInt32(&fmCalls, 1)
		return "v2"
	})
	inc := c.Includes("/abs/x.md", func() []string {
		atomic.AddInt32(&incCalls, 1)
		return []string{"b"}
	})

	assert.Equal(t, "v2", v, "Invalidate must clear the front matter slot")
	assert.Equal(t, []string{"b"}, inc, "Invalidate must clear the includes slot")
	assert.Equal(t, int32(2), atomic.LoadInt32(&fmCalls),
		"build must run again after Invalidate")
	assert.Equal(t, int32(2), atomic.LoadInt32(&incCalls),
		"include build must run again after Invalidate")
}

// TestRunCache_InvalidateMissingKeyIsNoop pins that Invalidate on a
// path that was never cached does not panic and leaves other keys
// untouched.
func TestRunCache_InvalidateMissingKeyIsNoop(t *testing.T) {
	c := NewRunCache()
	c.FrontMatter("/abs/x.md", func() any { return "kept" })

	c.Invalidate("/abs/never-seen.md")

	got := c.FrontMatter("/abs/x.md", func() any { return "rebuilt" })
	assert.Equal(t, "kept", got,
		"Invalidate on a missing key must not evict unrelated entries")
}

// TestRunCache_ConcurrentSingleBuild pins that build runs exactly once
// even when many goroutines race for the same key — the cache is read
// by the parallel worker pool and by the LSP's concurrent readers.
func TestRunCache_ConcurrentSingleBuild(t *testing.T) {
	c := NewRunCache()

	var calls int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := c.FrontMatter("/abs/shared.md", func() any {
				atomic.AddInt32(&calls, 1)
				return "once"
			})
			assert.Equal(t, "once", v)
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"build must run exactly once under concurrent access")
}

// TestRunCache_AnchorsBuildsOnce pins that the anchor cache caches
// successful builds: the second call with the same absPath returns
// the cached map without re-invoking build. Mirrors the
// FrontMatter/Includes pattern but for the anchor slot the
// cross-file rule consumes.
func TestRunCache_AnchorsBuildsOnce(t *testing.T) {
	c := NewRunCache()
	var calls int32
	anchors1, err := c.Anchors("/abs/target.md", func() (map[string]bool, error) {
		atomic.AddInt32(&calls, 1)
		return map[string]bool{"intro": true}, nil
	})
	require.NoError(t, err)
	require.True(t, anchors1["intro"])

	anchors2, err := c.Anchors("/abs/target.md", func() (map[string]bool, error) {
		atomic.AddInt32(&calls, 1)
		return nil, nil
	})
	require.NoError(t, err)
	require.True(t, anchors2["intro"])
	require.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"Anchors build must run exactly once per absPath")
}

// TestRunCache_AnchorsErrorIsRetryable pins that a failing build
// does not flip the done flag: the next caller's build runs again.
// Matches the catalog-rule semantics where a transient read
// failure on a missing target should produce a host-side
// diagnostic without poisoning the cache for sibling host files.
func TestRunCache_AnchorsErrorIsRetryable(t *testing.T) {
	c := NewRunCache()
	failure := assert.AnError
	var calls int32
	_, err := c.Anchors("/abs/oops.md", func() (map[string]bool, error) {
		atomic.AddInt32(&calls, 1)
		return nil, failure
	})
	require.ErrorIs(t, err, failure)

	_, err = c.Anchors("/abs/oops.md", func() (map[string]bool, error) {
		atomic.AddInt32(&calls, 1)
		return nil, failure
	})
	require.ErrorIs(t, err, failure)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls),
		"a failed build must not be cached; the second caller's build must run again")
}

// TestRunCache_AnchorsInvalidateForcesRebuild pins that Invalidate
// drops the anchors slot alongside FrontMatter and Includes. Without
// this the LSP edits-to-document → next-Check flow would serve stale
// cross-file diagnostics from the cached anchor set.
func TestRunCache_AnchorsInvalidateForcesRebuild(t *testing.T) {
	c := NewRunCache()
	var calls int32
	build := func(value string) func() (map[string]bool, error) {
		return func() (map[string]bool, error) {
			atomic.AddInt32(&calls, 1)
			return map[string]bool{value: true}, nil
		}
	}

	a1, err := c.Anchors("/abs/x.md", build("v1"))
	require.NoError(t, err)
	require.True(t, a1["v1"])

	c.Invalidate("/abs/x.md")

	a2, err := c.Anchors("/abs/x.md", build("v2"))
	require.NoError(t, err)
	require.True(t, a2["v2"], "Invalidate must clear the anchors slot")
	require.Equal(t, int32(2), atomic.LoadInt32(&calls),
		"Anchors build must run again after Invalidate")
}

// TestRunCache_AnchorsConcurrentSingleBuild pins the per-key mutex:
// concurrent callers race for the entry but the build runs exactly
// once, and every caller observes the same map.
func TestRunCache_AnchorsConcurrentSingleBuild(t *testing.T) {
	c := NewRunCache()
	var calls int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := c.Anchors("/abs/shared.md", func() (map[string]bool, error) {
				atomic.AddInt32(&calls, 1)
				return map[string]bool{"x": true}, nil
			})
			assert.NoError(t, err)
			assert.True(t, a["x"])
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"Anchors build must run exactly once under concurrent access")
}

func TestRunCache_WikilinksBuildsOnce(t *testing.T) {
	c := NewRunCache()
	var calls int32
	build := func() any {
		atomic.AddInt32(&calls, 1)
		return "index-instance"
	}
	for i := 0; i < 4; i++ {
		got := c.Wikilinks("/root/a", build)
		require.Equal(t, "index-instance", got)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"Wikilinks must build exactly once per root key")
}

func TestRunCache_WikilinksConcurrent(t *testing.T) {
	c := NewRunCache()
	var calls int32
	build := func() any {
		atomic.AddInt32(&calls, 1)
		return "idx"
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Wikilinks("/root/a", build)
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"concurrent callers must share one build")
}

func TestRunCache_InvalidateWikilinks(t *testing.T) {
	c := NewRunCache()
	var calls int32
	build := func() any {
		atomic.AddInt32(&calls, 1)
		return "v"
	}
	_ = c.Wikilinks("/root/a", build)
	c.InvalidateWikilinks()
	_ = c.Wikilinks("/root/a", build)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls),
		"InvalidateWikilinks must let the next call rebuild")
}

// TestRunCache_ParsedSchemaBuildsOnce pins that the parsed-schema cache
// closes MDS020's per-host-file re-parse hot spot: a schema file
// referenced by N host files is parsed exactly once per run, matching
// the FrontMatter / Includes single-build guarantee.
func TestRunCache_ParsedSchemaBuildsOnce(t *testing.T) {
	c := NewRunCache()
	var calls int32
	build := func() any {
		atomic.AddInt32(&calls, 1)
		return "parsed-schema-instance"
	}
	for i := 0; i < 3; i++ {
		got := c.ParsedSchema("/abs/schema.md", build)
		require.Equal(t, "parsed-schema-instance", got)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"ParsedSchema build must run exactly once per absPath")
}

// TestRunCache_ParsedSchemaCachesErrors pins that ParsedSchema caches
// the build result wholesale, including parse errors. A malformed
// schema file must not be re-read on every host file that references
// it; the cached value carries the error so the next caller's lookup
// is a map hit.
func TestRunCache_ParsedSchemaCachesErrors(t *testing.T) {
	c := NewRunCache()
	var calls int32
	build := func() any {
		atomic.AddInt32(&calls, 1)
		return assert.AnError
	}
	for i := 0; i < 3; i++ {
		got := c.ParsedSchema("/abs/broken.md", build)
		require.ErrorIs(t, got.(error), assert.AnError)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"ParsedSchema must cache build results including errors")
}

// TestRunCache_ParsedSchemaInvalidateForcesRebuild pins that
// Invalidate drops the parsed-schema slot alongside FrontMatter,
// Includes, and Anchors. Without this, an LSP edit to the schema
// file would not refresh MDS020's view of it.
func TestRunCache_ParsedSchemaInvalidateForcesRebuild(t *testing.T) {
	c := NewRunCache()
	var calls int32
	c.ParsedSchema("/abs/schema.md", func() any {
		atomic.AddInt32(&calls, 1)
		return "v1"
	})
	c.Invalidate("/abs/schema.md")
	got := c.ParsedSchema("/abs/schema.md", func() any {
		atomic.AddInt32(&calls, 1)
		return "v2"
	})
	assert.Equal(t, "v2", got, "Invalidate must clear the parsed-schema slot")
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls),
		"ParsedSchema build must run again after Invalidate")
}

// TestRunCache_ParsedSchemaConcurrentSingleBuild pins the per-key
// once: the parallel file worker pool may race for the same schema
// path; the build must still run exactly once.
func TestRunCache_ParsedSchemaConcurrentSingleBuild(t *testing.T) {
	c := NewRunCache()
	var calls int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := c.ParsedSchema("/abs/shared.md", func() any {
				atomic.AddInt32(&calls, 1)
				return "once"
			})
			assert.Equal(t, "once", v)
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"ParsedSchema build must run exactly once under concurrent access")
}

// TestRunCache_CompiledCUEBuildsOnce pins that compiling the same CUE
// source string twice returns the cached value: the schema-frontmatter
// CUE expression is shared across every host file referencing the
// schema, so compiling it once per Run closes the second half of
// MDS020's parity gap.
//
// The key is the source string itself, not a path, so an inline
// `schema:` block and a schema file that produce the same CUE share
// the slot.
func TestRunCache_CompiledCUEBuildsOnce(t *testing.T) {
	c := NewRunCache()
	var calls int32
	build := func() any {
		atomic.AddInt32(&calls, 1)
		return "compiled-value"
	}
	const src = `close({id: string, status: "✅" | "🔳"})`
	for i := 0; i < 4; i++ {
		got := c.CompiledCUE(src, build)
		require.Equal(t, "compiled-value", got)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"CompiledCUE build must run exactly once per source string")
}

// TestRunCache_CompiledCUEDistinctSourcesDoNotShare pins that two
// different CUE sources keep independent slots: caching one must
// not silently serve the other.
func TestRunCache_CompiledCUEDistinctSourcesDoNotShare(t *testing.T) {
	c := NewRunCache()
	c.CompiledCUE(`{a: string}`, func() any { return "a-val" })
	c.CompiledCUE(`{b: string}`, func() any { return "b-val" })
	assert.Equal(t, "a-val", c.CompiledCUE(`{a: string}`,
		func() any { return "different" }))
	assert.Equal(t, "b-val", c.CompiledCUE(`{b: string}`,
		func() any { return "different" }))
}

// TestRunCache_CompiledCUEConcurrentSingleBuild pins the per-key
// once: many goroutines compiling the same CUE source must observe
// exactly one build.
func TestRunCache_CompiledCUEConcurrentSingleBuild(t *testing.T) {
	c := NewRunCache()
	var calls int32
	var wg sync.WaitGroup
	const src = `{x: int}`
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := c.CompiledCUE(src, func() any {
				atomic.AddInt32(&calls, 1)
				return "once"
			})
			assert.Equal(t, "once", v)
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"CompiledCUE build must run exactly once under concurrent access")
}
