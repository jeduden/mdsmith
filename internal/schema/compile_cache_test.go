package schema

import (
	"sync"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCachedCompile_BuildsOncePerSource pins that compiling the same
// CUE source twice through CachedCompile returns the same cached
// wrapper. Plan 195 task 15 follow-up: extends the RunCache.CompiledCUE
// reuse to the schema package's frontmatter validator.
func TestCachedCompile_BuildsOncePerSource(t *testing.T) {
	cache := lint.NewRunCache()
	const src = `close({id: string})`
	v1 := CachedCompile(cache, src)
	v2 := CachedCompile(cache, src)
	require.NotNil(t, v1)
	require.NoError(t, v1.Err())
	assert.Same(t, v1, v2,
		"the same source string must return the same compiled-CUE wrapper")
}

// TestCachedCompile_NilCacheStillCompiles pins the struct-literal
// path: a missing RunCache falls back to a fresh compile so direct
// unit tests and tests passing nil cache still work.
func TestCachedCompile_NilCacheStillCompiles(t *testing.T) {
	v := CachedCompile(nil, `{id: string}`)
	require.NotNil(t, v)
	require.NoError(t, v.Err())
}

// TestCachedCompile_DistinctSourcesDoNotShare pins that two different
// CUE source strings produce independent entries.
func TestCachedCompile_DistinctSourcesDoNotShare(t *testing.T) {
	cache := lint.NewRunCache()
	v1 := CachedCompile(cache, `{a: string}`)
	v2 := CachedCompile(cache, `{b: string}`)
	require.NotNil(t, v1)
	require.NotNil(t, v2)
	assert.NotSame(t, v1, v2,
		"distinct CUE source strings must not share a slot")
}

// TestCachedCompile_ConcurrentSingleBuild pins that concurrent
// callers compiling the same source share one compile.
func TestCachedCompile_ConcurrentSingleBuild(t *testing.T) {
	cache := lint.NewRunCache()
	const src = `{shared: string}`
	var wg sync.WaitGroup
	results := make([]*CompiledCUE, 16)
	for i := 0; i < 16; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = CachedCompile(cache, src)
		}()
	}
	wg.Wait()
	for i := 1; i < len(results); i++ {
		assert.Same(t, results[0], results[i],
			"all concurrent callers must observe the same compiled-CUE wrapper")
	}
}
