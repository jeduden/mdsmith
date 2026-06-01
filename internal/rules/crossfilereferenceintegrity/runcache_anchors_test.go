package crossfilereferenceintegrity

import (
	"errors"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// errReadFailed is a sentinel for the RunCache read-error
// retest path. Declared at package scope so require.ErrorIs
// compares against a single stable value rather than a
// per-test wrapper.
var errReadFailed = errors.New("simulated target read failure")

// TestAnchorsForFile_RunCacheHit pins the engine-wide anchor cache:
// a host with a RunCache and a target with a runCacheKey routes the
// build through RunCache.Anchors so subsequent host files looking
// at the same target hit the cached map instead of re-parsing.
// Covers the plan-195 perf-chunk path lines that the cache-hit
// and read-error tests do not reach (the existing tests pass
// `host == nil` so the RunCache branch never fires).
func TestAnchorsForFile_RunCacheHit(t *testing.T) {
	rc := lint.NewRunCache()
	host := &lint.File{RunCache: rc}

	target := targetFile{
		cacheKey:    "os:/abs/target.md",
		runCacheKey: "/abs/target.md",
		read: func() ([]byte, error) {
			return []byte("# Intro\n\nbody\n"), nil
		},
	}

	// First call: builds and stores.
	cache1 := map[string]map[string]struct{}{}
	anchors1, err := anchorsForFile(host, target, cache1)
	require.NoError(t, err)
	require.Contains(t, anchors1, "intro", "expected the anchor slug from `# Intro`")

	// Second call (fresh per-Check cache, same host/RunCache):
	// the read function must NOT fire — the cache resolves it.
	called := false
	target.read = func() ([]byte, error) {
		called = true
		return nil, nil
	}
	cache2 := map[string]map[string]struct{}{}
	anchors2, err := anchorsForFile(host, target, cache2)
	require.NoError(t, err)
	require.Contains(t, anchors2, "intro")
	require.False(t, called,
		"RunCache hit must skip the target.read() call on the second invocation")
}

// TestAnchorsForFile_RunCacheReadError pins that a read failure
// does not poison the RunCache slot — the next host file's check
// retries the read. Matches the per-Check cache's pre-plan-195
// semantics, where an unreadable target produced a host-side
// diagnostic without silencing siblings.
func TestAnchorsForFile_RunCacheReadError(t *testing.T) {
	rc := lint.NewRunCache()
	host := &lint.File{RunCache: rc}

	calls := 0
	target := targetFile{
		cacheKey:    "os:/abs/missing.md",
		runCacheKey: "/abs/missing.md",
		read: func() ([]byte, error) {
			calls++
			return nil, errReadFailed
		},
	}

	_, err1 := anchorsForFile(host, target, map[string]map[string]struct{}{})
	require.ErrorIs(t, err1, errReadFailed)
	require.Equal(t, 1, calls)

	_, err2 := anchorsForFile(host, target, map[string]map[string]struct{}{})
	require.ErrorIs(t, err2, errReadFailed)
	require.Equal(t, 2, calls,
		"a read error must not be cached — the retry on the next host file should re-call read()")
}

// TestAnchorsForFile_EmptyRunCacheKeySkipsRunCache pins the
// FS-only fallback: when target.runCacheKey is empty (the FS
// resolution path in resolveTargetFile, where there is no
// stable on-disk path to invalidate against), anchorsForFile
// uses the per-Check cache only and never reaches RunCache.
func TestAnchorsForFile_EmptyRunCacheKeySkipsRunCache(t *testing.T) {
	rc := lint.NewRunCache()
	host := &lint.File{RunCache: rc}

	target := targetFile{
		cacheKey:    "fs:rel/target.md",
		runCacheKey: "", // empty ⇒ skip RunCache
		read: func() ([]byte, error) {
			return []byte("# Intro\n"), nil
		},
	}

	// Two separate per-Check caches simulate two host files.
	// Without RunCache, each one calls read() independently.
	cache1 := map[string]map[string]struct{}{}
	_, err := anchorsForFile(host, target, cache1)
	require.NoError(t, err)

	calls := 0
	target.read = func() ([]byte, error) {
		calls++
		return []byte("# Intro\n"), nil
	}
	cache2 := map[string]map[string]struct{}{}
	_, err = anchorsForFile(host, target, cache2)
	require.NoError(t, err)
	require.Equal(t, 1, calls,
		"FS-only target with empty runCacheKey must not share through RunCache")
}
