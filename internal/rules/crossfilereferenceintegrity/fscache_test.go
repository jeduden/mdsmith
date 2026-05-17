package crossfilereferenceintegrity

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCachedStatExists_MemoizesOutcome pins that the existence result
// is cached: a path stat'd while it exists keeps reporting true even
// after the file is removed, which is the whole point of the
// process-lifetime cache (the filesystem is stable under a check run).
func TestCachedStatExists_MemoizesOutcome(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "target.md")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))

	assert.True(t, cachedStatExists(p), "existing file must stat true")

	require.NoError(t, os.Remove(p))
	assert.True(t, cachedStatExists(p), "result must be memoized, not re-stat'd")

	missing := filepath.Join(dir, "never.md")
	assert.False(t, cachedStatExists(missing), "absent file must stat false")
}

// TestCachedEvalSymlinks_ResolvesAndCaches checks a real path resolves
// (ok=true) and a non-existent one reports ok=false so callers apply
// their clean-path fallback.
func TestCachedEvalSymlinks_ResolvesAndCaches(t *testing.T) {
	dir := t.TempDir()
	real, ok := cachedEvalSymlinks(dir)
	require.True(t, ok)
	assert.NotEmpty(t, real)

	again, ok2 := cachedEvalSymlinks(dir)
	require.True(t, ok2)
	assert.Equal(t, real, again, "second call returns the cached resolution")

	_, ok3 := cachedEvalSymlinks(filepath.Join(dir, "no-such-path"))
	assert.False(t, ok3, "unresolvable path must report ok=false")
}

// TestFSCache_ConcurrentAccessRaceFree drives both caches from many
// goroutines because MDS027 runs under the multi-goroutine check walk;
// the package-level sync.Map caches must stay race-free. Run with -race.
func TestFSCache_ConcurrentAccessRaceFree(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.md")
	require.NoError(t, os.WriteFile(p, []byte("y"), 0o644))

	const goroutines = 24
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			assert.True(t, cachedStatExists(p))
			_, ok := cachedEvalSymlinks(dir)
			assert.True(t, ok)
		}()
	}
	wg.Wait()
}
