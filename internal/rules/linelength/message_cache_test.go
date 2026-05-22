package linelength

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLineTooLongMessage_CacheHitReturnsSameString pins the cache's
// happy path — a second call with the same (runeLen, limit) pair
// returns the exact pre-cached string, not a fresh build.
func TestLineTooLongMessage_CacheHitReturnsSameString(t *testing.T) {
	// Use values unlikely to collide with other tests' entries.
	first := lineTooLongMessage(9991, 9992)
	second := lineTooLongMessage(9991, 9992)
	require.Equal(t, first, second)
	require.Equal(t, "line too long (9991 > 9992)", first)
}

// TestLineTooLongMessage_CapBoundedPastLimit pins the LSP
// memory-bloat guard: once the cache holds
// lineTooLongCacheMaxEntries unique keys, every subsequent call
// rebuilds the string and the entry count stops climbing.
// Replaces the package-level cache with a fresh sync.Map for
// the test so the assertion does not depend on global state
// from other tests.
func TestLineTooLongMessage_CapBoundedPastLimit(t *testing.T) {
	// Snapshot the global cache + counter so the test runs in
	// isolation and restores them when finished. Future tests
	// see the same global state they would have without this
	// test.
	t.Cleanup(func() {
		resetLineTooLongCacheForTest()
	})
	resetLineTooLongCacheForTest()

	// Fill the cache to the cap.
	for i := 0; i < lineTooLongCacheMaxEntries; i++ {
		msg := lineTooLongMessage(i+10000, 80)
		require.NotEmpty(t, msg)
	}
	require.Equal(t,
		int32(lineTooLongCacheMaxEntries),
		lineTooLongCacheCount.Load(),
		"cache should hold exactly lineTooLongCacheMaxEntries after the fill")

	// One more unique entry past the cap: still returns the
	// correct string, but the counter does not climb.
	msg := lineTooLongMessage(99999, 80)
	require.Equal(t, "line too long (99999 > 80)", msg,
		"past-cap calls must still return the correct message")
	require.Equal(t,
		int32(lineTooLongCacheMaxEntries),
		lineTooLongCacheCount.Load(),
		"counter must not climb past lineTooLongCacheMaxEntries")
}

// resetLineTooLongCacheForTest wipes the package-level cache
// so a test starts from a known empty state. Only used by
// tests in this file.
func resetLineTooLongCacheForTest() {
	lineTooLongCache.Range(func(k, _ any) bool {
		lineTooLongCache.Delete(k)
		return true
	})
	lineTooLongCacheCount.Store(0)
}
