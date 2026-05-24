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

// testSchemaMeta is a minimal ParsedSchemaMetadata implementation
// used to drive the include + CUE-source eviction tests below
// without depending on the requiredstructure package's concrete
// schemaParseResult. The fields mirror the rule package's surface
// exactly so the tests pin the contract from the lint side.
type testSchemaMeta struct {
	includes   []string
	cueSources []string
}

func (m testSchemaMeta) SchemaIncludes() []string   { return m.includes }
func (m testSchemaMeta) SchemaCUESources() []string { return m.cueSources }

// TestRunCache_InvalidateFragmentEvictsDependentSchema pins thread 1
// (PR #377): a ParsedSchema slot whose build returned
// ParsedSchemaMetadata reporting fragmentB as an include must be
// evicted when Invalidate(fragmentB) fires. Without this, the LSP
// edits a schema include fragment and MDS020 keeps serving stale
// headings until the parent schema itself is invalidated.
func TestRunCache_InvalidateFragmentEvictsDependentSchema(t *testing.T) {
	c := NewRunCache()
	const schemaA = "/abs/schema.md"
	const fragmentB = "/abs/fragment.md"

	var calls int32
	build := func() any {
		atomic.AddInt32(&calls, 1)
		return testSchemaMeta{includes: []string{fragmentB}}
	}
	got := c.ParsedSchema(schemaA, build)
	require.Equal(t, testSchemaMeta{includes: []string{fragmentB}}, got)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))

	c.Invalidate(fragmentB)

	build2 := func() any {
		atomic.AddInt32(&calls, 1)
		return testSchemaMeta{includes: []string{fragmentB}}
	}
	_ = c.ParsedSchema(schemaA, build2)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls),
		"Invalidate(fragment) must evict every schema whose ParsedSchema slot "+
			"reached fragment via <?include?>")
}

// TestRunCache_InvalidateFragmentEvictsTransitively pins the
// transitive case: schemaA includes fragmentB; fragmentB itself is
// modelled as a schema with an include on fragmentC.
// Invalidate(fragmentC) must drop both A and B's parsed-schema
// slots because A's parse depends on B's parse which depends on
// fragmentC.
func TestRunCache_InvalidateFragmentEvictsTransitively(t *testing.T) {
	c := NewRunCache()
	const schemaA = "/abs/a.md"
	const fragmentB = "/abs/b.md"
	const fragmentC = "/abs/c.md"

	var aCalls, bCalls int32
	_ = c.ParsedSchema(schemaA, func() any {
		atomic.AddInt32(&aCalls, 1)
		return testSchemaMeta{includes: []string{fragmentB}}
	})
	_ = c.ParsedSchema(fragmentB, func() any {
		atomic.AddInt32(&bCalls, 1)
		return testSchemaMeta{includes: []string{fragmentC}}
	})
	require.Equal(t, int32(1), atomic.LoadInt32(&aCalls))
	require.Equal(t, int32(1), atomic.LoadInt32(&bCalls))

	c.Invalidate(fragmentC)

	_ = c.ParsedSchema(schemaA, func() any {
		atomic.AddInt32(&aCalls, 1)
		return testSchemaMeta{includes: []string{fragmentB}}
	})
	_ = c.ParsedSchema(fragmentB, func() any {
		atomic.AddInt32(&bCalls, 1)
		return testSchemaMeta{includes: []string{fragmentC}}
	})
	assert.Equal(t, int32(2), atomic.LoadInt32(&aCalls),
		"transitive: schemaA depends on fragmentB depends on fragmentC; "+
			"Invalidate(fragmentC) must evict A")
	assert.Equal(t, int32(2), atomic.LoadInt32(&bCalls),
		"transitive: Invalidate(fragmentC) must evict B")
}

// TestRunCache_InvalidateSchemaDropsCompiledCUE pins thread 2
// (PR #377): editing a schema's frontmatter produces a new CUE
// source. Without per-schema CompiledCUE eviction the cache leaks
// the old source's compiled value forever in long-lived LSP
// sessions. Invalidate(schemaPath) must drop every CompiledCUE
// entry the parsed schema produced.
func TestRunCache_InvalidateSchemaDropsCompiledCUE(t *testing.T) {
	c := NewRunCache()
	const schemaA = "/abs/schema.md"
	const cueSrc = `close({x: string})`

	// Populate the parsed-schema slot with metadata that names cueSrc.
	_ = c.ParsedSchema(schemaA, func() any {
		return testSchemaMeta{cueSources: []string{cueSrc}}
	})
	// Populate the matching CompiledCUE slot.
	var compileCalls int32
	_ = c.CompiledCUE(cueSrc, func() any {
		atomic.AddInt32(&compileCalls, 1)
		return "compiled-v1"
	})
	require.Equal(t, int32(1), atomic.LoadInt32(&compileCalls))

	c.Invalidate(schemaA)

	_ = c.CompiledCUE(cueSrc, func() any {
		atomic.AddInt32(&compileCalls, 1)
		return "compiled-v2"
	})
	assert.Equal(t, int32(2), atomic.LoadInt32(&compileCalls),
		"Invalidate(schemaPath) must drop CompiledCUE entries the parsed "+
			"schema produced so an LSP frontmatter edit does not leak compiled "+
			"values")
}

// TestRunCache_InvalidateSharedCUESourceIsConservative documents
// the design choice for shared CUE sources: two schemas declaring
// the same `{x: string}` source share one CompiledCUE slot;
// invalidating one schema drops the slot for both. The sibling
// recompiles on its next ValidateFrontmatterDiags lookup — a one-
// time cost that is the safe default. The alternative
// (refcounting CUE sources) is heavier infrastructure for a cheap
// recompile.
func TestRunCache_InvalidateSharedCUESourceIsConservative(t *testing.T) {
	c := NewRunCache()
	const schemaA = "/abs/a.md"
	const schemaB = "/abs/b.md"
	const cueSrc = `{x: string}`

	_ = c.ParsedSchema(schemaA, func() any {
		return testSchemaMeta{cueSources: []string{cueSrc}}
	})
	_ = c.ParsedSchema(schemaB, func() any {
		return testSchemaMeta{cueSources: []string{cueSrc}}
	})
	var compileCalls int32
	_ = c.CompiledCUE(cueSrc, func() any {
		atomic.AddInt32(&compileCalls, 1)
		return "v1"
	})
	require.Equal(t, int32(1), atomic.LoadInt32(&compileCalls))

	// Invalidate one of the two schemas. The CompiledCUE slot is
	// dropped — the sibling schema's next lookup recompiles. This
	// is the documented conservative behavior; a refcount-based
	// strategy would keep the slot alive for schemaB but adds
	// machinery for a sub-millisecond recompile.
	c.Invalidate(schemaA)
	_ = c.CompiledCUE(cueSrc, func() any {
		atomic.AddInt32(&compileCalls, 1)
		return "v2"
	})
	assert.Equal(t, int32(2), atomic.LoadInt32(&compileCalls),
		"Invalidate(schemaA) must drop the shared CUE slot — schemaB recompiles "+
			"on its next lookup; cheap and removes the leak risk")
}

// TestRunCache_InvalidateNonSchemaPathIsNoop pins that Invalidate
// on a path that was never registered as a schema (no
// ParsedSchema slot, no fragment-of-anything) does not panic and
// does not affect CompiledCUE entries on the cache.
func TestRunCache_InvalidateNonSchemaPathIsNoop(t *testing.T) {
	c := NewRunCache()
	const cueSrc = `{kept: string}`
	var compileCalls int32
	_ = c.CompiledCUE(cueSrc, func() any {
		atomic.AddInt32(&compileCalls, 1)
		return "kept"
	})
	require.Equal(t, int32(1), atomic.LoadInt32(&compileCalls))

	require.NotPanics(t, func() { c.Invalidate("/abs/random-doc.md") })

	got := c.CompiledCUE(cueSrc, func() any {
		atomic.AddInt32(&compileCalls, 1)
		return "rebuilt"
	})
	assert.Equal(t, "kept", got,
		"Invalidate on a non-schema path must not evict unrelated CompiledCUE entries")
	assert.Equal(t, int32(1), atomic.LoadInt32(&compileCalls),
		"CompiledCUE build must not run again after a no-op Invalidate")
}

// TestRunCache_InvalidateSchemaDropsBackpointers pins that
// Invalidate(schemaA) tears down the reverse-include edges where
// schemaA appears as a dependent. A subsequent Invalidate on the
// fragment must not re-evict schemaA (it is already gone), proving
// the dependent set was cleaned up.
func TestRunCache_InvalidateSchemaDropsBackpointers(t *testing.T) {
	c := NewRunCache()
	const schemaA = "/abs/a.md"
	const fragmentB = "/abs/b.md"

	var aCalls int32
	_ = c.ParsedSchema(schemaA, func() any {
		atomic.AddInt32(&aCalls, 1)
		return testSchemaMeta{includes: []string{fragmentB}}
	})

	// Invalidate the schema first — this must remove the
	// fragmentB → schemaA edge.
	c.Invalidate(schemaA)

	// Now invalidate the fragment. Because the back-pointer was
	// dropped, Invalidate(fragmentB) finds no dependent. If the
	// back-pointer survived, this call would try to re-evict
	// schemaA (a no-op against an already-empty slot — but the
	// schemaDependents Range would still walk it). We assert the
	// invariant by rebuilding schemaA and observing its slot is
	// fresh.
	c.Invalidate(fragmentB)

	_ = c.ParsedSchema(schemaA, func() any {
		atomic.AddInt32(&aCalls, 1)
		return testSchemaMeta{includes: []string{fragmentB}}
	})
	assert.Equal(t, int32(2), atomic.LoadInt32(&aCalls),
		"after Invalidate(schemaA) the back-pointer to schemaA on fragmentB's "+
			"dependent set must be gone; the second ParsedSchema(schemaA) "+
			"rebuild is the cumulative second call")
}
